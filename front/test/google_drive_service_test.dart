import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/auth/google_drive_service.dart';

void main() {
  group('GoogleDriveService.extractOAuthCode', () {
    test('código crudo se conserva', () {
      expect(
        GoogleDriveService.extractOAuthCode('4/0AabcDEF-123'),
        '4/0AabcDEF-123',
      );
    });

    test('URL completa con code sin codificar', () {
      expect(
        GoogleDriveService.extractOAuthCode(
          'http://localhost:8081/auth/google/callback?code=4/0Aabc&scope=email',
        ),
        '4/0Aabc',
      );
    });

    test('URL completa con code URL-encoded se decodifica', () {
      expect(
        GoogleDriveService.extractOAuthCode(
          'http://localhost:8081/auth/google/callback?code=4%2F0Axyz-_.&scope=email',
        ),
        '4/0Axyz-_.',
      );
    });

    test('query code=...', () {
      expect(
        GoogleDriveService.extractOAuthCode('code=ABC123&scope=email'),
        'ABC123',
      );
    });

    test('retorno con error= no produce código', () {
      expect(
        GoogleDriveService.extractOAuthCode(
          'http://localhost:8081/auth/google/callback?error=access_denied',
        ),
        '',
      );
    });

    test('vacío devuelve vacío', () {
      expect(GoogleDriveService.extractOAuthCode('   '), '');
    });
  });

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    AuthedHttp.refreshOverride = null;
  });

  tearDown(() async {
    AuthedHttp.refreshOverride = null;
    await SessionManager.clear();
  });

  test('disconnect renueva una vez y solo confirma con HTTP 204', () async {
    await SessionManager.saveSession(
      'access-viejo',
      {'id': 'user'},
      rememberMe: true,
      refreshToken: 'refresh-vigente',
    );
    AuthedHttp.refreshOverride = (token) async {
      expect(token, 'refresh-vigente');
      return RefreshResult.success(
        accessToken: 'access-rotado',
        refreshToken: 'refresh-rotado',
      );
    };
    final authorization = <String?>[];
    var requests = 0;
    final client = MockClient((request) async {
      expect(request.method, 'DELETE');
      expect(request.url.path, '/auth/google-drive/connection');
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
      return http.Response('', 204);
    });
    final result = await GoogleDriveService(client: client).disconnectDrive(
      backendBaseUrl: 'https://auth.example.test',
      appAccessToken: 'access-viejo',
    );

    expect(result.confirmed, isTrue);
    expect(authorization, ['Bearer access-viejo', 'Bearer access-rotado']);
    final prefs = await SharedPreferences.getInstance();
    final persisted = jsonDecode(prefs.getString('auth_tokens_v1')!) as Map;
    expect(persisted['access_token'], 'access-rotado');
    expect(persisted['refresh_token'], 'refresh-rotado');
    client.close();
  });

  test('fallo HTTP no confirma disconnect ni borra sesión', () async {
    await SessionManager.saveSession(
      'access-activo',
      {'id': 'user'},
      rememberMe: true,
      refreshToken: 'refresh-activo',
    );
    final client = MockClient((_) async => http.Response('', 503));

    final result = await GoogleDriveService(client: client).disconnectDrive(
      backendBaseUrl: 'https://auth.example.test',
      appAccessToken: 'access-activo',
    );

    expect(result.confirmed, isFalse);
    expect(SessionManager.isLoggedIn, isTrue);
    expect(SessionManager.token, 'access-activo');
    expect(SessionManager.refreshToken, 'refresh-activo');
    client.close();
  });

  test('respuesta tardía de otra sesión no confirma la desconexión', () async {
    await SessionManager.saveSession('access-A', {'id': 'A'});
    final requestStarted = Completer<void>();
    final response = Completer<http.Response>();
    final authorization = <String?>[];
    final client = MockClient((request) {
      authorization.add(request.headers['authorization']);
      requestStarted.complete();
      return response.future;
    });
    final pending = GoogleDriveService(client: client).disconnectDrive(
      backendBaseUrl: 'https://auth.example.test',
      appAccessToken: 'access-A',
    );
    await requestStarted.future;
    await SessionManager.clear();
    await SessionManager.saveSession('access-B', {'id': 'B'});
    response.complete(http.Response('', 204));

    final result = await pending;

    expect(result.confirmed, isFalse);
    expect(authorization, ['Bearer access-A']);
    expect(SessionManager.token, 'access-B');
    client.close();
  });
}
