import 'package:flutter/foundation.dart';

/// Almacén simple en memoria para JWT entre pantallas.
/// En producción se usaría flutter_secure_storage / shared_preferences.
class SessionManager {
  SessionManager._();
  static String? _token;
  static Map<String, dynamic>? _user;

  static void saveSession(String token, Map<String, dynamic>? user) {
    _token = token;
    _user = user;
    debugPrint('SessionManager: token guardado (${token.substring(0, 8)}...)');
  }

  static String? get token => _token;
  static Map<String, dynamic>? get user => _user;

  static void clear() {
    _token = null;
    _user = null;
  }

  static bool get isLoggedIn => _token != null && _token!.isNotEmpty;
}
