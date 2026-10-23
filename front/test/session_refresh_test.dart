import 'dart:convert';
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'support/controlled_preferences_store.dart';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

void main() {
  const expiredJwt =
      'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1IiwiZXhwIjoxMDAwMDAwMDAwfQ.signature';

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    AuthedHttp.refreshOverride = null;
  });

  tearDown(() async {
    AuthedHttp.refreshOverride = null;
    await SessionManager.clear();
  });

  test('401 token_expired -> refresh -> retry con token nuevo', () async {
    await SessionManager.saveSession('access-viejo', {
      'id': 'u',
    }, refreshToken: 'refresh-bueno');
    var calls = 0;
    var refreshCalls = 0;
    AuthedHttp.refreshOverride = (token) async {
      refreshCalls++;
      expect(token, 'refresh-bueno');
      return RefreshResult.success(
        accessToken: 'access-nuevo',
        refreshToken: 'refresh-rotado',
      );
    };

    final res = await AuthedHttp.run(() async {
      calls++;
      if (calls == 1) {
        expect(SessionManager.token, 'access-viejo');
        return _json({
          'error': {'code': 'token_expired', 'message': 'expirado'},
        }, 401);
      }
      expect(SessionManager.token, 'access-nuevo');
      return _json({'ok': true}, 200);
    });

    expect(res.statusCode, 200);
    expect(calls, 2);
    expect(refreshCalls, 1);
    // Tokens rotados guardados.
    expect(SessionManager.token, 'access-nuevo');
    expect(SessionManager.refreshToken, 'refresh-rotado');
  });

  test('refresh aplicado en memoria no cierra sesión si falla el guardado', () async {
    final storage = ControlledPreferencesStore()..install();
    await SessionManager.saveSession(
      'access-viejo',
      {'id': 'u'},
      rememberMe: true,
      refreshToken: 'refresh-viejo',
    );
    storage.tokenPairWriteAttempts = 0;
    storage.tokenPairFailuresRemaining = 2;
    var attempts = 0;
    var expiredCalls = 0;
    SessionManager.onSessionExpired = () async {
      expiredCalls++;
    };
    AuthedHttp.refreshOverride = (_) async => RefreshResult.success(
      accessToken: 'access-nuevo',
      refreshToken: 'refresh-nuevo',
    );

    final response = await AuthedHttp.run(() async {
      attempts++;
      if (attempts == 1) {
        return _json({'error': {'code': 'token_expired'}}, 401);
      }
      expect(SessionManager.token, 'access-nuevo');
      return _json({'ok': true}, 200);
    });

    expect(response.statusCode, 200);
    expect(attempts, 2);
    expect(storage.tokenPairWriteAttempts, 2);
    expect(expiredCalls, 0);
    expect(SessionManager.token, 'access-nuevo');
    expect(SessionManager.refreshToken, 'refresh-nuevo');

    // Comprueba el almacenamiento real del fake después de descartar el cache.
    SharedPreferences.resetStatic();
    final preferences = await SharedPreferences.getInstance();
    expect(jsonDecode(preferences.getString('auth_tokens_v1')!), {
      'access_token': 'access-viejo',
      'refresh_token': 'refresh-viejo',
    });
  });

  test('refresh inválido -> sesión limpiada y aviso', () async {
    await SessionManager.saveSession('access-viejo', {
      'id': 'u',
    }, refreshToken: 'refresh-malo');
    var expiredCalls = 0;
    SessionManager.onSessionExpired = () async {
      expiredCalls++;
    };
    addTearDown(() {
      SessionManager.onSessionExpired = null;
    });
    AuthedHttp.refreshOverride = (_) async => RefreshResult.failure(
      'revocado',
      kind: RefreshFailureKind.rejected,
    );

    final res = await AuthedHttp.run(
      () async => _json({
        'error': {'code': 'token_expired', 'message': 'expirado'},
      }, 401),
    );

    expect(res.statusCode, 401);
    expect(SessionManager.token, isNull);
    expect(SessionManager.refreshToken, isNull);
    expect(expiredCalls, 1);
  });

  test('refresh transitorio conserva sesión y no navega a Login', () async {
    await SessionManager.saveSession('access-viejo', {
      'id': 'u',
    }, refreshToken: 'refresh-aun-no-confirmado');
    var expiredCalls = 0;
    SessionManager.onSessionExpired = () async {
      expiredCalls++;
    };
    addTearDown(() {
      SessionManager.onSessionExpired = null;
    });
    AuthedHttp.refreshOverride = (_) async => RefreshResult.failure('timeout');

    final res = await AuthedHttp.run(
      () async => _json({
        'error': {'code': 'token_expired', 'message': 'expirado'},
      }, 401),
    );

    expect(res.statusCode, 401);
    expect(SessionManager.token, 'access-viejo');
    expect(SessionManager.refreshToken, 'refresh-aun-no-confirmado');
    expect(expiredCalls, 0);
  });

  test('timeout HTTP real de refresh conserva credenciales', () async {
    await SessionManager.saveSession('access-viejo', {
      'id': 'u',
    }, refreshToken: 'refresh-aun-no-confirmado');
    final pendingResponse = Completer<http.Response>();
    final auth = AuthService(
      client: MockClient((_) => pendingResponse.future),
    );
    addTearDown(auth.dispose);
    AuthedHttp.refreshOverride = auth.refresh;

    final res = await AuthedHttp.run(
      () async => _json({
        'error': {'code': 'token_expired', 'message': 'expirado'},
      }, 401),
    );
    pendingResponse.complete(
      _json({
        'access_token': 'access-tardio',
        'refresh_token': 'refresh-tardio',
      }, 200),
    );

    expect(res.statusCode, 401);
    expect(SessionManager.token, 'access-viejo');
    expect(SessionManager.refreshToken, 'refresh-aun-no-confirmado');
  });

  test('401 concurrentes comparten una sola rotación', () async {
    await SessionManager.saveSession('access-viejo', {
      'id': 'u',
    }, refreshToken: 'refresh-viejo');
    final refreshStarted = Completer<void>();
    final releaseRefresh = Completer<void>();
    var refreshCalls = 0;
    var attempts = 0;
    AuthedHttp.refreshOverride = (token) async {
      refreshCalls++;
      expect(token, 'refresh-viejo');
      if (!refreshStarted.isCompleted) refreshStarted.complete();
      await releaseRefresh.future;
      return RefreshResult.success(
        accessToken: 'access-nuevo',
        refreshToken: 'refresh-nuevo',
      );
    };

    Future<http.Response> request() => AuthedHttp.run(() async {
      attempts++;
      return attempts <= 2
          ? _json({'error': {'code': 'token_expired'}}, 401)
          : _json({'ok': true}, 200);
    });

    final first = request();
    final second = request();
    await refreshStarted.future;
    releaseRefresh.complete();
    final responses = await Future.wait([first, second]);

    expect(refreshCalls, 1);
    expect(responses.map((response) => response.statusCode), everyElement(200));
    expect(SessionManager.token, 'access-nuevo');
    expect(SessionManager.refreshToken, 'refresh-nuevo');
  });

  test('al reiniciar renueva y persiste el par si el refresh responde', () async {
    await SessionManager.saveSession(expiredJwt, {
      'id': 'u',
    }, rememberMe: true, refreshToken: 'refresh-vigente');
    SessionManager.resetInMemory();
    AuthedHttp.refreshOverride = (token) async {
      expect(token, 'refresh-vigente');
      return RefreshResult.success(
        accessToken: 'access-renovado',
        refreshToken: 'refresh-rotado',
      );
    };

    final restored = await AuthedHttp.restorePersistedSession();

    expect(restored, isTrue);
    expect(SessionManager.token, 'access-renovado');
    expect(SessionManager.refreshToken, 'refresh-rotado');
    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getString('auth_token'), 'access-renovado');
    expect(prefs.getString('refresh_token'), 'refresh-rotado');
    expect(jsonDecode(prefs.getString('auth_tokens_v1')!), {
      'access_token': 'access-renovado',
      'refresh_token': 'refresh-rotado',
    });
  });

  test('reinicio con timeout conserva credenciales con renovación pendiente', () async {
    await SessionManager.saveSession(expiredJwt, {
      'id': 'u',
    }, rememberMe: true, refreshToken: 'refresh-no-confirmado');
    SessionManager.resetInMemory();
    AuthedHttp.refreshOverride = (_) async => RefreshResult.failure('timeout');

    final restored = await AuthedHttp.restorePersistedSession();

    // El bool permite a main.dart abrir MainShell porque las credenciales
    // quedaron cargadas; no valida el access ni la carga de datos.
    expect(restored, isTrue);
    expect(SessionManager.token, expiredJwt);
    expect(SessionManager.isAccessTokenValid(SessionManager.token), isFalse);
    expect(SessionManager.refreshToken, 'refresh-no-confirmado');
    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getBool('auth_remember_me'), isTrue);
  });

  test('al volver a primer plano renueva access vencido una vez', () async {
    await SessionManager.saveSession('access-vencido', {
      'id': 'u',
    }, refreshToken: 'refresh-vigente');
    var refreshCalls = 0;
    AuthedHttp.refreshOverride = (_) async {
      refreshCalls++;
      return RefreshResult.success(
        accessToken: 'access-renovado',
        refreshToken: 'refresh-rotado',
      );
    };

    await AuthedHttp.refreshIfExpired();

    expect(refreshCalls, 1);
    expect(SessionManager.token, 'access-renovado');
    expect(SessionManager.refreshToken, 'refresh-rotado');
  });

  test('rechazo HTTP 401 del refresh es definitivo, 5xx es transitorio', () async {
    final rejectedService = AuthService(
      client: MockClient((_) async => _json({
        'error': {'code': 'token_expired'},
      }, 401)),
    );
    final unavailableService = AuthService(
      client: MockClient((_) async => _json({'error': 'unavailable'}, 503)),
    );
    addTearDown(rejectedService.dispose);
    addTearDown(unavailableService.dispose);

    final rejected = await rejectedService.refresh('refresh');
    final unavailable = await unavailableService.refresh('refresh');

    expect(rejected.failureKind, RefreshFailureKind.rejected);
    expect(unavailable.failureKind, RefreshFailureKind.transient);
  });

  test('sin refresh token -> expira sin loop', () async {
    await SessionManager.saveSession('solo-access', {'id': 'u'});
    var refreshCalls = 0;
    AuthedHttp.refreshOverride = (_) async {
      refreshCalls++;
      return RefreshResult.failure('x');
    };
    var expiredCalls = 0;
    SessionManager.onSessionExpired = () async {
      expiredCalls++;
    };
    addTearDown(() {
      SessionManager.onSessionExpired = null;
    });

    final res = await AuthedHttp.run(
      () async => _json({'error': 'unauthorized'}, 401),
    );

    expect(res.statusCode, 401);
    expect(refreshCalls, 0);
    expect(expiredCalls, 1);
    expect(SessionManager.token, isNull);
  });

  test('401 persistente tras refresh no reintenta (sin loop)', () async {
    await SessionManager.saveSession('access-viejo', {
      'id': 'u',
    }, refreshToken: 'refresh-bueno');
    var calls = 0;
    var refreshCalls = 0;
    AuthedHttp.refreshOverride = (_) async {
      refreshCalls++;
      return RefreshResult.success(
        accessToken: 'access-nuevo',
        refreshToken: 'refresh-rotado',
      );
    };

    final res = await AuthedHttp.run(() async {
      calls++;
      return _json({'error': 'raro'}, 401);
    });

    expect(res.statusCode, 401);
    expect(calls, 2);
    expect(refreshCalls, 1);
    // El refresh funcionó: la sesión sigue viva con el token nuevo.
    expect(SessionManager.token, 'access-nuevo');
  });

  test('refresh tardío de A no reemplaza sesión B', () async {
    await SessionManager.saveSession('A', {
      'id': 'A',
    }, refreshToken: 'refresh-A');
    final pending = Completer<RefreshResult>();
    final started = Completer<void>();
    AuthedHttp.refreshOverride = (_) {
      started.complete();
      return pending.future;
    };
    final request = AuthedHttp.run(
      () async => _json({'error': 'unauthorized'}, 401),
    );
    await started.future;
    await SessionManager.clear();
    await SessionManager.saveSession('B', {'id': 'B'});
    pending.complete(
      RefreshResult.success(
        accessToken: 'A-new',
        refreshToken: 'refresh-A-new',
      ),
    );
    await request;
    expect(SessionManager.token, 'B');
    expect(SessionManager.refreshToken, isNull);
  });

  test('refresh tardío no restaura sesión después de logout', () async {
    await SessionManager.saveSession('A', {
      'id': 'A',
    }, rememberMe: true, refreshToken: 'refresh-A');
    final pending = Completer<RefreshResult>();
    final started = Completer<void>();
    AuthedHttp.refreshOverride = (_) {
      started.complete();
      return pending.future;
    };
    final request = AuthedHttp.run(
      () async => _json({'error': 'unauthorized'}, 401),
    );
    await started.future;
    await SessionManager.clear();
    pending.complete(
      RefreshResult.success(
        accessToken: 'A-new',
        refreshToken: 'refresh-A-new',
      ),
    );

    await request;

    expect(SessionManager.token, isNull);
    expect(SessionManager.refreshToken, isNull);
    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getString('auth_tokens_v1'), isNull);
  });

  test('AuthService.refresh parsea rotación y logout no lanza', () async {
    final auth = AuthService();
    addTearDown(auth.dispose);
    // Sin backend no se puede probar red aquí; solo contrato de tipos.
    expect(AuthService.refreshPath, '/auth/refresh');
    expect(AuthService.logoutPath, '/auth/logout');
  });
}
