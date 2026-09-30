import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

void main() {
  testWidgets(
    'crear sprint vacío, seleccionar, editar y conservar aislamiento',
    (tester) async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession('jwt', {'id': 'u'});
      const gid = '11111111-1111-1111-1111-111111111111';
      final sheets = <Map<String, dynamic>>[
        {
          'id': 'A',
          'name': 'Sprint 1',
          'period_start': '2026-09-30',
          'period_end': '2026-10-15',
        },
      ];
      Map<String, dynamic>? created, patched;
      final svc = SocialService(
        client: MockClient((r) async {
          Object body = {};
          if (r.url.path.endsWith('/workspace')) {
            body = {
              'group': {'id': gid},
              'sprint_sheet': {
                'sheets': sheets,
                'tasks': [
                  {
                    'id': 'taskA',
                    'sheet_id': 'A',
                    'title': 'Solo Sprint 1',
                    'assigned_to': 'u',
                    'estimated_hours': 5,
                  },
                ],
              },
            };
          }
          if (r.url.path.endsWith('/members')) {
            body = [
              {
                'id': 'm',
                'user_id': 'u',
                'display_name': 'Persona',
                'role': 'member',
              },
            ];
          }
          if (r.url.path.endsWith('/hours')) {
            body = {
              'data': [
                {'task_id': 'taskA', 'log_date': '2026-10-05', 'hours': 2},
                {'task_id': 'taskA', 'log_date': '2026-10-12', 'hours': 3},
              ],
              'total_hours': 5,
            };
          }
          if (r.method == 'POST' && r.url.path.endsWith('/sprint-sheets')) {
            created = jsonDecode(r.body);
            sheets.add({'id': 'B', ...created!});
            return http.Response(jsonEncode({'data': sheets.last}), 201);
          }
          if (r.method == 'PATCH') {
            patched = jsonDecode(r.body);
            sheets[1] = {'id': 'B', ...patched!};
            body = {'data': sheets[1]};
          }
          return http.Response(
            jsonEncode(body),
            200,
            headers: {'content-type': 'application/json; charset=utf-8'},
          );
        }),
      );
      tester.view.physicalSize = const Size(1600, 1100);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          home: SprintSheetScreen(groupId: gid, service: svc),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Solo Sprint 1'), findsOneWidget);
      expect(find.text('05/10'), findsOneWidget);
      expect(find.text('12/10'), findsOneWidget);
      expect(find.text('LUNES'), findsNWidgets(2));
      expect(find.text('2.0h'), findsOneWidget);
      expect(find.text('3.0h'), findsOneWidget);
      await tester.tap(find.text('+ NUEVO SPRINT'));
      await tester.pumpAndSettle();
      expect(find.text('Sprint 2'), findsOneWidget);
      await tester.tap(find.text('CREAR SPRINT'));
      await tester.pumpAndSettle();
      expect(created!['name'], 'Sprint 2');
      expect(find.text('Solo Sprint 1'), findsNothing);
      await tester.tap(find.text('EDITAR SPRINT'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField).last, 'Sprint renombrado');
      await tester.tap(find.text('GUARDAR CAMBIOS'));
      await tester.pumpAndSettle();
      expect(patched!['name'], 'Sprint renombrado');
      expect(find.text('Solo Sprint 1'), findsNothing);
      await tester.tap(find.text('SPRINT 1'));
      await tester.pumpAndSettle();
      expect(find.text('Solo Sprint 1'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      svc.dispose();
      await SessionManager.clear();
    },
  );
  test('createSprintTask envía sheet_id explícito', () async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt', null);
    final svc = SocialService(
      client: MockClient((r) async {
        expect(jsonDecode(r.body)['sheet_id'], 'B');
        return http.Response(
          '{"data":{"id":"t","sheet_id":"B","title":"T"}}',
          201,
        );
      }),
    );
    await svc.createSprintTask(
      groupId: 'g',
      sheetId: 'B',
      title: 'T',
      assignedTo: 'u',
    );
    svc.dispose();
    await SessionManager.clear();
  });
}
