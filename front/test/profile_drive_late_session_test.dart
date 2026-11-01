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
  testWidgets('Drive ignora una consulta tardía de A cuando B ya está activo', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('A', {'id': 'A'});
    final lateA = Completer<http.Response>();
    final service = ProfileService(
      client: MockClient((request) async {
        final user = request.headers['Authorization']!.split(' ').last;
        if (request.url.path.endsWith('/status')) {
          return user == 'A'
              ? lateA.future
              : http.Response('{"connected":false}', 200);
        }
        return http.Response(jsonEncode({'display_name': user}), 200);
      }),
    );
    await tester.pumpWidget(MaterialApp(home: ProfileScreen(service: service)));
    // La respuesta de A se retiene para probar el cambio de cuenta.
    await tester.pump();
    await SessionManager.clear();
    await SessionManager.saveSession('B', {'id': 'B'});
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 1));
    expect(find.text('SIN CONECTAR'), findsOneWidget);
    lateA.complete(http.Response('{"connected":true}', 200));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 1));
    expect(find.text('CONECTADO'), findsNothing);
    expect(find.text('SIN CONECTAR'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    service.dispose();
    await SessionManager.clear();
  });
}
