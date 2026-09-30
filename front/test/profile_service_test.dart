import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('getProfile parsea solo los 6 campos reales', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = ProfileService(
      client: MockClient((request) async {
        expect(request.url.path, '/profile/me');
        return http.Response(
          jsonEncode(<String, dynamic>{
            'display_name': 'Ana',
            'photo_url': null,
            'phone': '+569',
            'institution': 'UCT',
            'description': 'Hola',
            'visibility': 'public',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final p = await service.getProfile();
    expect(p.displayName, 'Ana');
    expect(p.institution, 'UCT');
    expect(p.visibility, 'public');
  });

  test('getProfile incluye email cuando el backend lo entrega', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = ProfileService(
      client: MockClient((request) async {
        return http.Response(
          jsonEncode(<String, dynamic>{
            'display_name': 'Agustín Vega',
            'visibility': 'public',
            'email': 'agustin@example.com',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final p = await service.getProfile();
    expect(p.email, 'agustin@example.com');
  });

  test('patchProfile envía parcial sin user_id', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = ProfileService(
      client: MockClient((request) async {
        expect(request.method, 'PATCH');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body.containsKey('user_id'), isFalse);
        expect(body['display_name'], 'Ana B');
        return http.Response(
          jsonEncode(<String, dynamic>{
            'display_name': 'Ana B',
            'photo_url': null,
            'phone': null,
            'institution': 'UCT',
            'description': 'Hola',
            'visibility': 'private',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final updated = await service.patchProfile(<String, dynamic>{
      'display_name': 'Ana B',
      'visibility': 'private',
    });
    expect(updated.displayName, 'Ana B');
    expect(updated.visibility, 'private');
  });
}
