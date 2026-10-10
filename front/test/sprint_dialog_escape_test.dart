import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';

MockClient _client() {
  return MockClient((request) async {
    final path = request.url.path;
    if (request.method == 'GET' && path == '/groups/$_gid/workspace') {
      return http.Response(
        jsonEncode(<String, dynamic>{
          'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
          'kanban': {'todo': [], 'in_progress': [], 'done': []},
          'sprint_sheet': {
            'sheets': [
              {
                'id': 'cccccccc-0000-4000-8000-000000000001',
                'name': 'Sprint 1',
              },
            ],
            'tasks': [],
          },
          'meetings': {'upcoming': []},
          'chat': {'provider': 'stream', 'token_endpoint': ''},
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (request.method == 'GET' && path == '/groups/$_gid/members') {
      return http.Response(
        jsonEncode([
          {
            'id': 'm1',
            'group_id': _gid,
            'user_id': 'aaaaaaaa-0000-4000-8000-000000000001',
            'role': 'admin',
          },
        ]),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    return http.Response('{}', 404);
  });
}

void main() {
  testWidgets('cerrar con Escape el diálogo de tarea no rompe', (tester) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(client: _client());
    addTearDown(service.dispose);

    tester.view.physicalSize = const Size(1280, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        home: SprintSheetScreen(groupId: _gid, service: service),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('NUEVA TAREA'));
    await tester.pumpAndSettle();

    // Abrir el selector de responsable y elegir miembro.
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.textContaining('admin').last);
    await tester.pumpAndSettle();

    // Escape con el diálogo abierto no debe romper (bug framework
    // '_dependents.isEmpty' visto en Windows).
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pumpAndSettle();

    // Cierre explícito: tampoco debe lanzar.
    final cancel = find.text('Cancelar');
    if (tester.any(cancel)) {
      await tester.tap(cancel);
      await tester.pumpAndSettle();
    }
    expect(tester.takeException(), isNull);
  });
}
