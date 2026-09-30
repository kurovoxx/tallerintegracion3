import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

/// Sesión activa en memoria y persistencia opcional para Recordarme.
class SessionManager {
  SessionManager._();

  static const _tokenKey = 'auth_token';
  static const _refreshKey = 'refresh_token';
  static const _userKey = 'auth_user';
  static const _rememberMeKey = 'auth_remember_me';
  static String? _token;
  static String? _refreshToken;
  static Map<String, dynamic>? _user;
  static bool _rememberMe = false;

  /// Se invoca cuando la sesión muere definitivamente (refresh inválido).
  /// La app lo asigna al arrancar para navegar a Login (ver main.dart).
  static Future<void> Function()? onSessionExpired;

  static Future<void> saveSession(
    String token,
    Map<String, dynamic>? user, {
    bool rememberMe = false,
    String? refreshToken,
  }) async {
    _token = token;
    _user = user;
    _rememberMe = rememberMe;
    if (refreshToken != null && refreshToken.isNotEmpty) {
      _refreshToken = refreshToken;
    }

    final preferences = await SharedPreferences.getInstance();
    await _clearPersistence(preferences);
    if (rememberMe) {
      await preferences.setString(_tokenKey, token);
      if (_refreshToken != null) {
        await preferences.setString(_refreshKey, _refreshToken!);
      }
      await preferences.setString(_userKey, jsonEncode(user));
      // Se activa al final, cuando ambos datos ya están guardados.
      await preferences.setBool(_rememberMeKey, true);
    }
  }

  static String? get token => _token;
  static String? get refreshToken => _refreshToken;
  static Map<String, dynamic>? get user => _user;

  /// Rota tokens tras un refresh exitoso (respeta el flag rememberMe).
  static Future<void> updateTokens(
    String accessToken,
    String? refreshToken,
  ) async {
    _token = accessToken;
    if (refreshToken != null && refreshToken.isNotEmpty) {
      _refreshToken = refreshToken;
    }
    if (!_rememberMe) return;
    try {
      final preferences = await SharedPreferences.getInstance();
      await preferences.setString(_tokenKey, accessToken);
      if (_refreshToken != null) {
        await preferences.setString(_refreshKey, _refreshToken!);
      }
    } catch (_) {}
  }

  static Future<void> clear() async {
    _token = null;
    _refreshToken = null;
    _user = null;
    _rememberMe = false;
    final preferences = await SharedPreferences.getInstance();
    await _clearPersistence(preferences);
  }

  /// Limpia todo y avisa para navegar a Login con mensaje humano.
  /// Llamar solo cuando el refresh ya falló (sin reintentos aquí).
  static Future<void> expireSession() async {
    await clear();
    final cb = onSessionExpired;
    if (cb != null) {
      try {
        await cb();
      } catch (_) {}
    }
  }

  /// Restaura únicamente sesiones recordadas con un JWT aún vigente.
  /// La firma y la autorización siguen siendo responsabilidad del backend.
  static Future<bool> hasPersistedSession() async {
    final preferences = await SharedPreferences.getInstance();
    try {
      final token = preferences.getString(_tokenKey);
      if (preferences.getBool(_rememberMeKey) == true &&
          token != null &&
          _isTokenValid(token)) {
        final encodedUser = preferences.getString(_userKey);
        final user = encodedUser == null ? null : jsonDecode(encodedUser);
        if (user == null || user is Map<String, dynamic>) {
          _token = token;
          _refreshToken = preferences.getString(_refreshKey);
          _user = user as Map<String, dynamic>?;
          _rememberMe = true;
          return true;
        }
      }
    } on FormatException {
      // Datos incompletos o corruptos: descartar la sesión persistida.
    } on TypeError {
      // También descartar preferencias con tipos inesperados.
    }
    _token = null;
    _refreshToken = null;
    _user = null;
    _rememberMe = false;
    await _clearPersistence(preferences);
    return false;
  }

  static bool _isTokenValid(String token) {
    final parts = token.split('.');
    if (parts.length != 3 || parts.any((part) => part.isEmpty)) return false;
    try {
      final normalized = base64Url.normalize(parts[1]);
      final payload = jsonDecode(utf8.decode(base64Url.decode(normalized)));
      if (payload is! Map<String, dynamic>) return false;
      final expiresAt = payload['exp'];
      final notBefore = payload['nbf'];
      final now = DateTime.now().millisecondsSinceEpoch / 1000;
      return expiresAt is num &&
          expiresAt.isFinite &&
          expiresAt > now &&
          (notBefore == null || (notBefore is num && notBefore <= now));
    } catch (_) {
      return false;
    }
  }

  static Future<void> _clearPersistence(SharedPreferences preferences) async {
    await preferences.remove(_rememberMeKey);
    await preferences.remove(_tokenKey);
    await preferences.remove(_refreshKey);
    await preferences.remove(_userKey);
  }

  static bool get isLoggedIn => _token != null && _token!.isNotEmpty;

  /// user_id del JWT activo (claim `user_id`), solo para scoping LOCAL
  /// (p. ej. aislar notas por cuenta en SQLite). No sustituye la
  /// autorización del backend. Null sin sesión o token ilegible.
  static String? get currentUserId {
    final t = _token;
    if (t == null || t.isEmpty) return null;
    try {
      final parts = t.split('.');
      if (parts.length != 3) return null;
      final payload = jsonDecode(
        utf8.decode(base64Url.decode(base64Url.normalize(parts[1]))),
      );
      if (payload is! Map) return null;
      final id = payload['user_id'] ?? payload['sub'];
      if (id is String && id.trim().isNotEmpty) return id.trim();
      return null;
    } catch (_) {
      return null;
    }
  }
}
