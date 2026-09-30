import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/workspace/schedule_meeting_screen.dart';

void main() {
  testWidgets(
    'agenda real conserva campos UI sin enviarlos y refresca reuniones',
    (tester) async {
      SharedPreferences.setMockInitialValues({});
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      const groupId = '11111111-1111-1111-1111-111111111111';
      Map<String, dynamic>? payload;
      var loads = 0;
      final service = SocialService(
        client: MockClient((request) async {
          if (request.method == 'GET' &&
              request.url.path.endsWith('/workspace')) {
            loads++;
            return http.Response(
              jsonEncode({
                'group': {'id': groupId, 'name': 'Grupo', 'role': 'admin'},
                'kanban': {'todo': [], 'in_progress': [], 'done': []},
                'sprint_sheet': {'sheets': [], 'tasks': []},
                'meetings': {
                  'upcoming': [
                    {
                      'id': 'existing',
                      'title': 'Reunión existente',
                      'scheduled_at': '2027-01-01T12:00:00Z',
                    },
                    if (payload != null) {'id': 'created', ...payload!},
                  ],
                },
                'chat': {'provider': 'stream', 'token_endpoint': ''},
              }),
              200,
              headers: {'content-type': 'application/json; charset=utf-8'},
            );
          }
          if (request.method == 'POST' &&
              request.url.path.endsWith('/meetings')) {
            payload = jsonDecode(request.body) as Map<String, dynamic>;
            return http.Response('{"meeting_id":"created"}', 201);
          }
          if (request.url.path.endsWith('/members')) {
            return http.Response(
              jsonEncode([
                {
                  'id': 'membership',
                  'group_id': groupId,
                  'user_id': 'real-user',
                  'display_name': 'Camila Pérez',
                  'email': 'camila@example.com',
                  'role': 'member',
                },
                {
                  'id': 'membership2',
                  'group_id': groupId,
                  'user_id': 'hidden-uuid',
                  'role': 'member',
                },
              ]),
              200,
              headers: {'content-type': 'application/json; charset=utf-8'},
            );
          }
          return http.Response('{}', 404);
        }),
      );
      addTearDown(service.dispose);
      tester.view.physicalSize = const Size(1280, 1200);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          home: ScheduleMeetingScreen(groupId: groupId, service: service),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Reunión existente'), findsOneWidget);
      await tester.enterText(find.byType(TextFormField).at(0), '  Repaso  ');
      await tester.enterText(
        find.byType(TextFormField).at(1),
        '  Unidad dos  ',
      );
      // Los selectores abren desde la tarjeta, debajo de su etiqueta.
      await tester.tap(find.byIcon(Icons.calendar_month_rounded).first);
      await tester.pumpAndSettle();
      expect(find.byType(DatePickerDialog), findsOneWidget);
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.access_time_rounded));
      await tester.pumpAndSettle();
      expect(find.byType(TimePickerDialog), findsOneWidget);
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField).at(2), 'Sala 12');
      expect(find.text('CAMILA PÉREZ'), findsOneWidget);
      expect(find.textContaining('hidden-uuid'), findsNothing);
      await tester.ensureVisible(find.text('CAMILA PÉREZ'));
      await tester.tap(find.text('CAMILA PÉREZ'));
      await tester.tap(find.byType(NeobrutalistButton).first);
      await tester.pump();
      expect(
        find.text('Esta función estará disponible próximamente.'),
        findsOneWidget,
      );
      FocusManager.instance.primaryFocus?.unfocus();
      await tester.ensureVisible(find.byType(NeobrutalistButton).last);
      await tester.tap(find.byType(NeobrutalistButton).last);
      await tester.pumpAndSettle();
      expect(payload!['title'], 'Repaso');
      expect(payload!['description'], 'Unidad dos');
      expect(DateTime.parse(payload!['scheduled_at'] as String).isUtc, isTrue);
      expect(payload!.keys.toSet(), {'title', 'description', 'scheduled_at'});
      expect(loads, 2);
      expect(find.text('Repaso'), findsWidgets);
      expect(tester.takeException(), isNull);
    },
  );

  Future<void> pumpMeeting(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: ScheduleMeetingScreen()));
    await tester.pumpAndSettle();
  }

  testWidgets('restaura las secciones visuales de vistas', (tester) async {
    await pumpMeeting(tester, const Size(1280, 900));

    expect(find.text('TÍTULO DE LA REUNIÓN'), findsOneWidget);
    final form = find.byType(Form);
    expect(
      find.descendant(of: form, matching: find.byType(NeobrutalistButton)),
      findsNWidgets(2),
    );
    expect(
      find.descendant(of: form, matching: find.text('PRÓXIMAS REUNIONES')),
      findsNothing,
    );
    expect(
      find.descendant(of: form, matching: find.byType(TextFormField)),
      findsNWidgets(3),
    );
    expect(find.text('FECHA'), findsOneWidget);
    expect(find.text('HORA'), findsOneWidget);
    expect(find.text('AGENDAR REUNIÓN'), findsWidgets);
    expect(find.textContaining('AGENDAR'), findsWidgets);
    // Campos visuales completos, sin integrantes inventados.
    expect(find.textContaining('MIEMBROS INVITADOS'), findsOneWidget);
    expect(find.textContaining('ENLACE / SALA'), findsOneWidget);
    expect(find.textContaining('Sofía'), findsNothing);
    expect(find.textContaining('POST real'), findsNothing);
    expect(find.textContaining('LOCAL, NO SE ENV'), findsNothing);
    expect(find.text('VINCULAR CON GOOGLE CALENDAR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza el formulario sin desbordes', (
    tester,
  ) async {
    await pumpMeeting(tester, const Size(320, 800));

    expect(find.text('AGENDAR REUNIÓN'), findsWidgets);
    expect(
      find.text('Selecciona un grupo para agendar una reunión.'),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });
}
