import 'dart:convert';

import 'package:http/http.dart' as http;

import 'auth_service.dart';
import 'session_manager.dart';

/// Reintento autenticado centralizado para todos los clientes HTTP.
///
/// Flujo: ejecuta [attempt] (que construye headers con el token vigente);
/// si responde 401 UNA vez, intenta `POST /auth/refresh` UNA sola vez,
/// guarda los tokens rotados y reintenta la petición original UNA vez.
/// Si el refresh falla (revocado/expirado/inválido/sin red), limpia la
/// sesión, notifica para navegar a Login y devuelve el 401 original para
/// que la UI muestre un mensaje humano.
///
/// Sin loops: como máximo 1 refresh + 1 reintento por llamada.
/// Sin secretos en logs: nunca registra tokens.
class AuthedHttp {
  AuthedHttp._();

  /// Refrescador inyectable (tests). Por defecto usa AuthService real.
  static Future<RefreshResult> Function(String refreshToken)? refreshOverride;

  static bool _isExpired(http.Response res) {
    if (res.statusCode != 401) return false;
    try {
      if (res.body.isEmpty) return true;
      final body = jsonDecode(utf8.decode(res.bodyBytes));
      if (body is! Map) return true;
      final err = body['error'];
      if (err is Map) {
        final code = err['code']?.toString() ?? '';
        return code.isEmpty ||
            code == 'token_expired' ||
            code == 'invalid_token' ||
            code == 'unauthorized';
      }
      return true;
    } catch (_) {
      return true;
    }
  }

  static Future<RefreshResult> _doRefresh(String token) {
    final fn = refreshOverride;
    if (fn != null) return fn(token);
    return AuthService().refresh(token);
  }

  static Future<http.Response> run(
    Future<http.Response> Function() attempt,
  ) async {
    final first = await attempt();
    if (!_isExpired(first)) return first;

    final current = SessionManager.refreshToken;
    if (current == null || current.isEmpty) {
      await SessionManager.expireSession();
      return first;
    }
    final refreshed = await _doRefresh(current);
    final access = refreshed.accessToken;
    if (!refreshed.success || access == null || access.isEmpty) {
      await SessionManager.expireSession();
      return first;
    }
    await SessionManager.updateTokens(access, refreshed.refreshToken);
    // Reintento único con headers reconstruidos por el llamador.
    return attempt();
  }
}
