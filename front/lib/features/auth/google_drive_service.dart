import 'dart:convert';
import 'dart:io';
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';

/// Servicio temporal para probar POST /auth/google-drive/connect con Google real.
/// En móvil (Android/iOS) usa google_sign_in; en Windows usa url_launcher + localhost.
class GoogleDriveService {
  static const _serverClientId = String.fromEnvironment(
    'GOOGLE_CLIENT_ID',
    defaultValue: '148833740945-b2k56p1fb6oqbpo076gvgbulm40ks7r1.apps.googleusercontent.com',
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
  Future<bool> connectDrive({required String backendBaseUrl, required String appAccessToken, required String oauthCode}) async {
    final url = Uri.parse('$backendBaseUrl/auth/google-drive/connect');
    final resp = await http.post(
      url,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer $appAccessToken',
      },
      body: jsonEncode({'oauth_code': oauthCode}),
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
