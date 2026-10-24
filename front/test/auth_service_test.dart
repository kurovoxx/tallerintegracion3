import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';

void main() {
  test('login usa el endpoint público y el payload vigente', () async {
    final service = AuthService(
      client: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/auth/login');
        expect(request.headers['content-type'], 'application/json');
        expect(jsonDecode(request.body), {
          'email': 'qa@example.test',
          'password': 'solo-fixture-de-test',
        });
        return http.Response(
          '{"access_token":"access-test","refresh_token":"refresh-test"}',
          200,
        );
      }),
    );
    addTearDown(service.dispose);

    final result = await service.login(
      email: ' qa@example.test ',
      password: 'solo-fixture-de-test',
    );

    expect(result.success, isTrue);
    expect(result.data?['access_token'], 'access-test');
  });

  test('logout envía la pareja al endpoint y es best-effort', () async {
    final service = AuthService(
      client: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/auth/logout');
        expect(jsonDecode(request.body), {'refresh_token': 'refresh-test'});
        return http.Response('', 204);
      }),
    );
    addTearDown(service.dispose);

    await expectLater(service.logout('refresh-test'), completes);
    await expectLater(service.logout(null), completes);
  });

  test('un error de red en logout no lanza al usuario', () async {
    final service = AuthService(
      client: MockClient((_) async => throw StateError('network offline')),
    );
    addTearDown(service.dispose);

    await expectLater(service.logout('refresh-test'), completes);
  });
}
