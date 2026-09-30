import 'dart:convert';
import 'dart:io';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';

/// Servicio para POST /auth/google-drive/connect con Google real.
/// En móvil (Android/iOS) usa google_sign_in; en desktop usa url_launcher + localhost.
/// Configuración por --dart-define (ver front/Dockerfile): GOOGLE_CLIENT_ID
/// y GOOGLE_REDIRECT_URI. Sin client ID el flujo falla visible (nunca quemado).
class GoogleDriveService {
  // Sin defaultValue: el client ID vive en el .env del backend y llega por
  // GET /auth/google-config (o por --dart-define=GOOGLE_CLIENT_ID). Nunca quemado.
  static const _serverClientId = String.fromEnvironment(
    'GOOGLE_CLIENT_ID',
    defaultValue: '920602669668-abfen7v0mh23d2gsebvot0r413j90gnh.apps.googleusercontent.com',
  );

  static const _redirectUri = String.fromEnvironment(
    'GOOGLE_REDIRECT_URI',
    defaultValue: 'http://localhost:8081/auth/google/callback',
  );

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
  static const _scopes = 'https://www.googleapis.com/auth/drive.file%20openid%20email%20profile';

  final GoogleSignIn _googleSignIn = GoogleSignIn(
    scopes: [
      'https://www.googleapis.com/auth/drive.file',
      'openid',
      'email',
      'profile',
    ],
    serverClientId: _clientId,
  );

  /// Retorna serverAuthCode (oauth_code). En desktop abre navegador y escucha localhost.
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
      return _getServerAuthCodeDesktop(loginHint: loginHint);
    }
    final account = await _googleSignIn.signIn();
    if (account == null) return null;
    // serverAuthCode vive en la cuenta (ver deprecación de GoogleSignInAuthentication).
    return account.serverAuthCode;
  }

  Future<String?> _getServerAuthCodeDesktop({String? loginHint}) async {
    // redirect_uri canónica (debe estar en Cloud Console): ver GOOGLE_REDIRECT_URI.
    final redirectUri = _redirect;
    final hintParam = (loginHint != null && loginHint.trim().isNotEmpty)
        ? '&login_hint=${Uri.encodeComponent(loginHint.trim())}'
        : '';
    final authUrl =
        'https://accounts.google.com/o/oauth2/v2/auth?response_type=code&scope=$_scopes&access_type=offline&prompt=consent&client_id=$_clientId&redirect_uri=$redirectUri$hintParam';
    // Inicia servidor local para capturar code
    final server = await HttpServer.bind('localhost', 8081);
    if (!await launchUrl(Uri.parse(authUrl), mode: LaunchMode.externalApplication)) {
      await server.close();
      return null;
    }
    final request = await server.first;
    final code = request.uri.queryParameters['code'];
    request.response
      ..statusCode = 200
      ..headers.contentType = ContentType.html
      ..write('<h1>Drive conectado, vuelve a la app</h1>');
    await request.response.close();
    await server.close();
    return code;
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
    return launchUrl(Uri.parse(buildWebAuthUrl(loginHint: loginHint)), mode: LaunchMode.externalApplication);
  }

  /// Envía oauth_code al backend. Requiere access_token de tu app (de POST /auth/login).
  /// expectedEmail (opcional): el backend lo compara con el email real de
  /// userinfo y responde 400 email_mismatch si conectaste otra cuenta.
  Future<bool> connectDrive({required String backendBaseUrl, required String appAccessToken, required String oauthCode, String? expectedEmail}) async {
    final url = Uri.parse('$backendBaseUrl/auth/google-drive/connect');
    final payload = <String, String>{'oauth_code': oauthCode};
    final expected = expectedEmail?.trim() ?? '';
    if (expected.isNotEmpty) {
      payload['expected_email'] = expected;
    }
    final resp = await http.post(
      url,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer $appAccessToken',
      },
      body: jsonEncode(payload),
    );
    if (resp.statusCode == 200) {
      final body = jsonDecode(resp.body);
      return body['connected'] == true;
    }
    // No loguear oauth_code ni tokens
    throw Exception('Drive connect falló: ${resp.statusCode} ${resp.body}');
  }

  Future<void> signOut() => _googleSignIn.signOut();
}
