import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

void main() {
  Future<void> pumpNotes(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: AllNotesScreen()));
    // Sin pumpAndSettle: la barra de sincronización es un indicador
    // indeterminado que nunca "asienta". Drift usa I/O real, así que se le da
    // una ventana fuera del reloj falso antes de continuar.
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 300)),
    );
    for (var i = 0; i < 8; i++) {
      await tester.pump(const Duration(milliseconds: 250));
    }
  }

  testWidgets('renderiza y alterna la vista de la grilla en desktop', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    expect(find.text('TODAS LAS NOTAS'), findsOneWidget);
    expect(find.byTooltip('Vista grilla'), findsOneWidget);

    await tester.tap(find.byTooltip('Vista grilla'), warnIfMissed: false);
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byTooltip('Vista lista'), findsOneWidget);

    await tester.tap(find.byTooltip('Vista lista'), warnIfMissed: false);
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byTooltip('Vista grilla'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza la lista sin desbordes', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(360, 800));

    expect(find.text('TODAS LAS NOTAS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
