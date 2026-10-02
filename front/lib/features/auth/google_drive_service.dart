import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';

import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';

/// Resultado de POST /auth/google-drive/connect con mensaje humano.
/// Nunca expone c��digos HTTP, JSON interno ni detalles del proveedor.
class DriveConnectResult {
  const DriveConnectResult.ok() : ok = true, message = 'Drive conectado.';

  const DriveConnectResult.error(this.message) : ok = false;

  final bool ok;
  final String message;
}

/// Resultado del flujo desktop: código + redirect real usado (puerto efímero).
typedef DriveDesktopAuth = ({String code, String redirectUri});

/// El puerto loopback local está ocupado (ni efímero ni rango 8081-8090).
/// El llamador debe caer al flujo manual pegar-código con mensaje claro.
class LocalPortBusyException implements Exception {
  const LocalPortBusyException();
  @override
  String toString() => 'Puerto local ocupado: cierra la otra instancia e inténtalo de nuevo, o pega el código manualmente.';
}

/// Servicio para POST /auth/google-drive/connect con Google real.
/// En m��vil (Android/iOS) usa google_sign_in; en Windows usa url_launcher + localhost.
/// Configuración: backend (.env raíz, vía GET /auth/google-config) o
/// --dart-define (ver front/Dockerfile). Sin client ID el flujo falla visible.
class GoogleDriveService {
  // Sin defaultValue: el client ID vive en el .env del backend y llega por
  // GET /auth/google-config (o por --dart-define=GOOGLE_CLIENT_ID). Nunca quemado.
  static const _serverClientId = String.fromEnvironment(
    'GOOGLE_CLIENT_ID',
    // Vacío a propósito: el ID vive en el backend (GET /auth/google-config).
    // Nunca quemar credenciales en el frontend.
    defaultValue: '',
  );

  static const _redirectUri = String.fromEnvironment(
    'GOOGLE_REDIRECT_URI',
    defaultValue: 'http://localhost:8081/auth/google/callback',
  );

  /// Redirect fija legacy = la registrada en Cloud Console para cliente Web.
  /// Debe coincidir carácter por carácter o Google responde mismatch.
  static const legacyDesktopRedirect =
      'http://localhost:8081/auth/google/callback';

  // Config remota (cacheada) servida por el backend: GET {auth}/auth/google-config.
  // Prioridad: remoto > --dart-define > default. Así el .env raíz es la única fuente.
  static String? _remoteClientId;
  static String? _remoteRedirectUri;

  static String get _clientId {
    final remote = _remoteClientId?.trim() ?? '';
    if (remote.isNotEmpty) return remote;
    return _serverClientId.trim();
  }

  static String get _redirect {
    final remote = _remoteRedirectUri?.trim() ?? '';
    if (remote.isNotEmpty) return remote;
    return _redirectUri;
  }

  /// Trae la config OAuth desde el backend (una vez; luego cache).
  /// Llamar antes de getServerAuthCode para no depender de --dart-define.
  static Future<void> ensureConfigured({required String backendBaseUrl}) async {
    if ((_remoteClientId ?? '').trim().isNotEmpty) return;
    final base = backendBaseUrl.trim().replaceFirst(RegExp(r'/+$'), '');
    if (base.isEmpty) return;
    try {
      final resp = await http
          .get(Uri.parse('$base/auth/google-config'))
          .timeout(const Duration(seconds: 8));
      if (resp.statusCode != 200) return;
      final body = jsonDecode(resp.body);
      if (body is! Map) return;
      final clientId = (body['client_id'] ?? '').toString().trim();
      final redirectUri = (body['redirect_uri'] ?? '').toString().trim();
      // Solo cachea valores no vacíos: si el backend responde incompleto,
      // queda el fallback (--dart-define) y se reintenta en la próxima conexión.
      if (clientId.isNotEmpty) _remoteClientId = clientId;
      if (redirectUri.isNotEmpty) _remoteRedirectUri = redirectUri;
    } catch (_) {
      // Sin red no hay config remota: caen los defines/defaults.
    }
  }

  /// true si hay client ID configurado para iniciar el flujo OAuth.
  static bool get isConfigured => _clientId.isNotEmpty;

  // Scopes mínimos: drive.file (solo archivos creados por la app, dentro de
  // la carpeta "Apuntes TI3") + openid email profile (capturar el correo).
  // Nunca drive completo (vería todo el Drive) ni solo lectura (sin escritura).
  static const _scopes =
      'https://www.googleapis.com/auth/drive.file%20openid%20email%20profile';

