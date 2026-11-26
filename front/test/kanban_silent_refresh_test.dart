import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/kanban_screen.dart';

// Kanban de grupo: mover/crear no debe "reiniciar" el tablero. Antes cada
// acción ponía el spinner de carga completa y el tablero se reconstruía
// (scroll y columna visible perdidos). Ahora el movimiento es optimista y la
// recarga posterior es silenciosa.
const _gid = '11111111-1111-1111-1111-111111111111';
const _taskId = 'aaaaaaaa-0000-4000-8000-000000000001';

class _FakeBoard {
  String status = 'todo';
  Completer<void>? patchGate;
  bool failPatch = false;
  int workspaceGets = 0;

  Map<String, dynamic> _task() => {
    'id': _taskId,
    'group_id': _gid,
    'board_id': 'bbbbbbbb-0000-4000-8000-000000000001',
    'board_name': 'General',
    'title': 'Tarea Real Kanban',
    'status': status,
    'assigned_to': null,
    'due_date': null,
    'created_at': '2026-09-01T12:00:00Z',
    'updated_at': '2026-09-01T12:00:00Z',
  };

  Future<http.Response> handle(http.Request r) async {
    final path = r.url.path;
    if (r.method == 'GET' && path.endsWith('/workspace')) {
      workspaceGets++;
      return _json({
        'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
        'kanban': {
          'todo': [if (status == 'todo') _task()],
          'in_progress': [if (status == 'in_progress') _task()],
          'done': [if (status == 'done') _task()],
        },
        'sprint_sheet': {'sheets': [], 'tasks': []},
        'meetings': {'upcoming': []},
        'chat': {'provider': 'stream', 'token_endpoint': ''},
      });
    }
    if (r.method == 'PATCH' && path.contains(_taskId)) {
      await patchGate?.future;
      if (failPatch) {
        return _json({
          'error': {'code': 'internal', 'message': 'x'},
        }, 500);
      }
      status = (jsonDecode(r.body) as Map)['status'] as String? ?? status;
      return _json(_task());
    }
    if (r.method == 'GET' && path.endsWith('/members')) {
      return _json({'members': []});
    }
    return _json({}, 404);
  }

  http.Response _json(Object body, [int code = 200]) => http.Response(
    jsonEncode(body),
    code,
    headers: {'content-type': 'application/json'},
  );
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  Future<_FakeBoard> pumpBoard(WidgetTester tester) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    tester.view.physicalSize = const Size(1440, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final board = _FakeBoard();
    final service = SocialService(client: MockClient(board.handle));
    addTearDown(service.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: KanbanScreen(groupId: _gid, service: service),
      ),
    );
    await tester.pumpAndSettle();
    return board;
  }

  final back = find.byTooltip('Mover a columna anterior');
  final forward = find.byTooltip('Mover a columna siguiente');

  testWidgets('mover es optimista y no reemplaza el tablero por el spinner', (
    tester,
  ) async {
    final board = await pumpBoard(tester);
    expect(back, findsNothing); // la ficha parte en "Por hacer"
    board.patchGate = Completer<void>();

    await tester.tap(forward.first);
    await tester.pump();

    // PATCH aún en vuelo: tablero visible y ficha ya en la columna nueva.
    expect(find.byType(CircularProgressIndicator), findsNothing);
    expect(find.text('TABLERO KANBAN'), findsOneWidget);
    expect(back, findsOneWidget);

    board.patchGate!.complete();
    await tester.pumpAndSettle();
    expect(board.status, 'in_progress');
    expect(back, findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsNothing);
    // Se refrescó en segundo plano (carga inicial + recarga silenciosa).
    expect(board.workspaceGets, 2);
    expect(tester.takeException(), isNull);
  });

  testWidgets('si el servidor rechaza el movimiento la ficha vuelve', (
    tester,
  ) async {
    final board = await pumpBoard(tester);
    board.failPatch = true;

    await tester.tap(forward.first);
    await tester.pumpAndSettle();

    expect(board.status, 'todo');
    expect(back, findsNothing);
    expect(find.text('No se pudo mover la tarea.'), findsOneWidget);
    expect(find.text('TABLERO KANBAN'), findsOneWidget);
  });
}
