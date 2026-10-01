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
}
