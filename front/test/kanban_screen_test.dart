import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/workspace/kanban_screen.dart';

void main() {
  Future<void> pumpBoard(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: KanbanScreen()));
    await tester.pumpAndSettle();
  }

  testWidgets('renderiza las tres placas industriales en desktop', (
    tester,
  ) async {
    await pumpBoard(tester, const Size(1440, 900));

    expect(find.text('TABLERO KANBAN'), findsOneWidget);
    expect(find.text('POR HACER'), findsOneWidget);
    expect(find.text('EN PROGRESO'), findsOneWidget);
    expect(find.text('FINALIZADO'), findsOneWidget);
    expect(find.text('Investigar derivadas'), findsOneWidget);
    expect(find.text('ALTA'), findsNWidgets(2));
    expect(tester.takeException(), isNull);
  });

  testWidgets('compacto navega entre placas con chips neobrutalistas', (
    tester,
  ) async {
    await pumpBoard(tester, const Size(430, 900));

    expect(find.text('POR HACER (2)'), findsOneWidget);
    expect(find.text('EN PROGRESO (1)'), findsOneWidget);
    expect(find.text('FINALIZADO (1)'), findsOneWidget);

    await tester.tap(find.text('EN PROGRESO (1)'));
    await tester.pumpAndSettle();
    expect(find.text('Ejercicios capítulo 3'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('agrega una ficha técnica desde el formulario modal', (
    tester,
  ) async {
    await pumpBoard(tester, const Size(1440, 900));

    await tester.tap(find.byTooltip('Agregar ficha').first);
    await tester.pumpAndSettle();
    expect(find.text('NUEVA FICHA TÉCNICA'), findsOneWidget);

    await tester.enterText(
      find.byType(TextField).first,
      'Resolver guía de límites',
    );
    await tester.tap(find.text('AGREGAR A POR HACER'));
    await tester.pumpAndSettle();

    expect(find.text('Resolver guía de límites'), findsOneWidget);
    expect(
      find.descendant(
        of: find.byKey(const ValueKey('column-Por Hacer')),
        matching: find.text('Resolver guía de límites'),
      ),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('arrastra una ficha entre placas en desktop', (tester) async {
    await pumpBoard(tester, const Size(1440, 900));

    final card = find.text('Investigar derivadas');
    final start = tester.getCenter(card);
    final target = tester.getCenter(
      find.byKey(const ValueKey('column-En Progreso')),
    );
    await tester.dragFrom(start, Offset(target.dx - start.dx, 150));
    await tester.pumpAndSettle();

    expect(
      find.descendant(
        of: find.byKey(const ValueKey('column-En Progreso')),
        matching: find.text('Investigar derivadas'),
      ),
      findsOneWidget,
    );
    expect(
      find.descendant(
        of: find.byKey(const ValueKey('column-Por Hacer')),
        matching: find.text('Investigar derivadas'),
      ),
      findsNothing,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza el tablero sin desbordes', (
    tester,
  ) async {
    await pumpBoard(tester, const Size(320, 800));

    expect(find.text('TABLERO KANBAN'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto abre el formulario sin desbordes', (
    tester,
  ) async {
    await pumpBoard(tester, const Size(360, 800));

    await tester.tap(find.byTooltip('Agregar ficha').first);
    await tester.pumpAndSettle();

    expect(find.text('NUEVA FICHA TÉCNICA'), findsOneWidget);
    expect(find.text('Código / curso'), findsOneWidget);
    expect(
      find.byWidgetPredicate((widget) => widget is DropdownButtonFormField),
      findsNWidgets(2),
    );
    expect(tester.takeException(), isNull);
  });
}
