import 'dart:convert';

import 'package:http/http.dart' as http;

import 'auth_service.dart';
import 'session_manager.dart';

/// Reintento autenticado: como máximo un refresh y un reintento por petición.
class AuthedHttp {
  AuthedHttp._();

  /// Refrescador inyectable para pruebas.
  static Future<RefreshResult> Function(String refreshToken)? refreshOverride;
  static Future<_RefreshOutcome>? _refreshInFlight;
  static int? _refreshInFlightRevision;
  static String? _refreshInFlightToken;

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

  /// Carga las credenciales de Recordarme e intenta renovar si el access venció.
  ///
  /// `true` indica que las credenciales siguen en memoria. Si el refresh falla
  /// temporalmente, puede abrirse MainShell con access vencido; eso no confirma
  /// que el servidor acepte la sesión ni que puedan cargarse datos.
  static Future<bool> restorePersistedSession() async {
    if (!await SessionManager.hasPersistedSession()) return false;
    await refreshIfExpired();
    return SessionManager.isLoggedIn;
  }

  /// Se llama al volver a primer plano. No renueva access que todavía sirve.
  static Future<void> refreshIfExpired() async {
    final access = SessionManager.token;
    final refresh = SessionManager.refreshToken;
    if (SessionManager.isAccessTokenValid(access)) return;
    if ((access == null || access.isEmpty) &&
        (refresh == null || refresh.isEmpty)) {
      return;
    }

    final sessionRevision = SessionManager.revision.value;
    if (refresh == null || refresh.isEmpty) {
      await SessionManager.expireSession(
        expectedRevision: sessionRevision,
        expectedRefreshToken: null,
        checkRefreshToken: true,
      );
      return;
    }
    await _refreshSession(sessionRevision, refresh);
  }

  static Future<http.Response> run(
    Future<http.Response> Function() attempt,
  ) async {
    final sessionRevision = SessionManager.revision.value;
    final accessAtStart = SessionManager.token;
    final first = await attempt();
    if (sessionRevision != SessionManager.revision.value) return first;
    if (!_isExpired(first)) return first;

    // Another request may already have rotated the token while this 401 was
    // travelling back. Retry with that token instead of rotating it again.
    if (SessionManager.token != accessAtStart) return attempt();

    final refresh = SessionManager.refreshToken;
    if (refresh == null || refresh.isEmpty) {
      await SessionManager.expireSession(
        expectedRevision: sessionRevision,
        expectedRefreshToken: refresh,
        checkRefreshToken: true,
      );
      return first;
    }

    final outcome = await _refreshSession(sessionRevision, refresh);
    if (outcome != _RefreshOutcome.refreshed ||
        sessionRevision != SessionManager.revision.value) {
      return first;
    }
    // Headers are rebuilt by the caller using the current access token.
    return attempt();
  }

  static Future<_RefreshOutcome> _refreshSession(
    int sessionRevision,
    String refreshToken,
  ) {
    final existing = _refreshInFlight;
    if (existing != null &&
        _refreshInFlightRevision == sessionRevision &&
        _refreshInFlightToken == refreshToken) {
      return existing;
    }

    late final Future<_RefreshOutcome> operation;
    operation = _performRefresh(sessionRevision, refreshToken).whenComplete(() {
      if (identical(_refreshInFlight, operation)) {
        _refreshInFlight = null;
        _refreshInFlightRevision = null;
        _refreshInFlightToken = null;
      }
    });
    _refreshInFlight = operation;
    _refreshInFlightRevision = sessionRevision;
    _refreshInFlightToken = refreshToken;
    return operation;
  }

  static Future<_RefreshOutcome> _performRefresh(
    int sessionRevision,
    String refreshToken,
  ) async {
    if (sessionRevision != SessionManager.revision.value ||
        SessionManager.refreshToken != refreshToken) {
      return _RefreshOutcome.stale;
    }

    late final RefreshResult refreshed;
    try {
      refreshed = await _doRefresh(refreshToken);
    } catch (_) {
      return _RefreshOutcome.transient;
    }

    if (sessionRevision != SessionManager.revision.value ||
        SessionManager.refreshToken != refreshToken) {
      return _RefreshOutcome.stale;
    }
    if (!refreshed.success) {
      if (refreshed.failureKind == RefreshFailureKind.rejected) {
        await SessionManager.expireSession(
          expectedRevision: sessionRevision,
          expectedRefreshToken: refreshToken,
          checkRefreshToken: true,
        );
        return _RefreshOutcome.rejected;
      }
      return _RefreshOutcome.transient;
    }

    final access = refreshed.accessToken;
    final rotatedRefresh = refreshed.refreshToken;
    if (access == null ||
        access.isEmpty ||
        rotatedRefresh == null ||
        rotatedRefresh.isEmpty) {
      return _RefreshOutcome.transient;
    }

    final updateResult = await SessionManager.updateTokens(
      access,
      rotatedRefresh,
      expectedRevision: sessionRevision,
      expectedRefreshToken: refreshToken,
    );
    return switch (updateResult) {
      TokenUpdateResult.rotationAppliedAndPersisted ||
      TokenUpdateResult.rotationAppliedInMemoryOnly ||
      TokenUpdateResult.rotationAppliedInMemoryPersistenceFailed =>
        _RefreshOutcome.refreshed,
      TokenUpdateResult.staleSession => _RefreshOutcome.stale,
      TokenUpdateResult.invalidRotation => _RefreshOutcome.transient,
    };
  }
}

enum _RefreshOutcome { refreshed, rejected, transient, stale }
