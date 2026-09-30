import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/kanban_screen.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

const _groupA = '11111111-1111-1111-1111-111111111111';
const _groupB = '22222222-2222-2222-2222-222222222222';
const _taskA = 'aaaaaaaa-0000-4000-8000-000000000001';
const _member = 'bbbbbbbb-0000-4000-8000-000000000001';

Map<String, dynamic> _workspace({
  required String gid,
  required List<Map<String, dynamic>> todos,
  required List<Map<String, dynamic>> sprintTasks,
  List<Map<String, dynamic>> sheets = const [],
}) => {
  'group': {'id': gid, 'name': 'G', 'role': 'admin'},
  'kanban': {
    'todo': todos.where((t) => t['status'] == 'todo').toList(),
    'in_progress': todos.where((t) => t['status'] == 'in_progress').toList(),
    'done': todos.where((t) => t['status'] == 'done').toList(),
  },
  'sprint_sheet': {'sheets': sheets, 'tasks': sprintTasks},
  'meetings': {'upcoming': []},
  'chat': {'provider': 'stream', 'token_endpoint': ''},
};

Map<String, dynamic> _todo(String status) => {
  'id': _taskA,
  'group_id': _groupA,
  'board_id': 'b1',
  'board_name': 'General',
  'title': 'Tarea A',
  'status': status,
  'assigned_to': _member,
};

Map<String, dynamic> _sprintTask() => {
  'id': _taskA,
  'group_id': _groupA,
  'sheet_id': 'cccccccc-0000-4000-8000-000000000001',
  'sheet_name': 'Sprint 1',
  'title': 'Tarea Sprint A',
  'assigned_to': _member,
  'priority': 'alta',
  'status': 'sin_empezar',
  'estimated_hours': 4.0,
};

const _sheets = [
  {
    'id': 'cccccccc-0000-4000-8000-000000000001',
    'name': 'Sprint 1',
    'period_start': '2026-09-28',
    'period_end': '2026-10-02',
  },
];

http.Response _ok(Object body, [int status = 200]) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

