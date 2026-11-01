import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/profile_models.dart';
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
  'display_name': 'Nombre Viejo',
  'visibility': 'public',
  'institution': 'UCT',
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

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    ProfileService.current.value = null;
  });

  tearDown(() async {
    ProfileService.current.value = null;
    await SessionManager.clear();
  });

  testWidgets('sidebar muestra perfil y reacciona a PATCH sin relogin', (
    tester,
  ) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    final social = SocialService(
      client: MockClient((request) async {
        return http.Response(
          jsonEncode(_overview),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(social.dispose);
    final profiles = ProfileService(
      client: MockClient((request) async {
        return http.Response(
          jsonEncode(_profile),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(profiles.dispose);

    await _pumpShell(tester, social, profiles);

    expect(find.text('Nombre Viejo'), findsOneWidget);

    // Simula PATCH /profile/me exitoso en otra pantalla: el notifier es la
    // única fuente y el sidebar se repinta solo.
    ProfileService.current.value = UserProfile(
      displayName: 'Nombre Nuevo',
      institution: 'UFRO',
      visibility: 'public',
    );
    await tester.pumpAndSettle();

    expect(find.text('Nombre Nuevo'), findsOneWidget);
    expect(find.text('UFRO'), findsOneWidget);
    expect(find.text('Nombre Viejo'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
