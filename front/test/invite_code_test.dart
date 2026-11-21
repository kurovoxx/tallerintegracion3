import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/social_models.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';

import 'support/service_uri.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _tok = 'aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee';

void main() {
  group('extractInviteCode', () {
    test('código solo, con espacios o mayúsculas', () {
      expect(extractInviteCode(_tok), _tok);
      expect(extractInviteCode('  $_tok\n'), _tok);
      expect(extractInviteCode(_tok.toUpperCase()), _tok);
    });

    test('mensaje que contiene el código', () {
      expect(
        extractInviteCode('Únete a mi grupo con el código: $_tok :)'),
        _tok,
      );
    });

    test('formato antiguo «id código»: toma el último UUID', () {
      expect(extractInviteCode('$_gid $_tok'), _tok);
    });

    test('sin código devuelve null', () {
      expect(extractInviteCode(''), isNull);
      expect(extractInviteCode('   '), isNull);
      expect(extractInviteCode('tok-123'), isNull);
      expect(extractInviteCode('aaaaaaaa-bbbb-4ccc-8ddd'), isNull);
    });
  });

  group('SocialService.joinGroupByCode', () {
    setUp(() async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession('jwt-test', {'id': 'u'});
    });
    tearDown(() async => SessionManager.clear());

    test(
      'POST /groups/join con solo el código y parsea la respuesta',
      () async {
        late http.Request sent;
        final service = SocialService(
          client: MockClient((request) async {
            sent = request;
            return http.Response(
              jsonEncode({
                'group_id': _gid,
                'name': 'Grupo Ñandú',
                'already_member': false,
              }),
              201,
              headers: {'content-type': 'application/json; charset=utf-8'},
            );
          }),
        );
        addTearDown(service.dispose);
        final before = SocialService.groupsChanged.value;

        final res = await service.joinGroupByCode(' $_tok ');

        expect(sent.method, 'POST');
        expect(sent.url.servicePath, '/groups/join');
        expect(jsonDecode(sent.body), {'invite_token': _tok});
        expect(sent.headers['Authorization'], 'Bearer jwt-test');
        expect(res.groupId, _gid);
        expect(res.name, 'Grupo Ñandú');
        expect(res.alreadyMember, isFalse);
        expect(SocialService.groupsChanged.value, before + 1);
      },
    );

    test('200 already_member=true', () async {
      final service = SocialService(
        client: MockClient(
          (_) async => http.Response(
            jsonEncode({'group_id': _gid, 'name': 'G', 'already_member': true}),
            200,
          ),
        ),
      );
      addTearDown(service.dispose);
      final res = await service.joinGroupByCode(_tok);
      expect(res.alreadyMember, isTrue);
    });

    test('404 y 403 lanzan SocialApiException con su estado', () async {
      for (final status in [404, 403]) {
        final service = SocialService(
          client: MockClient(
            (_) async => http.Response(jsonEncode({'error': 'x'}), status),
          ),
        );
        addTearDown(service.dispose);
        await expectLater(
          service.joinGroupByCode(_tok),
          throwsA(
            isA<SocialApiException>().having(
              (e) => e.statusCode,
              'statusCode',
              status,
            ),
          ),
        );
      }
    });

    test('código vacío no hace petición', () async {
      var calls = 0;
      final service = SocialService(
        client: MockClient((_) async {
          calls++;
          return http.Response('', 201);
        }),
      );
      addTearDown(service.dispose);
      await expectLater(
        service.joinGroupByCode('   '),
        throwsA(isA<SocialApiException>()),
      );
      expect(calls, 0);
    });
  });
}
