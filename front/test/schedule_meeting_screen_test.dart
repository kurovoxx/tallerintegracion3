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

  testWidgets('renderiza el formulario en desktop sin campos fantasma', (
    tester,
  ) async {
    await pumpMeeting(tester, const Size(1280, 900));

    expect(find.text('TÍTULO DE LA REUNIÓN'), findsOneWidget);
    expect(find.text('FECHA'), findsOneWidget);
    expect(find.text('HORA'), findsOneWidget);
    expect(find.text('AGENDAR REUNIÓN'), findsWidgets);
    expect(find.textContaining('AGENDAR'), findsWidgets);
    // Sin mocks ni campos que el backend ignora.
    expect(find.textContaining('MIEMBROS INVITADOS'), findsNothing);
    expect(find.textContaining('ENLACE / SALA'), findsNothing);
    expect(find.textContaining('Sofía'), findsNothing);
    expect(find.textContaining('POST real'), findsNothing);
    expect(find.textContaining('LOCAL, NO SE ENV'), findsNothing);
    expect(find.text('Vincular con Google Calendar'), findsNothing);
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