  final GoogleSignIn _googleSignIn = GoogleSignIn(
    scopes: [
      'https://www.googleapis.com/auth/drive.file',
      'openid',
      'email',
      'profile',
    ],
    serverClientId: _clientId,
  );

  /// Retorna serverAuthCode (oauth_code). En desktop abre navegador y escucha
  /// en loopback (ver getDesktopAuthCode); en móvil usa google_sign_in.
  /// En web retorna null (sin localhost disponible): usar buildWebAuthUrl +
  /// diálogo pegar-código. Lanza StateError visible si GOOGLE_CLIENT_ID falta.
  Future<String?> getServerAuthCode({String? loginHint}) async {
    if (!isConfigured) {
      throw StateError(
        'GOOGLE_CLIENT_ID no configurado: rebuild con --dart-define=GOOGLE_CLIENT_ID=... (ver front/Dockerfile)',
      );
    }
    if (kIsWeb) return null;
    if (Platform.isWindows || Platform.isLinux) {
      final auth = await getDesktopAuthCode(loginHint: loginHint);
      return auth?.code;
    }
    final account = await _googleSignIn.signIn();
    if (account == null) return null;
    // serverAuthCode vive en la cuenta (ver deprecación de GoogleSignInAuthentication).
    return account.serverAuthCode;
  }

  /// Flujo desktop con cliente OAuth tipo Web: Google exige redirect_uri
  /// EXACTA a la registrada. Se usa la fija legacy
  /// http://localhost:8081/auth/google/callback (NO puerto efímero: con
  /// cliente Web daría redirect_uri_mismatch garantizado).
  /// Si el puerto está ocupado -> LocalPortBusyException (cae al pegar-código
  /// con la misma redirect fija: el navegador muestra error de conexión pero
  /// la URL trae el ?code= para copiar).
  /// Retorna código + redirect usada, para enviarla en connectDrive.
  /// null = usuario canceló en Google (?error=) o cerró el navegador.
  /// Lanza LocalPortBusyException si no hay puerto libre.
  Future<DriveDesktopAuth?> getDesktopAuthCode({
    String? loginHint,
    String? scopes,
  }) async {
    if (!isConfigured) {
      throw StateError(
        'GOOGLE_CLIENT_ID no configurado: rebuild con --dart-define=GOOGLE_CLIENT_ID=... (ver front/Dockerfile)',
      );
    }
    final customScopes = scopes?.trim() ?? '';
    final scopeParam = customScopes.isEmpty ? _scopes : customScopes;
    final hintParam = (loginHint != null && loginHint.trim().isNotEmpty)
        ? '&login_hint=${Uri.encodeComponent(loginHint.trim())}'
        : '';

    HttpServer? server;
    try {
      server = await HttpServer.bind('localhost', 8081);
    } catch (_) {
      server = null;
    }
    if (server == null) {
      throw const LocalPortBusyException();
    }
    const redirectUri = legacyDesktopRedirect;
    final bound = server;
    try {
      final authUrl =
          'https://accounts.google.com/o/oauth2/v2/auth?response_type=code&scope=$scopeParam&access_type=offline&prompt=consent&client_id=$_clientId&redirect_uri=$redirectUri$hintParam';
      if (!await launchUrl(
        Uri.parse(authUrl),
        mode: LaunchMode.externalApplication,
      )) {
        return null;
      }
      // Espera el retorno con timeout; ?error=access_denied = cancelación.
      final request = await bound.first.timeout(
        const Duration(minutes: 3),
        onTimeout: () => throw TimeoutException('Se agotó el tiempo de autorización en Google.'),
      );
      final params = request.uri.queryParameters;
      request.response
        ..statusCode = 200
        ..headers.contentType = ContentType.html
        ..write('<h1>Drive conectado, vuelve a la app</h1>');
      await request.response.close();
      final code = (params['code'] ?? '').trim();
      if (code.isEmpty) return null;
      return (code: code, redirectUri: redirectUri);
    } finally {
      await bound.close(force: true);
    }
  }

  /// URL de autorización para flujo web manual (pegar-código):
  /// se abre en el navegador y Google redirige a redirect_uri con ?code=.
  /// Como nada escucha ese puerto en web, el usuario copia el code de la
  /// barra de direcciones y lo pega en la app.
  String buildWebAuthUrl({String? loginHint}) {
    final hint = (loginHint != null && loginHint.trim().isNotEmpty)
        ? '&login_hint=${Uri.encodeComponent(loginHint.trim())}'
        : '';
    return 'https://accounts.google.com/o/oauth2/v2/auth?response_type=code&scope=$_scopes&access_type=offline&prompt=consent&client_id=$_clientId&redirect_uri=$_redirect$hint';
  }

