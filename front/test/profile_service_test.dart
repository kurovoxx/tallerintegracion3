import 'support/service_uri.dart';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/profile_models.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('getProfile parsea los campos reales y la miniatura', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = ProfileService(
      client: MockClient((request) async {
        expect(request.url.servicePath, '/profile/me');
        return http.Response(
          jsonEncode(<String, dynamic>{
            'display_name': 'Ana',
            'photo_url': null,
            'photo_thumb_url': 'data:image/png;base64,thumb',
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
    expect(p.photoThumbUrl, 'data:image/png;base64,thumb');
  });

  test('getProfile parsea public_id sin exponer el UUID', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = ProfileService(
      client: MockClient((request) async {
        return http.Response(
          jsonEncode(<String, dynamic>{
            'display_name': 'Ana',
            'visibility': 'public',
            'public_id': 'a8kp92mqwx7r',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final p = await service.getProfile();
    expect(p.publicId, 'a8kp92mqwx7r');
  });

  test(
    'getPublicProfile resuelve por public_id y lee nombre e institución',
    () async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = ProfileService(
        client: MockClient((request) async {
          expect(request.url.servicePath, '/profile/public/a8kp92mqwx7r');
          expect(request.headers['Authorization'], 'Bearer jwt-test');
          return http.Response(
            jsonEncode(<String, dynamic>{
              'public_id': 'a8kp92mqwx7r',
              'display_name': 'Ana',
              'photo_url': null,
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

      final p = await service.getPublicProfile('  A8KP92MQWX7R ');
      expect(p.publicId, 'a8kp92mqwx7r');
      expect(p.displayName, 'Ana');
      expect(p.institution, 'UCT');
    },
  );

  test('normalizePublicId solo acepta el ID crudo de 12 caracteres', () {
    expect(normalizePublicId(' A8KP92MQWX7R '), 'a8kp92mqwx7r');
    for (final invalid in [
      '',
      'a8kp92mqwx7l', // l excluida: no se corrige a 1
      'a8kp92mqwx7', // 11 caracteres
      '550e8400-e29b-41d4-a716-446655440000',
      'sigma-academy://perfil/a8kp92mqwx7r',
      'ana@uct.cl',
    ]) {
      expect(normalizePublicId(invalid), isNull, reason: invalid);
    }
  });

  test('getPublicProfile rechaza UUID antes de llamar al backend', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    var calls = 0;
    final service = ProfileService(
      client: MockClient((request) async {
        calls++;
        return http.Response('{}', 200);
      }),
    );
    addTearDown(service.dispose);

    await expectLater(
      service.getPublicProfile('550e8400-e29b-41d4-a716-446655440000'),
      throwsA(
        isA<ProfileApiException>().having(
          (e) => e.code,
          'code',
          'invalid_public_id',
        ),
      ),
    );
    expect(calls, 0);
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
