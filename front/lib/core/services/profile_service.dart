import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/profile_models.dart';
import 'api_config.dart';
import 'session_manager.dart';

// Cliente SOLO para Perfil (Auth/Identity).
// No mezclar con Social (ver social_service.dart).
// Contratos verificados en back/auth/cmd/server/main.go:107-108 y
// back/auth/internal/handler/http/profile_handler.go.
class ProfileService {
  ProfileService({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  static String get baseUrl => authApiBaseUrl;

  Map<String, String> _headers({bool json = false}) {
    final token = SessionManager.token;
    if (token == null || token.isEmpty) {
      throw ProfileApiException(
        'No hay sesión. Inicia sesión para ver tu perfil.',
        code: 'no_session',
      );
    }
    return <String, String>{
      if (json) 'Content-Type': 'application/json',
      'Accept': 'application/json',
      'Authorization': 'Bearer $token',
    };
  }

  ProfileApiException _toError(http.Response res) {
    String message = 'Error HTTP ${res.statusCode}';
    String? code;
    try {
      if (res.body.isNotEmpty) {
        final body = jsonDecode(utf8.decode(res.bodyBytes));
        if (body is Map) {
          final err = body['error'];
          if (err is Map) {
            code = err['code']?.toString();
            message = err['message']?.toString() ?? err.toString();
          } else if (err is String) {
            message = err;
          }
        }
      }
    } catch (_) {}
    if (res.statusCode == 401) {
      message = 'No autorizado (401). Revisa tu sesión.';
      code ??= 'unauthorized';
    }
    return ProfileApiException(message,
        statusCode: res.statusCode, code: code);
  }

  // GET /profile/me -> 200 {display_name, photo_url, phone, institution, description, visibility}
  Future<UserProfile> getProfile() async {
    final res = await _client
        .get(Uri.parse('$baseUrl/profile/me'), headers: _headers())
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return UserProfile.fromJson(body);
  }

  // PATCH /profile/me (parcial) -> 200 perfil actualizado.
  // Body vacío -> 400. user_id prohibido -> 400.
  Future<UserProfile> patchProfile(Map<String, dynamic> patch) async {
    final res = await _client
        .patch(
          Uri.parse('$baseUrl/profile/me'),
          headers: _headers(json: true),
          body: jsonEncode(patch),
        )
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return UserProfile.fromJson(body);
  }

  void dispose() {
    _client.close();
  }
}