  Future<bool> openWebAuthUrl({String? loginHint}) {
    return launchUrl(
      Uri.parse(buildWebAuthUrl(loginHint: loginHint)),
      mode: LaunchMode.externalApplication,
    );
  }

  /// Envía oauth_code al backend. Requiere access_token de tu app (de POST /auth/login).
  /// expectedEmail (opcional): el backend lo compara con la cuenta real de
  /// Google y rechaza con email_mismatch si elegiste otra cuenta. El campo es
  /// ignorado por backends anteriores (compatibilidad hacia adelante).
  /// Nunca lanza con detalles técnicos: devuelve el mensaje para mostrar.
  Future<DriveConnectResult> connectDrive({
    required String backendBaseUrl,
    required String appAccessToken,
    required String oauthCode,
    String? expectedEmail,
    String? redirectUri,
  }) async {
    final revision = SessionManager.revision.value;
    final url = Uri.parse('$backendBaseUrl/auth/google-drive/connect');
    final payload = <String, String>{'oauth_code': oauthCode};
    final expected = expectedEmail?.trim() ?? '';
    if (expected.isNotEmpty) {
      payload['expected_email'] = expected;
    }
    final redirect = redirectUri?.trim() ?? '';
    if (redirect.isNotEmpty) {
      payload['redirect_uri'] = redirect;
    }
    http.Response resp;
    try {
      resp = await AuthedHttp.run(() {
        if (SessionManager.revision.value != revision) {
          throw StateError('La sesión cambió');
        }
        return http
            .post(
              url,
              headers: {
                'Content-Type': 'application/json',
                // Token vigente en cada intento (rota tras refresh).
                'Authorization':
                    'Bearer ${SessionManager.token ?? appAccessToken}',
              },
              body: jsonEncode(payload),
            )
            .timeout(const Duration(seconds: 15));
      });
    } catch (_) {
      return const DriveConnectResult.error(
        'No se pudo conectar con Google Drive. Inténtalo nuevamente más tarde.',
      );
    }
    if (resp.statusCode == 200) {
      try {
        final body = jsonDecode(resp.body);
        if (body is Map && body['connected'] == true) {
          return const DriveConnectResult.ok();
        }
      } catch (_) {}
      return const DriveConnectResult.error(
        'No se pudo completar la conexión. Inténtalo nuevamente.',
      );
    }
    String? code;
    try {
      final body = jsonDecode(resp.body);
      if (body is Map) {
        final err = body['error'];
        if (err is Map) code = err['code']?.toString();
      }
    } catch (_) {}
    switch (resp.statusCode) {
      case 400 when code == 'email_mismatch':
        return const DriveConnectResult.error(
          'La cuenta de Google elegida no coincide con tu correo. Vuelve a intentarlo con la cuenta correcta.',
        );
      case 400:
        return const DriveConnectResult.error(
          'No se pudo completar la conexión. Inténtalo nuevamente.',
        );
      case 401:
        return const DriveConnectResult.error(
          'Tu sesión venció. Vuelve a iniciar sesión e inténtalo de nuevo.',
        );
      case 502:
        return const DriveConnectResult.error(
          'No se pudo conectar con Google Drive. Inténtalo nuevamente más tarde.',
        );
      default:
        return const DriveConnectResult.error(
          'No se pudo conectar con Google Drive. Inténtalo nuevamente más tarde.',
        );
    }
  }

  Future<void> signOut() => _googleSignIn.signOut();

  /// Desconecta por completo: cierra sesión local de Google y pide al backend
  /// que revoque en Google y borre la fila (DELETE /auth/google-drive/connection).
  /// Ya no hay que revocar a mano desde la cuenta. Retorna mensaje para mostrar.
  Future<String> disconnectDrive({
    required String backendBaseUrl,
    required String appAccessToken,
  }) async {
    try {
      await signOut();
    } catch (_) {}
    try {
      final resp = await http
          .delete(
            Uri.parse('$backendBaseUrl/auth/google-drive/connection'),
            headers: {
              'Authorization': 'Bearer $appAccessToken',
            },
          )
          .timeout(const Duration(seconds: 10));
      if (resp.statusCode == 204) return 'Drive desconectado.';
      return 'Drive desconectado localmente.';
    } catch (_) {
      return 'Drive desconectado localmente.';
    }
  }
}
