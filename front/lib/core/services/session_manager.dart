import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

enum TokenUpdateResult {
  rotationAppliedAndPersisted,
  rotationAppliedInMemoryOnly,
  rotationAppliedInMemoryPersistenceFailed,
  staleSession,
  invalidRotation,
}

enum _RotationPersistenceResult { persisted, failed, staleSession }

/// Sesión activa en memoria y persistencia opcional para Recordarme.
class SessionManager {
  SessionManager._();

  static const _tokenKey = 'auth_token';
  static const _refreshKey = 'refresh_token';
  static const _tokensKey = 'auth_tokens_v1';
  static const _userKey = 'auth_user';
  static const _rememberMeKey = 'auth_remember_me';
  static String? _token;
  static String? _refreshToken;
  static Map<String, dynamic>? _user;
  static bool _rememberMe = false;
  static Future<void> _persistenceQueue = Future<void>.value();

  /// Cambia cuando cambia la identidad de sesión, no cuando rota un token.
  static final ValueNotifier<int> revision = ValueNotifier(0);

  /// Reloj reemplazable en pruebas de expiración.
  @visibleForTesting
  static DateTime Function()? clockOverride;

  /// Se invoca únicamente cuando el servidor rechaza el refresh.
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
    _refreshToken = refreshToken;
    revision.value++;
    final sessionRevision = revision.value;

