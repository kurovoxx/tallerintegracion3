import 'dart:convert';
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;

/// Servicio temporal para probar POST /auth/google-drive/connect con Google real.
/// No se usa en producción aún; solo para verificar que el backend con
/// GOOGLE_CLIENT_ID/SECRET/REDIRECT_URI=postmessage funciona con tu cuenta
/// agustin.vega2024@alu.uct.cl y el cliente "Cliente testeo web".
class GoogleDriveService {
  // Usa el mismo serverClientId que está en back/auth/.env GOOGLE_CLIENT_ID
  // Para google_sign_in, se pasa como serverClientId para obtener serverAuthCode.
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

  /// Inicia flujo Google Sign-In y retorna serverAuthCode (oauth_code) para backend.
  /// Retorna null si el usuario cancela.
  Future<String?> getServerAuthCode() async {
    final account = await _googleSignIn.signIn();
    if (account == null) return null;
    final auth = await account.authentication;
    return auth.serverAuthCode; // <- este es tu oauth_code
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
