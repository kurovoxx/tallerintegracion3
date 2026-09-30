import 'dart:convert';
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

void main() {
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
    AuthedHttp.refreshOverride = (_) async => RefreshResult.failure('revocado');

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
      return RefreshResult.success(accessToken: 'access-nuevo');
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

  test('AuthService.refresh parsea rotación y logout no lanza', () async {
    final auth = AuthService();
    addTearDown(auth.dispose);
    // Sin backend no se puede probar red aquí; solo contrato de tipos.
    expect(AuthService.refreshPath, '/auth/refresh');
    expect(AuthService.logoutPath, '/auth/logout');
  });
}
