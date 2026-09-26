import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

void main() {
  Future<void> pumpSprint(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: SprintSheetScreen()));
    await tester.pumpAndSettle();
  }

  testWidgets('renderiza la planilla tecnica retro con doble scroll', (
    tester,
  ) async {
    await pumpSprint(tester, const Size(1280, 900));

    expect(find.text('HOJA DE SPRINT'), findsOneWidget);
    expect(find.text('TAREAS'), findsOneWidget);
    expect(find.text('EN PROCESO'), findsWidgets);
    expect(find.text('ALTA'), findsWidgets);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza la planilla sin desbordes', (
    tester,
  ) async {
    await pumpSprint(tester, const Size(320, 800));

    expect(find.text('HOJA DE SPRINT'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
