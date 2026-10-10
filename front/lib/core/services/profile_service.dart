import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../models/profile_models.dart';
import 'api_config.dart';
import 'authed_client.dart';
import 'session_manager.dart';

// Cliente SOLO para Perfil (Auth/Identity).
// No mezclar con Social (ver social_service.dart).
// Contratos verificados en back/auth/cmd/server/main.go:107-108 y
// back/auth/internal/handler/http/profile_handler.go.
class ProfileService {
  ProfileService({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  static String get baseUrl => authApiBaseUrl;

  /// Último perfil conocido en el proceso (única fuente reactiva).
  /// Se actualiza en cada GET/PATCH exitoso; MainShell lo escucha para
  /// refrescar el sidebar sin logout/login.
  static final ValueNotifier<UserProfile?> current = _sessionProfile();

  static ValueNotifier<UserProfile?> _sessionProfile() {
    final value = ValueNotifier<UserProfile?>(null);
    SessionManager.revision.addListener(() => value.value = null);
    return value;
  }

  void _checkSession(int revision) {
    if (SessionManager.revision.value != revision) {
      throw ProfileApiException('La sesión cambió.', code: 'session_changed');
    }
  }

  Future<bool> getDriveStatus() async {
    final revision = SessionManager.revision.value;
    final res = await AuthedHttp.run(() {
      _checkSession(revision);
      return _client
          .get(
            Uri.parse('$baseUrl/auth/google-drive/status'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10));
    });
    _checkSession(revision);
    if (res.statusCode != 200) throw _toError(res);
    return (jsonDecode(res.body) as Map<String, dynamic>)['connected'] == true;
  }

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
    return ProfileApiException(message, statusCode: res.statusCode, code: code);
  }

  // GET /profile/me -> 200 {display_name, photo_url, phone, institution, description, visibility}
  Future<UserProfile> getProfile() async {
    final revision = SessionManager.revision.value;
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$baseUrl/profile/me'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
    _checkSession(revision);
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final profile = UserProfile.fromJson(body);
    current.value = profile;
    return profile;
  }

  // PATCH /profile/me (parcial) -> 200 perfil actualizado.
  // Body vacío -> 400. user_id prohibido -> 400.
  Future<UserProfile> patchProfile(Map<String, dynamic> patch) async {
    final revision = SessionManager.revision.value;
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse('$baseUrl/profile/me'),
            headers: _headers(json: true),
            body: jsonEncode(patch),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    _checkSession(revision);
    final updated = UserProfile.fromJson(body);
    current.value = updated;
    return updated;
  }

  void dispose() {
    _client.close();
  }
}
