import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/academic/schedule_screen.dart';

void main() {
  Future<void> pumpSchedule(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: ScheduleScreen()));
    await tester.pump(const Duration(milliseconds: 600));
  }

  testWidgets('renderiza la matriz semanal expandida en desktop', (
    tester,
  ) async {
    await pumpSchedule(tester, const Size(1440, 1000));

    expect(find.text('HORARIO SEMESTRAL'), findsOneWidget);
    expect(find.text('SEMANA'), findsOneWidget);
    expect(find.text('POR DÍA'), findsOneWidget);
    expect(find.text('CÁLCULO III'), findsNWidgets(2));
    expect(tester.takeException(), isNull);
  });

  testWidgets('renderiza la vista diaria compacta y abre el detalle de clase', (
    tester,
  ) async {
    await pumpSchedule(tester, const Size(430, 900));

    final mondayChip = find.textContaining(RegExp(r'^LUN '));
    expect(mondayChip, findsOneWidget);
    await tester.tap(mondayChip);
    await tester.pumpAndSettle();

    final block = find.text('CÁLCULO III');
    expect(block, findsOneWidget);

    await tester.ensureVisible(block);
    await tester.pumpAndSettle();
    await tester.tap(block, warnIfMissed: false);
    await tester.pumpAndSettle();

    expect(find.byType(Dialog), findsOneWidget);
    expect(find.text('REGISTRO DE MATRIZ · SIGMA ACADEMY'), findsOneWidget);

    await tester.tap(find.text('CERRAR FICHA'));
    await tester.pumpAndSettle();
    expect(find.byType(Dialog), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'matriz semanal compacta con scroll horizontal y cambio de vista',
    (tester) async {
      await pumpSchedule(tester, const Size(430, 900));

      await tester.tap(find.text('SEMANA'));
      await tester.pumpAndSettle();
      expect(find.text('MATRIZ SEMANAL'), findsOneWidget);
      expect(tester.takeException(), isNull);

      await tester.tap(find.text('POR DÍA'));
      await tester.pumpAndSettle();
      expect(find.text('DÍA'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('matriz semanal en tablet usa columnas desplazables', (
    tester,
  ) async {
    await pumpSchedule(tester, const Size(800, 1000));

    expect(find.text('MATRIZ SEMANAL'), findsOneWidget);
    expect(find.text('CÁLCULO III'), findsNWidgets(2));
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'matriz expandida conserva el ancho mínimo de día con scroll lateral',
    (tester) async {
      await pumpSchedule(tester, const Size(1030, 1000));

      expect(find.text('MATRIZ SEMANAL'), findsOneWidget);
      final horizontalScrollable = find.byWidgetPredicate(
        (widget) =>
            widget is Scrollable && widget.axisDirection == AxisDirection.right,
      );
      expect(horizontalScrollable, findsWidgets);
      final position = tester
          .state<ScrollableState>(horizontalScrollable.first)
          .position;
      expect(position.maxScrollExtent, greaterThan(0));
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('telefono angosto soporta la ficha de detalle sin desbordes', (
    tester,
  ) async {
    await pumpSchedule(tester, const Size(320, 800));

    await tester.tap(find.textContaining(RegExp(r'^LUN ')));
    await tester.pumpAndSettle();

    final block = find.text('CÁLCULO III');
    await tester.ensureVisible(block);
    await tester.pumpAndSettle();
    await tester.tap(block, warnIfMissed: false);
    await tester.pumpAndSettle();

    expect(find.byType(Dialog), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
