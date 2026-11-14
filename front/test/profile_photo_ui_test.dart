import 'dart:ui' show PointerDeviceKind;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/profile_screen.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    ProfileService.current.value = null;
  });

  tearDown(() async {
    ProfileService.current.value = null;
    await SessionManager.clear();
  });

  testWidgets('hover sobre el avatar muestra el lápiz y editar perfil permite '
      'cambiar la foto', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    final service = ProfileService(
      client: MockClient((request) async {
        if (request.url.path.endsWith('/auth/google-drive/status')) {
          return http.Response(
            '{"connected":false}',
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        return http.Response(
          '{"display_name":"Ana","photo_url":null,"visibility":"public"}',
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    tester.view.physicalSize = const Size(1280, 1200);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(MaterialApp(home: ProfileScreen(service: service)));
    await tester.pumpAndSettle();

    final avatar = find.byKey(const ValueKey('profile-avatar'));
    expect(avatar, findsOneWidget);
    final center = tester.getCenter(avatar);
    final mouse = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await mouse.addPointer(location: const Offset(4, 4));
    addTearDown(mouse.removePointer);
    await mouse.moveTo(center);
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('profile-photo-edit')), findsOneWidget);

    await tester.tap(find.text('EDITAR PERFIL'));
    await tester.pumpAndSettle();

    expect(find.text('FOTO DE PERFIL'), findsOneWidget);
    expect(find.text('AGREGAR FOTO'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
