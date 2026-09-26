import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/models/social_models.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';

void main() {
  test('getOverview parsea contrato real GET /me/overview', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/me/overview');
        expect(request.headers['Authorization'], 'Bearer jwt-test');
        return http.Response(
          jsonEncode(<String, dynamic>{
            'user_id': 'u',
            'sidebar': {
              'groups': [
                {'group_id': 'g1', 'name': 'G1', 'role': 'admin'},
              ],
            },
            'groups': [
              {
                'group_id': 'g1',
                'name': 'G1',
                'description': 'D1',
                'role': 'admin',
                'member_count': 5,
                'joined_at': '2026-08-11T12:00:00Z',
              },
            ],
            'stats': {'groups_count': 1, 'admin_groups_count': 1},
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final ov = await service.getOverview();
    expect(ov.userId, 'u');
    expect(ov.groups, hasLength(1));
    expect(ov.groups.single.memberCount, 5);
    expect(ov.sidebarGroups.single.name, 'G1');
    expect(ov.groupsCount, 1);
    expect(ov.adminGroupsCount, 1);
  });

  test('createMeeting envía POST real con scheduled_at RFC3339', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    const gid = '11111111-1111-1111-1111-111111111111';
    var checked = false;
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/groups/$gid/meetings');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['title'], 'Daily');
        expect(body['scheduled_at'], contains('T'));
        expect(body.containsKey('members'), isFalse);
        checked = true;
        return http.Response(jsonEncode({'meeting_id': 'm1'}), 201,
            headers: {'content-type': 'application/json'});
      }),
    );
    addTearDown(service.dispose);

    final id = await service.createMeeting(
      groupId: gid,
      title: 'Daily',
      scheduledAtUtc: DateTime.utc(2026, 9, 30, 15),
    );
    expect(id, 'm1');
    expect(checked, isTrue);
  });

  test('getGroup parsea contrato real GET /groups/:id', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    const gid = '11111111-1111-1111-1111-111111111111';
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/groups/$gid');
        return http.Response(
          jsonEncode(<String, dynamic>{
            'id': gid,
            'name': 'Grupo Real',
            'description': 'Desc real',
            'owner_user_id': 'u',
            'notes_restricted_to_staff': false,
            'created_at': '2026-08-11T12:00:00Z',
            'role': 'admin',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final d = await service.getGroup(gid);
    expect(d.name, 'Grupo Real');
    expect(d.role, 'admin');
    expect(d.description, 'Desc real');
  });

  test('sin sesión lanza no_session sin hacer HTTP', () async {
    SessionManager.clear();
    var calls = 0;
    final service = SocialService(
      client: MockClient((request) async {
        calls++;
        return http.Response('{}', 200);
      }),
    );
    addTearDown(service.dispose);

    expect(() => service.getOverview(), throwsA(isA<SocialApiException>()));
    expect(calls, 0);
  });
}
