import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/core/widgets/main_shell.dart';

const _overview = {
  'user_id': 'u',
  'sidebar': {
    'groups': [
      {'group_id': 'g1', 'name': 'G1', 'role': 'admin'},
    ],
  },
  'groups': [
    {'group_id': 'g1', 'name': 'G1', 'role': 'admin', 'member_count': 1},
  ],
  'stats': {'groups_count': 1, 'admin_groups_count': 1},
};

const _profile = {
  'display_name': 'Nombre',
  'visibility': 'public',
  'institution': 'UCT',
};

Map<String, dynamic> _notification({required bool read}) => {
  'id': 'n1',
  'meeting_id': 'm1',
  'group_id': 'g1',
  'group_name': 'G1',
  'title': 'Reunión',
  'description': 'detalle',
  'link': null,
  'scheduled_at': '2026-11-13T10:00:00Z',
  'read': read,
  'read_at': read ? '2026-11-12T10:00:00Z' : null,
  'created_at': '2026-11-12T09:00:00Z',
};

Future<void> _pumpShell(
  WidgetTester tester,
  SocialService social,
  ProfileService profiles,
) async {
  tester.view.physicalSize = const Size(1280, 900);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      home: MainShell(
        initialIndex: 6,
        service: social,
        profileService: profiles,
      ),
    ),
  );
  await tester.pumpAndSettle();
}

SocialService _socialWithNotifications(List<Map<String, dynamic>> items) {
  return SocialService(
    client: MockClient((request) async {
      if (request.url.path.endsWith('/notifications/me')) {
        return http.Response(
          jsonEncode({'data': items}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response(
        jsonEncode(_overview),
        200,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
}

ProfileService _profiles() {
  return ProfileService(
    client: MockClient(
      (request) async => http.Response(
        jsonEncode(_profile),
        200,
        headers: {'content-type': 'application/json'},
      ),
    ),
  );
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    ProfileService.current.value = null;
  });

  tearDown(() async {
    ProfileService.current.value = null;
    await SessionManager.clear();
  });

  testWidgets('campana muestra puntito rojo si hay no leídas', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    final social = _socialWithNotifications([_notification(read: false)]);
    addTearDown(social.dispose);
    final profiles = _profiles();
    addTearDown(profiles.dispose);

    await _pumpShell(tester, social, profiles);

    expect(
      find.byKey(const ValueKey('notifications-unread-dot')),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('campana sin puntito si todas están leídas', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    final social = _socialWithNotifications([_notification(read: true)]);
    addTearDown(social.dispose);
    final profiles = _profiles();
    addTearDown(profiles.dispose);

    await _pumpShell(tester, social, profiles);

    expect(
      find.byKey(const ValueKey('notifications-unread-dot')),
      findsNothing,
    );
    expect(tester.takeException(), isNull);
  });
}
