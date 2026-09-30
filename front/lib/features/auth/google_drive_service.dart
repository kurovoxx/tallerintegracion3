import 'dart:convert';
import 'dart:io';
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';

import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';

/// Resultado de POST /auth/google-drive/connect con mensaje humano.
/// Nunca expone códigos HTTP, JSON interno ni detalles del proveedor.
class DriveConnectResult {
  const DriveConnectResult.ok() : ok = true, message = 'Drive conectado.';

  const DriveConnectResult.error(this.message) : ok = false;

  final bool ok;
  final String message;
}

/// Servicio para POST /auth/google-drive/connect con Google real.
/// En móvil (Android/iOS) usa google_sign_in; en Windows usa url_launcher + localhost.
class GoogleDriveService {
  static const _serverClientId = String.fromEnvironment(
    'GOOGLE_CLIENT_ID',
    defaultValue:
        '148833740945-b2k56p1fb6oqbpo076gvgbulm40ks7r1.apps.googleusercontent.com',
  );

  final GoogleSignIn _googleSignIn = GoogleSignIn(
    scopes: [
      'https://www.googleapis.com/auth/drive.file',
      'openid',
      'email',
      'profile',
    ],
    serverClientId: _serverClientId,
  );

  /// Retorna serverAuthCode (oauth_code). En Windows abre navegador y escucha localhost.
  Future<String?> getServerAuthCode() async {
    if (Platform.isWindows || Platform.isLinux) {
      return _getServerAuthCodeWindows();
    }
    final account = await _googleSignIn.signIn();
    if (account == null) return null;
    final auth = await account.authentication;
    return auth.serverAuthCode;
  }

  Future<String?> _getServerAuthCodeWindows() async {
    // Usa redirect_uri http://localhost:8081/auth/google/callback (debe estar en Cloud Console)
    const redirectUri = 'http://localhost:8081/auth/google/callback';
    const authUrl =
        'https://accounts.google.com/o/oauth2/v2/auth?response_type=code&scope=https://www.googleapis.com/auth/drive.file%20openid%20email%20profile&access_type=offline&prompt=consent&client_id=$_serverClientId&redirect_uri=$redirectUri';
    // Inicia servidor local para capturar code
    final server = await HttpServer.bind('localhost', 8081);
    if (!await launchUrl(
      Uri.parse(authUrl),
      mode: LaunchMode.externalApplication,
    )) {
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
  }) async {
    final url = Uri.parse('$backendBaseUrl/auth/google-drive/connect');
    final payload = <String, String>{'oauth_code': oauthCode};
    final expected = expectedEmail?.trim() ?? '';
    if (expected.isNotEmpty) {
      payload['expected_email'] = expected;
    }
    http.Response resp;
    try {
      resp = await AuthedHttp.run(
        () => http
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
            .timeout(const Duration(seconds: 15)),
      );
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
}
