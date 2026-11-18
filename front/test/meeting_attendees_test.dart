import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/workspace/schedule_meeting_screen.dart';

void main() {
  test('normaliza duplicados, inválidos y máximo exacto del backend', () {
    expect(normalizeMeetingAttendees([' A@X.CL ', 'a@x.cl', ' B@X.CL ', '']), [
      'a@x.cl',
      'b@x.cl',
    ]);
    expect(() => normalizeMeetingAttendees(['uuid']), throwsFormatException);
    expect(
      normalizeMeetingAttendees(List.generate(50, (i) => 'u$i@x.cl')).length,
      50,
    );
    expect(
      () => normalizeMeetingAttendees(List.generate(51, (i) => 'u$i@x.cl')),
      throwsFormatException,
    );
  });
  for (final guests in [
    <String>[],
    ['a@x.cl'],
    ['a@x.cl', 'b@x.cl'],
  ]) {
    test('payload exacto con ${guests.length} invitados', () async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession('test', null);
      final service = SocialService(
        client: MockClient((r) async {
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          expect(body.keys.toSet(), {
            'title',
            'scheduled_at',
            if (guests.isNotEmpty) 'attendees',
          });
          expect(body['attendees'], guests.isEmpty ? isNull : guests);
          return http.Response('{"meeting_id":"id"}', 201);
        }),
      );
      await service.createMeeting(
        groupId: 'g',
        title: 'test',
        scheduledAtUtc: DateTime.utc(2026, 10, 1),
        attendees: guests,
      );
      service.dispose();
      await SessionManager.clear();
    });
  }
  testWidgets('entrada de correo real, sin duplicados y eliminar chip', (
    t,
  ) async {
    await t.pumpWidget(const MaterialApp(home: ScheduleMeetingScreen()));
    await t.pumpAndSettle();
    // El formulario ya no vincula Calendar: el enlace se envía por correo.
    expect(find.textContaining('GOOGLE CALENDAR'), findsNothing);
    expect(find.textContaining('próximamente'), findsNothing);
    expect(find.textContaining('INVITAR MIEMBROS'), findsOneWidget);
    expect(find.textContaining('INVITAR POR CORREO'), findsOneWidget);
    final field = find.widgetWithText(TextField, 'Correo del invitado');
    await t.ensureVisible(field);
    await t.enterText(field, 'Persona@Correo.cl');
    await t.ensureVisible(find.text('AÑADIR INVITADO'));
    await t.tap(find.text('AÑADIR INVITADO'));
    await t.pumpAndSettle();
    final chip = find.byKey(const Key('guest-chip-persona@correo.cl'));
    expect(chip, findsOneWidget);
    // Sin chips Material: el invitado es un sticker neobrutalista.
    expect(find.byType(InputChip), findsNothing);
    await t.enterText(field, ' persona@correo.cl ');
    await t.tap(find.text('AÑADIR INVITADO'));
    await t.pumpAndSettle();
    expect(chip, findsOneWidget);
    await t.ensureVisible(find.byKey(const Key('guest-chip-remove')));
    await t.tap(find.byKey(const Key('guest-chip-remove')));
    await t.pumpAndSettle();
    expect(chip, findsNothing);
    await t.enterText(field, 'nombre-sin-email');
    await t.tap(find.text('AÑADIR INVITADO'));
    await t.pumpAndSettle();
    expect(find.text('Ingresa un correo válido.'), findsOneWidget);
  });
}
