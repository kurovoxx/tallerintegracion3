import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/kanban_screen.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';

MockClient _workspaceClient() {
  return MockClient((request) async {
    final body = <String, dynamic>{
      'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
      'kanban': {
        'todo': [
          {
            'id': 'aaaaaaaa-0000-4000-8000-000000000001',
            'group_id': _gid,
            'board_id': 'bbbbbbbb-0000-4000-8000-000000000001',
            'board_name': 'General',
            'title': 'Tarea Real Kanban',
            'status': 'todo',
            'assigned_to': null,
            'due_date': null,
            'created_at': '2026-09-01T12:00:00Z',
            'updated_at': '2026-09-01T12:00:00Z',
          },
        ],
        'in_progress': [],
        'done': [],
      },
      'sprint_sheet': {
        'sheets': [
          {'id': 'cccccccc-0000-4000-8000-000000000001', 'name': 'Sprint 1'}
        ],
        'tasks': [
          {
            'id': 'dddddddd-0000-4000-8000-000000000001',
            'group_id': _gid,
            'sheet_id': 'cccccccc-0000-4000-8000-000000000001',
            'sheet_name': 'Sprint 1',
            'title': 'Tarea Real Sprint',
            'assigned_to': 'u-test',
            'priority': 'alta',
            'status': 'sin_empezar',
            'estimated_hours': 2.0,
            'created_at': '2026-09-01T12:00:00Z',
            'updated_at': '2026-09-01T12:00:00Z',
          },
        ],
      },
      'meetings': {'upcoming': []},
      'chat': {'provider': 'stream', 'token_endpoint': ''},
    };
    return http.Response(jsonEncode(body), 200,
        headers: {'content-type': 'application/json'});
  });
}

Future<void> _pump(WidgetTester tester, Widget w, Size size) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(home: w));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('sprint con grupo real muestra real y oculta preview',
      (tester) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(client: _workspaceClient());
    addTearDown(service.dispose);

    await _pump(tester, SprintSheetScreen(groupId: _gid, service: service),
        const Size(1280, 900));

    expect(find.textContaining('HOJA REAL'), findsWidgets);
    expect(find.textContaining('Tarea Real Sprint'), findsOneWidget);
    expect(find.text('Setup Drift FTS5'), findsNothing);
    expect(find.textContaining('vista previa local'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('sprint sin grupo conserva preview local rotulada',
      (tester) async {
    await _pump(
        tester, const SprintSheetScreen(), const Size(1280, 900));

    expect(find.text('HOJA DE SPRINT'), findsOneWidget);
    expect(find.textContaining('Vista previa local'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('kanban con grupo real muestra real y oculta preview',
      (tester) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(client: _workspaceClient());
    addTearDown(service.dispose);

    await _pump(tester, KanbanScreen(groupId: _gid, service: service),
        const Size(1440, 900));

    expect(find.textContaining('TABLERO REAL'), findsWidgets);
    expect(find.textContaining('Tarea Real Kanban'), findsWidgets);
    expect(find.text('Investigar derivadas'), findsNothing);
    expect(find.text('TABLERO KANBAN'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('kanban sin grupo conserva preview local', (tester) async {
    await _pump(tester, const KanbanScreen(), const Size(1440, 900));

    expect(find.text('TABLERO KANBAN'), findsOneWidget);
    expect(find.text('Investigar derivadas'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
