import 'support/service_uri.dart';
import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/profile_screen.dart';

void main() {
  testWidgets('Drive A conectado → B desconectado → A consulta de nuevo', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'profile_google_drive_connected': true,
    });
    await SessionManager.saveSession('A', {'id': 'A'});
    final pendingB = Completer<http.Response>();
    final consulted = <String>[];
    final service = ProfileService(
      client: MockClient((request) async {
        final user = request.headers['Authorization']!.split(' ').last;
        if (request.url.servicePath.endsWith('/status')) {
          consulted.add(user);
          if (user == 'B') return pendingB.future;
          return http.Response('{"connected":true}', 200);
        }
        return http.Response(
          jsonEncode({'display_name': user, 'visibility': 'public'}),
          200,
        );
      }),
    );
    tester.view.physicalSize = const Size(1280, 1200);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(MaterialApp(home: ProfileScreen(service: service)));
    await tester.pumpAndSettle();
    expect(find.text('CONECTADO'), findsOneWidget);
    final logout = SessionManager.clear();
    expect(ProfileService.current.value, isNull);
    await tester.pump();
    expect(find.text('CONECTADO'), findsNothing);
    await logout;
    await SessionManager.saveSession('B', {'id': 'B'});
    await tester.pump();
    expect(find.text('CONECTADO'), findsNothing);
    // El estado B queda retenido deliberadamente hasta completar la respuesta;
    // no esperes a que se asiente el indicador de carga indeterminado.
    await tester.pump(const Duration(milliseconds: 1));
    expect(find.text('CONSULTANDO'), findsOneWidget);
    pendingB.complete(http.Response('{"connected":false}', 200));
    await tester.pumpAndSettle();
    expect(find.text('SIN CONECTAR'), findsOneWidget);
    await SessionManager.clear();
    await SessionManager.saveSession('A', {'id': 'A'});
    await tester.pumpAndSettle();
    expect(find.text('CONECTADO'), findsOneWidget);
    expect(consulted, ['A', 'B', 'A']);
    await tester.pumpWidget(const SizedBox());
    service.dispose();
    await SessionManager.clear();
  });
}