Future<void> _pump(
  WidgetTester tester,
  Widget w,
  Size size, {
  bool settle = true,
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(home: w));
  if (settle) {
    await tester.pumpAndSettle();
  } else {
    await tester.pump();
  }
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  testWidgets('kanban: mover con flecha hace PATCH y no duplica', (
    tester,
  ) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    var moved = false;
    final patched = <Map<String, dynamic>>[];
    final service = SocialService(
      client: MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.endsWith('/workspace')) {
          return _ok(
            _workspace(gid: _groupA, todos: [_todo('todo')], sprintTasks: []),
          );
        }
        if (request.method == 'GET' && path.endsWith('/members')) {
          return _ok([], 200);
        }
        if (request.method == 'PATCH' && path.contains('/todo/')) {
          expect(path, '/groups/$_groupA/todo/$_taskA');
          patched.add(jsonDecode(request.body) as Map<String, dynamic>);
          moved = true;
          return _ok({
            'message': 'ok',
            'data': {..._todo('in_progress')},
          });
        }
        return _ok({}, 404);
      }),
    );
    addTearDown(service.dispose);

    await _pump(
      tester,
      KanbanScreen(groupId: _groupA, service: service),
      const Size(1440, 900),
    );

    expect(find.text('Tarea A'), findsOneWidget);
    // Sin drag en modo real: solo flechas y menú.
    expect(find.byIcon(Icons.drag_indicator_rounded), findsNothing);
    expect(find.byIcon(Icons.more_vert_rounded), findsWidgets);

    await tester.tap(find.byTooltip('Mover a columna siguiente').first);
    await tester.pumpAndSettle();

    expect(moved, isTrue);
    expect(patched.single['status'], 'in_progress');
    // Una sola tarjeta tras recargar (sin duplicados).
    expect(find.text('Tarea A'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('kanban A/B: tareas de A jamás aparecen en B', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.contains(_groupA)) {
          return _ok(
            _workspace(gid: _groupA, todos: [_todo('todo')], sprintTasks: []),
          );
        }
        if (request.method == 'GET' && path.contains(_groupB)) {
          return _ok(_workspace(gid: _groupB, todos: [], sprintTasks: []));
        }
        return _ok({}, 404);
      }),
    );
    addTearDown(service.dispose);

    await _pump(
      tester,
      KanbanScreen(groupId: _groupA, service: service),
      const Size(1440, 900),
    );
    expect(find.text('Tarea A'), findsOneWidget);

    // Navegar A -> B (didUpdateWidget): limpia y recarga.
    await tester.pumpWidget(
      MaterialApp(
        home: KanbanScreen(groupId: _groupB, service: service),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Tarea A'), findsNothing);
    expect(find.text('PLACA LIBRE'), findsWidgets);
    expect(tester.takeException(), isNull);
  });

  testWidgets('kanban compacto real no desborda', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        return _ok(
          _workspace(gid: _groupA, todos: [_todo('todo')], sprintTasks: []),
        );
      }),
    );
    addTearDown(service.dispose);

    await _pump(
      tester,
      KanbanScreen(groupId: _groupA, service: service),
      const Size(360, 800),
    );
    expect(find.text('Tarea A'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('sprint real: un solo botón y cabeceras sin duplicar', (
    tester,
  ) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.endsWith('/workspace')) {
          return _ok(
            _workspace(
              gid: _groupA,
              todos: [],
              sprintTasks: [_sprintTask()],
              sheets: _sheets,
            ),
          );
        }
        if (request.method == 'GET' && path.endsWith('/members')) {
          return _ok([
            {
              'id': 'm1',
              'group_id': _groupA,
              'user_id': _member,
              'role': 'admin',
              'display_name': 'Ana Administradora',
            },
          ]);
        }
        if (request.method == 'GET' && path.contains('/hours')) {
          return _ok({
            'data': [
              {'task_id': _taskA, 'log_date': '2026-09-29', 'hours': 1.5},
            ],
            'total_hours': 1.5,
          });
        }
        return _ok({}, 404);
      }),
    );
    addTearDown(service.dispose);

    await _pump(
      tester,
      SprintSheetScreen(groupId: _groupA, service: service),
      const Size(1280, 900),
    );

    // Un solo punto de alta.
    expect(find.text('NUEVA TAREA'), findsOneWidget);
    expect(find.text('AGREGAR TAREA'), findsNothing);
    expect(find.text('Agregar tarea'), findsNothing);
    // Tarea real con responsable por nombre y horas usadas.
    expect(find.textContaining('Tarea Sprint A'), findsOneWidget);
    expect(find.text('Ana Administradora'), findsWidgets);
    // Cabecera de fecha real, sin duplicar día/nombre.
    expect(find.text('29/09'), findsWidgets);
    expect(find.text('VER GRÁFICO BURNDOWN'), findsOneWidget);
    expect(find.text('SPRINT 1'), findsWidgets);
    expect(tester.takeException(), isNull);
  });

  testWidgets('sprint A/B: Task A no aparece en B vacío', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.contains(_groupA)) {
          return _ok(
            _workspace(
              gid: _groupA,
              todos: [],
              sprintTasks: [_sprintTask()],
              sheets: _sheets,
            ),
          );
        }
        if (request.method == 'GET' && path.contains(_groupB)) {
          return _ok(_workspace(gid: _groupB, todos: [], sprintTasks: []));
        }
        return _ok({}, 404);
      }),
    );
    addTearDown(service.dispose);

    await _pump(
      tester,
      SprintSheetScreen(groupId: _groupA, service: service),
      const Size(1280, 900),
    );
    expect(find.textContaining('Tarea Sprint A'), findsOneWidget);

    await tester.pumpWidget(
      MaterialApp(
        home: SprintSheetScreen(groupId: _groupB, service: service),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('Tarea Sprint A'), findsNothing);
    // Grupo vacío: misma carcasa con mensaje integrado.
    expect(find.text('Aún no hay tareas en este sprint.'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
