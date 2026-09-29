import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

/// Sesión activa en memoria y persistencia opcional para Recordarme.
class SessionManager {
  SessionManager._();

  static const _tokenKey = 'auth_token';
  static const _userKey = 'auth_user';
  static const _rememberMeKey = 'auth_remember_me';
  static String? _token;
  static Map<String, dynamic>? _user;

  static Future<void> saveSession(
    String token,
    Map<String, dynamic>? user, {
    bool rememberMe = false,
  }) async {
    _token = token;
    _user = user;

    final preferences = await SharedPreferences.getInstance();
    await _clearPersistence(preferences);
    if (rememberMe) {
      await preferences.setString(_tokenKey, token);
      await preferences.setString(_userKey, jsonEncode(user));
      // Se activa al final, cuando ambos datos ya están guardados.
      await preferences.setBool(_rememberMeKey, true);
    }
  }

  static String? get token => _token;
  static Map<String, dynamic>? get user => _user;

  static Future<void> clear() async {
    _token = null;
    _user = null;
    final preferences = await SharedPreferences.getInstance();
    await _clearPersistence(preferences);
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
          _user = user as Map<String, dynamic>?;
          return true;
        }
      }
    } on FormatException {
      // Datos incompletos o corruptos: descartar la sesión persistida.
    } on TypeError {
      // También descartar preferencias con tipos inesperados.
    }
    _token = null;
    _user = null;
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
    await preferences.remove(_userKey);
  }

  static bool get isLoggedIn => _token != null && _token!.isNotEmpty;
}