    await _serializePersistence(() async {
      if (revision.value != sessionRevision) return;
      final preferences = await SharedPreferences.getInstance();
      if (revision.value != sessionRevision) return;
      await _clearPersistence(preferences);
      if (!rememberMe) return;

      // El par queda disponible en una sola escritura. Las claves separadas
      // siguen actualizándose para poder leer instalaciones anteriores.
      await preferences.setString(
        _tokensKey,
        jsonEncode({'access_token': token, 'refresh_token': refreshToken}),
      );
      await preferences.setString(_tokenKey, token);
      if (refreshToken == null || refreshToken.isEmpty) {
        await preferences.remove(_refreshKey);
      } else {
        await preferences.setString(_refreshKey, refreshToken);
      }
      if (user != null) {
        await preferences.setString(_userKey, jsonEncode(user));
      }
      await preferences.setBool(_rememberMeKey, true);
    });
  }

  static String? get token => _token;
  static String? get refreshToken => _refreshToken;
  static Map<String, dynamic>? get user => _user;
  static bool get rememberMe => _rememberMe;

  /// Aplica la rotación en memoria antes de intentar persistirla.
  ///
  /// El guardado se intenta como máximo dos veces. Un fallo deja vigentes en
  /// memoria los tokens nuevos, ya que el servidor pudo invalidar el refresh
  /// anterior. La revisión evita guardar una rotación sobre otra sesión.
  static Future<TokenUpdateResult> updateTokens(
    String accessToken,
    String? refreshToken, {
    required int expectedRevision,
    required String? expectedRefreshToken,
  }) async {
    if (!_matches(expectedRevision, expectedRefreshToken)) {
      return TokenUpdateResult.staleSession;
    }
    final nextRefresh = refreshToken?.isNotEmpty == true
        ? refreshToken
        : _refreshToken;
    if (nextRefresh == null || nextRefresh.isEmpty) {
      return TokenUpdateResult.invalidRotation;
    }

    _token = accessToken;
    _refreshToken = nextRefresh;
    bool isStillThisRotation() =>
        revision.value == expectedRevision &&
        _token == accessToken &&
        _refreshToken == nextRefresh;

    if (!_rememberMe) return TokenUpdateResult.rotationAppliedInMemoryOnly;

    for (var attempt = 0; attempt < 2; attempt++) {
      if (!isStillThisRotation()) return TokenUpdateResult.staleSession;

      var persistenceResult = _RotationPersistenceResult.failed;
      try {
        persistenceResult = await _serializePersistence(() async {
          if (!isStillThisRotation()) {
            return _RotationPersistenceResult.staleSession;
          }
          final preferences = await SharedPreferences.getInstance();
          if (!isStillThisRotation()) {
            return _RotationPersistenceResult.staleSession;
          }
          final pairSaved = await preferences.setString(
            _tokensKey,
            jsonEncode({
              'access_token': accessToken,
              'refresh_token': nextRefresh,
            }),
          );
          if (!isStillThisRotation()) {
            return _RotationPersistenceResult.staleSession;
          }
          final accessSaved = await preferences.setString(
            _tokenKey,
            accessToken,
          );
          if (!isStillThisRotation()) {
            return _RotationPersistenceResult.staleSession;
          }
          final refreshSaved = await preferences.setString(
            _refreshKey,
            nextRefresh,
          );
          if (!isStillThisRotation()) {
            return _RotationPersistenceResult.staleSession;
          }
          return pairSaved && accessSaved && refreshSaved
              ? _RotationPersistenceResult.persisted
              : _RotationPersistenceResult.failed;
        });
      } catch (_) {
        persistenceResult = _RotationPersistenceResult.failed;
      }

      if (persistenceResult == _RotationPersistenceResult.persisted) {
        return TokenUpdateResult.rotationAppliedAndPersisted;
      }
      if (persistenceResult == _RotationPersistenceResult.staleSession) {
        return TokenUpdateResult.staleSession;
      }
    }

    return isStillThisRotation()
        ? TokenUpdateResult.rotationAppliedInMemoryPersistenceFailed
        : TokenUpdateResult.staleSession;
  }

  static Future<void> clear() async {
    _token = null;
    _refreshToken = null;
    _user = null;
    _rememberMe = false;
    revision.value++;
    await _serializePersistence(() async {
      final preferences = await SharedPreferences.getInstance();
      await _clearPersistence(preferences);
    });
  }

  /// Limpia y avisa solo si la respuesta corresponde a la sesión actual.
  static Future<void> expireSession({
    int? expectedRevision,
    String? expectedRefreshToken,
    bool checkRefreshToken = false,
  }) async {
    if (!_matches(
      expectedRevision,
      expectedRefreshToken,
      checkRefreshToken: checkRefreshToken,
    )) {
      return;
    }
    final clearing = clear();
    final clearedRevision = revision.value;
    await clearing;
    if (revision.value != clearedRevision ||
        _token != null ||
        _refreshToken != null ||
        _rememberMe) {
      return;
    }
    final cb = onSessionExpired;
    if (cb != null) {
      try {
        await cb();
      } catch (_) {}
    }
  }

  /// Restaura Recordarme aunque el access haya vencido si hay refresh para
  /// intentar una renovación. El servidor determina si ese refresh sigue vivo.
  static Future<bool> hasPersistedSession() async {
    final preferences = await SharedPreferences.getInstance();
    try {
      final remembered = preferences.getBool(_rememberMeKey) == true;
      String? token;
      String? refreshToken;
      final tokenPair = preferences.getString(_tokensKey);
      if (tokenPair != null) {
        final decoded = jsonDecode(tokenPair);
        if (decoded is! Map<String, dynamic>) {
          throw const FormatException('Invalid persisted token pair');
        }
        final accessValue = decoded['access_token'];
        final refreshValue = decoded['refresh_token'];
        if ((accessValue != null && accessValue is! String) ||
            (refreshValue != null && refreshValue is! String)) {
          throw const FormatException('Invalid persisted token values');
        }
        token = accessValue as String?;
        refreshToken = refreshValue as String?;
      } else {
        // Compatibilidad con sesiones escritas por versiones anteriores.
        token = preferences.getString(_tokenKey);
        refreshToken = preferences.getString(_refreshKey);
      }

      if (remembered &&
          ((_isTokenValid(token) || (refreshToken?.isNotEmpty ?? false)))) {
        final encodedUser = preferences.getString(_userKey);
        final user = encodedUser == null ? null : jsonDecode(encodedUser);
        if (user == null || user is Map<String, dynamic>) {
          _token = token;
          _refreshToken = refreshToken;
          _user = user as Map<String, dynamic>?;
          _rememberMe = true;
          revision.value++;
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
    revision.value++;
    await _serializePersistence(() => _clearPersistence(preferences));
    return false;
  }

  static bool isAccessTokenValid(String? token) => _isTokenValid(token);

  static bool _isTokenValid(String? token) {
    if (token == null || token.isEmpty) return false;
    final parts = token.split('.');
    if (parts.length != 3 || parts.any((part) => part.isEmpty)) return false;
    try {
      final normalized = base64Url.normalize(parts[1]);
      final payload = jsonDecode(utf8.decode(base64Url.decode(normalized)));
      if (payload is! Map<String, dynamic>) return false;
      final expiresAt = payload['exp'];
      final notBefore = payload['nbf'];
      final now = (clockOverride?.call() ?? DateTime.now())
              .millisecondsSinceEpoch /
          1000;
      return expiresAt is num &&
          expiresAt.isFinite &&
          expiresAt > now &&
          (notBefore == null || (notBefore is num && notBefore <= now));
    } catch (_) {
      return false;
    }
  }

  static bool _matches(
    int? expectedRevision,
    String? expectedRefreshToken, {
    bool checkRefreshToken = true,
  }) =>
      (expectedRevision == null || revision.value == expectedRevision) &&
      (!checkRefreshToken || _refreshToken == expectedRefreshToken);

  static Future<void> _clearPersistence(
    SharedPreferences preferences,
  ) async {
    await preferences.remove(_rememberMeKey);
    await preferences.remove(_tokensKey);
    await preferences.remove(_tokenKey);
    await preferences.remove(_refreshKey);
    await preferences.remove(_userKey);
  }

  static Future<T> _serializePersistence<T>(Future<T> Function() action) {
    final result = _persistenceQueue.then((_) => action());
    _persistenceQueue = result.then<void>((_) {}).catchError((Object _) {});
    return result;
  }

  @visibleForTesting
  static void resetInMemory() {
    _token = null;
    _refreshToken = null;
    _user = null;
    _rememberMe = false;
    revision.value++;
  }

  static bool get isLoggedIn => _token != null && _token!.isNotEmpty;

  /// user_id del JWT activo (claim `user_id`), solo para scoping LOCAL.
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
