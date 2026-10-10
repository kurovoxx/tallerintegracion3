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
  for (final days in [1, 5, 35]) {
    testWidgets('Sprint legacy, vacío y navegación $days días', (t) async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession('test', {'id': 'u'});
      const group = '11111111-1111-1111-1111-111111111111';
      var sheet = <String, dynamic>{
        'id': 'legacy',
        'name': 'Sprint 1',
        'period_start': null,
        'period_end': null,
      };
      var hasTask = false;
      String iso(DateTime d) => d.toIso8601String().substring(0, 10);
      final start = DateTime(2026, 10, 1), end = DateTime(2026, 10, days);
      var patches = 0;
      final service = SocialService(
        client: MockClient((r) async {
          Object body = {};
          if (r.method == 'PATCH') {
            expect(r.url.path, '/groups/$group/sprint-sheets/legacy');
            patches++;
            sheet = {
              'id': 'legacy',
              ...jsonDecode(r.body) as Map<String, dynamic>,
            };
            body = {'data': sheet};
          } else if (r.url.path.endsWith('/workspace')) {
            body = {
              'group': {'id': group},
              'sprint_sheet': {
                'sheets': [sheet],
                'tasks': hasTask
                    ? [
                        {
                          'id': 'task',
                          'sheet_id': 'legacy',
                          'title': 'Primera tarea',
                          'assigned_to': 'u',
                          'estimated_hours': 5,
                        },
                      ]
                    : [],
              },
            };
          } else if (r.url.path.endsWith('/members')) {
            body = [
              {'user_id': 'u', 'display_name': 'Persona'},
            ];
          } else if (r.url.path.endsWith('/hours')) {
            body = {'data': []};
          }
          return http.Response(
            jsonEncode(body),
            200,
            headers: {'content-type': 'application/json'},
          );
        }),
      );
      addTearDown(service.dispose);
      addTearDown(SessionManager.clear);
      t.view.physicalSize = const Size(1200, 1000);
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.resetPhysicalSize);
      addTearDown(t.view.resetDevicePixelRatio);
      Future<void> mount(int key) async {
        await t.pumpWidget(
          MaterialApp(
            home: SprintSheetScreen(
              key: ValueKey(key),
              groupId: group,
              service: service,
            ),
          ),
        );
        await t.pumpAndSettle();
      }

      await mount(0);
      expect(find.text('Aún no hay tareas en este sprint.'), findsOneWidget);
      expect(find.byKey(const ValueKey('sprint-date-scrollbar')), findsNothing);
      await t.tap(find.text('EDITAR SPRINT'));
      await t.pumpAndSettle();
      expect(find.textContaining('Fecha inicio:'), findsOneWidget);
      expect(find.textContaining('Fecha fin:'), findsOneWidget);
      await t.tap(find.text('GUARDAR CAMBIOS'));
      await t.pumpAndSettle();
      expect(patches, 1);
      expect(sheet['period_start'], isNotNull);
      expect(sheet['period_end'], isNotNull);
      sheet = {...sheet, 'period_start': iso(start), 'period_end': iso(end)};
      await mount(1);
      expect(find.byType(Table), findsNothing);
      hasTask = true;
      await mount(2);
      expect(find.text('Primera tarea'), findsOneWidget);
      final bar = t.widget<Scrollbar>(
        find.byKey(const ValueKey('sprint-date-scrollbar')),
      );
      expect(bar.thumbVisibility, isTrue);
      expect(bar.trackVisibility, isTrue);
      final controller = bar.controller!;
      controller.jumpTo(controller.position.maxScrollExtent);
      await t.pumpAndSettle();
      final last =
          '${end.day.toString().padLeft(2, '0')}/${end.month.toString().padLeft(2, '0')}';
      final lastFinder = find.text(last);
      expect(lastFinder, findsOneWidget);
      expect(t.getTopLeft(lastFinder).dx, lessThan(1200));
      expect(t.getTopLeft(lastFinder).dx, greaterThanOrEqualTo(0));
      final next = end.add(const Duration(days: 1));
      expect(
        find.text(
          '${next.day.toString().padLeft(2, '0')}/${next.month.toString().padLeft(2, '0')}',
        ),
        findsNothing,
      );
      await mount(3);
      expect(find.text(last), findsOneWidget);
      await t.pumpWidget(const SizedBox());
    });
  }

  testWidgets('Eliminar sprint con confirmación y reselección vecina', (t) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('test', {'id': 'u'});
    const group = '11111111-1111-1111-1111-111111111111';
    var sheets = [
      {
        'id': 's1',
        'name': 'Sprint 1',
        'period_start': '2026-10-01',
        'period_end': '2026-10-05',
      },
      {
        'id': 's2',
        'name': 'Sprint 2',
        'period_start': '2026-10-06',
        'period_end': '2026-10-10',
      },
      {
        'id': 's3',
        'name': 'Sprint 3',
        'period_start': '2026-10-11',
        'period_end': '2026-10-15',
      },
    ];
    final deletedIds = <String>[];
    final service = SocialService(
      client: MockClient((r) async {
        Object body = {};
        if (r.method == 'DELETE') {
          final id = r.url.pathSegments.last;
          if (sheets.length <= 1) {
            return http.Response(
              jsonEncode({
                'error': {
                  'code': 'invalid_body',
                  'message': 'No puedes eliminar el único sprint del grupo.',
                },
              }),
              400,
              headers: {'content-type': 'application/json'},
            );
          }
          sheets.removeWhere((s) => s['id'] == id);
          deletedIds.add(id);
          body = {
            'data': {'id': id, 'name': 'borrado'},
          };
        } else if (r.url.path.endsWith('/workspace')) {
          body = {
            'group': {'id': group},
            'sprint_sheet': {'sheets': sheets, 'tasks': []},
          };
        } else if (r.url.path.endsWith('/members')) {
          body = [
            {'user_id': 'u', 'display_name': 'Persona'},
          ];
        } else if (r.url.path.endsWith('/hours')) {
          body = {'data': []};
        }
        return http.Response(
          jsonEncode(body),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);
    addTearDown(SessionManager.clear);
    t.view.physicalSize = const Size(1200, 1000);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(
      MaterialApp(
        home: SprintSheetScreen(groupId: group, service: service),
      ),
    );
    await t.pumpAndSettle();
    // X visible con 3 sprints.
    expect(find.byKey(const ValueKey('delete-sheet-s1')), findsOneWidget);
    expect(find.byKey(const ValueKey('delete-sheet-s2')), findsOneWidget);
    expect(find.byKey(const ValueKey('delete-sheet-s3')), findsOneWidget);
    // 12. Cancelar no elimina nada.
    await t.tap(find.byKey(const ValueKey('delete-sheet-s3')));
    await t.pumpAndSettle();
    expect(find.text('ELIMINAR SPRINT'), findsOneWidget);
    expect(
      find.textContaining('También se eliminarán sus tareas'),
      findsOneWidget,
    );
    await t.tap(find.text('CANCELAR'));
    await t.pumpAndSettle();
    expect(deletedIds, isEmpty);
    expect(find.byKey(const ValueKey('delete-sheet-s3')), findsOneWidget);
    // 10. Borrar el activo (Sprint 2) selecciona al siguiente (Sprint 3).
    await t.tap(find.text('SPRINT 2'));
    await t.pumpAndSettle();
    await t.tap(find.byKey(const ValueKey('delete-sheet-s2')));
    await t.pumpAndSettle();
    await t.tap(find.text('ELIMINAR'));
    await t.pumpAndSettle();
    expect(deletedIds, ['s2']);
    expect(find.byKey(const ValueKey('delete-sheet-s2')), findsNothing);
    // 11. Borrar Sprint 1 con otro existente funciona y no renombra.
    await t.tap(find.byKey(const ValueKey('delete-sheet-s1')));
    await t.pumpAndSettle();
    await t.tap(find.text('ELIMINAR'));
    await t.pumpAndSettle();
    expect(deletedIds, ['s2', 's1']);
    expect(find.text('SPRINT 3'), findsOneWidget);
    expect(find.text('SPRINT 1'), findsNothing);
    // 13. Recargar confirma la eliminación.
    await t.pumpWidget(const SizedBox());
    await t.pumpWidget(
      MaterialApp(
        home: SprintSheetScreen(groupId: group, service: service),
      ),
    );
    await t.pumpAndSettle();
    expect(find.byKey(const ValueKey('delete-sheet-s2')), findsNothing);
    expect(find.byKey(const ValueKey('delete-sheet-s1')), findsNothing);
    expect(find.text('SPRINT 3'), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('Sprint único no ofrece eliminar', (t) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('test', {'id': 'u'});
    const group = '11111111-1111-1111-1111-111111111111';
    final service = SocialService(
      client: MockClient((r) async {
        Object body = {};
        if (r.url.path.endsWith('/workspace')) {
          body = {
            'group': {'id': group},
            'sprint_sheet': {
              'sheets': [
                {
                  'id': 'solo',
                  'name': 'Sprint 1',
                  'period_start': '2026-10-01',
                  'period_end': '2026-10-05',
                },
              ],
              'tasks': [],
            },
          };
        } else if (r.url.path.endsWith('/members')) {
          body = [
            {'user_id': 'u', 'display_name': 'Persona'},
          ];
        } else if (r.url.path.endsWith('/hours')) {
          body = {'data': []};
        }
        return http.Response(
          jsonEncode(body),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);
    addTearDown(SessionManager.clear);
    t.view.physicalSize = const Size(1200, 1000);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(
      MaterialApp(
        home: SprintSheetScreen(groupId: group, service: service),
      ),
    );
    await t.pumpAndSettle();
    expect(find.byKey(const ValueKey('delete-sheet-solo')), findsNothing);
    expect(find.text('SPRINT 1'), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });
}
