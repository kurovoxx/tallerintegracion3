import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/workspace/schedule_meeting_screen.dart';

void main() {
  Future<void> pumpMeeting(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: ScheduleMeetingScreen()));
    await tester.pumpAndSettle();
  }

  testWidgets('renderiza el formulario neobrutalista en desktop', (
    tester,
  ) async {
    await pumpMeeting(tester, const Size(1280, 900));

    expect(find.text('TÍTULO DE LA REUNIÓN'), findsOneWidget);
    expect(find.text('FECHA'), findsOneWidget);
    expect(find.text('HORA'), findsOneWidget);
    expect(find.text('MIEMBROS INVITADOS'), findsOneWidget);
    expect(find.text('AGENDAR REUNIÓN'), findsNWidgets(2));
    expect(find.text('SOFÍA'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza el formulario sin desbordes', (
    tester,
  ) async {
    await pumpMeeting(tester, const Size(320, 800));

    expect(find.text('AGENDAR REUNIÓN'), findsWidgets);
    expect(find.text('ENLACE / SALA'), findsOneWidget);
    expect(find.text('MIEMBROS INVITADOS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('alterna la seleccion de integrantes con chips sticker', (
    tester,
  ) async {
    await pumpMeeting(tester, const Size(1280, 900));

    final matias = find.text('MATÍAS');
    await tester.ensureVisible(matias);
    await tester.pumpAndSettle();
    await tester.tap(matias);
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
  });
}
