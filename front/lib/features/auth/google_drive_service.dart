import 'dart:convert';
import 'dart:io';
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';

/// Servicio para POST /auth/google-drive/connect con Google real.
/// En móvil (Android/iOS) usa google_sign_in; en desktop usa url_launcher + localhost.
/// Configuración por --dart-define (ver front/Dockerfile): GOOGLE_CLIENT_ID
/// y GOOGLE_REDIRECT_URI. Sin client ID el flujo falla visible (nunca quemado).
class GoogleDriveService {
  static const _serverClientId = String.fromEnvironment(
    'GOOGLE_CLIENT_ID',
    defaultValue: '',
  );

  static const _redirectUri = String.fromEnvironment(
    'GOOGLE_REDIRECT_URI',
    defaultValue: 'http://localhost:8081/auth/google/callback',
  );

  /// true si hay client ID configurado para iniciar el flujo OAuth.
  static bool get isConfigured => _serverClientId.trim().isNotEmpty;

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
    serverClientId: _serverClientId,
  );

  /// Retorna serverAuthCode (oauth_code). En desktop abre navegador y escucha localhost.
  /// Lanza StateError visible si GOOGLE_CLIENT_ID no está configurado.
  Future<String?> getServerAuthCode({String? loginHint}) async {
    if (!isConfigured) {
      throw StateError(
        'GOOGLE_CLIENT_ID no configurado: rebuild con --dart-define=GOOGLE_CLIENT_ID=... (ver front/Dockerfile)',
      );
    }
    if (Platform.isWindows || Platform.isLinux) {
      return _getServerAuthCodeDesktop(loginHint: loginHint);
    }
    final account = await _googleSignIn.signIn();
    if (account == null) return null;
    final auth = await account.authentication;
    return auth.serverAuthCode;
  }

  Future<String?> _getServerAuthCodeDesktop({String? loginHint}) async {
    // redirect_uri canónica (debe estar en Cloud Console): ver GOOGLE_REDIRECT_URI.
    final redirectUri = _redirectUri;
    final hintParam = (loginHint != null && loginHint.trim().isNotEmpty)
        ? '&login_hint=${Uri.encodeComponent(loginHint.trim())}'
        : '';
    final authUrl =
        'https://accounts.google.com/o/oauth2/v2/auth?response_type=code&scope=$_scopes&access_type=offline&prompt=consent&client_id=$_serverClientId&redirect_uri=$redirectUri$hintParam';
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
