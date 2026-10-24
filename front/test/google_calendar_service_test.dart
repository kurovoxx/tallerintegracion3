import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/auth/google_calendar_service.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    AuthedHttp.refreshOverride = null;
  });

  tearDown(() async {
    AuthedHttp.refreshOverride = null;
    await SessionManager.clear();
  });

  test(
    'un error HTTP de status no se presenta como Calendar desconectado',
    () async {
      await SessionManager.saveSession('access-activo', {'id': 'user'});
      final service = GoogleCalendarService(
        client: MockClient((request) async {
          expect(request.method, 'GET');
          expect(request.url.path, '/auth/google-calendar/status');
          expect(request.headers['authorization'], 'Bearer access-activo');
          return http.Response('{"error":"unavailable"}', 503);
        }),
      );

      final status = await service.fetchStatus(
        backendBaseUrl: 'https://auth.example.test',
      );

      expect(status, isNull);
      expect(SessionManager.isLoggedIn, isTrue);
      expect(SessionManager.token, 'access-activo');
    },
  );

  test(
    'status reintenta con el JWT rotado y devuelve el estado confirmado',
    () async {
      final authorization = <String?>[];
      var requests = 0;
      AuthedHttp.refreshOverride = (token) async {
        expect(token, 'refresh-vigente');
        return RefreshResult.success(
          accessToken: 'access-rotado',
          refreshToken: 'refresh-rotado',
        );
      };
      await SessionManager.saveSession('access-viejo', {
        'id': 'user',
      }, refreshToken: 'refresh-vigente');
      final service = GoogleCalendarService(
        client: MockClient((request) async {
          authorization.add(request.headers['authorization']);
          requests++;
          if (requests == 1) {
            return http.Response(
              jsonEncode({
                'error': {'code': 'token_expired'},
              }),
              401,
            );
          }
          return http.Response('{"connected":true}', 200);
        }),
      );

      final status = await service.fetchStatus(
        backendBaseUrl: 'https://auth.example.test',
      );

      expect(authorization, ['Bearer access-viejo', 'Bearer access-rotado']);
      expect(status?.connected, isTrue);
    },
  );

  test(
    'connect usa el endpoint real, JWT vigente y confirma el JSON',
    () async {
      await SessionManager.saveSession('access-activo', {'id': 'user'});
      final service = GoogleCalendarService(
        client: MockClient((request) async {
          expect(request.method, 'POST');
          expect(request.url.path, '/auth/google-calendar/connect');
          expect(request.headers['authorization'], 'Bearer access-activo');
          expect(jsonDecode(request.body), {'oauth_code': 'codigo-de-prueba'});
          return http.Response('{"connected":true}', 200);
        }),
      );

      final result = await service.connectCalendar(
        backendBaseUrl: 'https://auth.example.test',
        oauthCode: 'codigo-de-prueba',
      );

      expect(result.ok, isTrue);
    },
  );
}
