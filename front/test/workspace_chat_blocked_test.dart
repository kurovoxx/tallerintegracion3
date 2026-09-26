import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/chat/group_chat_screen.dart';
import 'package:taller_integracion_front/features/workspace/schedule_meeting_screen.dart';

void main() {
  testWidgets('chat muestra bloqueo sin mensajes demo', (tester) async {
    tester.view.physicalSize = const Size(1280, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(const MaterialApp(
        home: GroupChatScreen(
            groupId: '11111111-1111-1111-1111-111111111111')));
    await tester.pumpAndSettle();

    expect(find.text('MENSAJERÍA NO DISPONIBLE'), findsOneWidget);
    expect(find.text('Hola equipo, ¿revisaron los apuntes de cálculo?'),
        findsNothing);
    expect(find.textContaining('stream-token'), findsWidgets);
    expect(tester.takeException(), isNull);
  });

  testWidgets('agendar sin grupo real no envía y lo dice', (tester) async {
    tester.view.physicalSize = const Size(1280, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
        const MaterialApp(home: ScheduleMeetingScreen()));
    await tester.pumpAndSettle();

    expect(find.textContaining('Sin grupo real'), findsOneWidget);
    expect(find.text('Vincular con Google Calendar'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
