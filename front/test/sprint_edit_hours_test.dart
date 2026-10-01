import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _member = 'bbbbbbbb-0000-4000-8000-000000000001';
const _taskId = 'dddddddd-0000-4000-8000-000000000001';
const _sheetId = 'cccccccc-0000-4000-8000-000000000001';

Map<String, dynamic> _taskJson(double estimated) => {
  'id': _taskId,
  'group_id': _gid,
  'sheet_id': _sheetId,
  'sheet_name': 'Sprint 1',
  'title': 'Tarea Horas',
  'assigned_to': _member,
  'priority': 'media',
  'status': 'sin_empezar',
  'estimated_hours': estimated,
};

/// Abre Editar desde el menú de la fila.
Future<void> _openEditDialog(WidgetTester tester) async {
  await tester.tap(find.byIcon(Icons.more_vert_rounded).first);
  await tester.pumpAndSettle();
  await tester.tap(find.text('Editar'));
  await tester.pumpAndSettle();
}

/// Suelta el foco antes de pulsar GUARDAR (marco canónico en mayúsculas):
/// hacer pop del diálogo con un TextField enfocado dispara una aserción
/// del framework solo bajo el test binding.
Future<void> _tapGuardar(WidgetTester tester) async {
  FocusManager.instance.primaryFocus?.unfocus();
  await tester.pumpAndSettle();
  await tester.tap(find.text('GUARDAR'));
  await tester.pumpAndSettle();
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  Future<void> pumpSheet(WidgetTester tester, SocialService service) async {
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
  }

  testWidgets('editar tarea muestra horas y las guarda por PATCH', (
    tester,
  ) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    Map<String, dynamic>? patched;
    var currentEstimate = 2.0;
    final service = SocialService(
      client: MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.endsWith('/workspace')) {
          return http.Response(
            jsonEncode({
              'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {
                'sheets': [
                  {'id': _sheetId, 'name': 'Sprint 1'},
                ],
                'tasks': [_taskJson(currentEstimate)],
              },
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            }),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.method == 'GET' && path.endsWith('/members')) {
          return http.Response(
            jsonEncode([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': _member,
                'role': 'member',
                'display_name': 'Ana',
              },
            ]),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.method == 'GET' && path.contains('/hours')) {
          return http.Response(
            jsonEncode({'data': [], 'total_hours': 0}),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.method == 'PATCH' && path.contains('/sprint-sheet/')) {
          patched = jsonDecode(request.body) as Map<String, dynamic>;
          final hours = (patched!['estimated_hours'] as num).toDouble();
          currentEstimate = hours;
          return http.Response(
            jsonEncode({'message': 'ok', 'data': _taskJson(hours)}),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        return http.Response('{}', 404);
      }),
    );
    addTearDown(service.dispose);

    await pumpSheet(tester, service);
    expect(find.textContaining('Tarea Horas'), findsOneWidget);

    // Menú de la fila real -> Editar.
    await _openEditDialog(tester);

    expect(find.text('EDITAR TAREA'), findsOneWidget);
    expect(find.text('Horas asignadas'), findsOneWidget);
    // Precargado con las horas actuales (estimated_hours real).
    expect(find.text('2.0'), findsOneWidget);

    await tester.enterText(find.byType(TextField).at(1), '5.5');
    await _tapGuardar(tester);

    expect(patched, isNotNull);
    expect(patched!['estimated_hours'], 5.5);
    // La tabla se refresca con las horas nuevas (métricas + celdas).
    expect(find.text('5.5h'), findsWidgets);
    expect(tester.takeException(), isNull);
  });

  testWidgets('editar tarea rechaza horas inválidas sin llamar PATCH', (
    tester,
  ) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    var patchCalls = 0;
    final service = SocialService(
      client: MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.endsWith('/workspace')) {
          return http.Response(
            jsonEncode({
              'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {
                'sheets': [
                  {'id': _sheetId, 'name': 'Sprint 1'},
                ],
                'tasks': [_taskJson(2.0)],
              },
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            }),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.method == 'GET' && path.endsWith('/members')) {
          return http.Response(
            jsonEncode([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': _member,
                'role': 'member',
                'display_name': 'Ana',
              },
            ]),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.method == 'GET' && path.contains('/hours')) {
          return http.Response(
            jsonEncode({'data': [], 'total_hours': 0}),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.method == 'PATCH') patchCalls++;
        return http.Response('{}', 404);
      }),
    );
    addTearDown(service.dispose);

    await pumpSheet(tester, service);

    await _openEditDialog(tester);

    await tester.enterText(find.byType(TextField).at(1), '-3');
    await _tapGuardar(tester);

    // Sin PATCH y con aviso humano (el diálogo ya se cerró al guardar).
    expect(find.text('EDITAR TAREA'), findsNothing);
    expect(patchCalls, 0);
    expect(find.text('Horas asignadas inválidas.'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
