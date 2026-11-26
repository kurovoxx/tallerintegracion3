import 'support/service_uri.dart';

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/social_models.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('GroupMember parsea permisos delegados y asume false si faltan', () {
    final m = GroupMember.fromJson({
      'id': '1',
      'group_id': 'g',
      'user_id': 'u',
      'role': 'member',
      'can_share_notes': true,
      'can_edit_kanban': true,
    });
    expect(m.canShareNotes, isTrue);
    expect(m.canCreateMeetings, isFalse);
    expect(m.canEditKanban, isTrue);

    final legacy = GroupMember.fromJson({
      'id': '1',
      'group_id': 'g',
      'user_id': 'u',
      'role': 'member',
    });
    expect(legacy.canShareNotes || legacy.canCreateMeetings, isFalse);
  });

  test('setMemberPermissions hace PATCH parcial a /permissions', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(request.url.servicePath, '/groups/g1/members/u2/permissions');
        expect(jsonDecode(request.body), {
          'can_share_notes': true,
          'can_edit_kanban': false,
        });
        return http.Response('{}', 200);
      }),
    );
    await service.setMemberPermissions(
      groupId: 'g1',
      userId: 'u2',
      canShareNotes: true,
      canEditKanban: false,
    );
    expect(
      () => service.setMemberPermissions(groupId: 'g1', userId: 'u2'),
      throwsA(isA<SocialApiException>()),
    );
  });
}
