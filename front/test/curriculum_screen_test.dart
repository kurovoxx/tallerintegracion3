import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/academic/curriculum_screen.dart';

void main() {
  Future<void> pumpCurriculum(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: CurriculumScreen()));
    await tester.pump(const Duration(milliseconds: 700));
    await tester.pumpAndSettle();
  }

  testWidgets('despliega los semestres en una fila horizontal en desktop', (
    tester,
  ) async {
    await pumpCurriculum(tester, const Size(1600, 1000));

    expect(find.text('MI MALLA CURRICULAR'), findsOneWidget);
    for (var semester = 1; semester <= 8; semester++) {
      expect(find.text('SEMESTRE $semester'), findsOneWidget);
    }
    expect(find.text('Programación I'), findsOneWidget);

    // Fila horizontal única: los semestres 1 a 8 comparten la misma línea y
    // se recorren con scroll lateral, sin grillas de dos filas.
    final first = tester.getTopLeft(find.text('SEMESTRE 1'));
    final second = tester.getTopLeft(find.text('SEMESTRE 2'));
    final fifth = tester.getTopLeft(find.text('SEMESTRE 5'));
    expect(second.dy, closeTo(first.dy, 1));
    expect(second.dx, greaterThan(first.dx));
    expect(fifth.dy, closeTo(first.dy, 1));
    expect(fifth.dx, greaterThan(second.dx));

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
  });

  testWidgets('movil conserva scroll horizontal con BouncingScrollPhysics', (
    tester,
  ) async {
    await pumpCurriculum(tester, const Size(390, 844));
    expect(find.text('SEMESTRE 1'), findsOneWidget);

    final outerList = find.byWidgetPredicate(
      (widget) =>
          widget is ListView && widget.scrollDirection == Axis.horizontal,
    );
    expect(outerList, findsOneWidget);
    expect(
      tester.widget<ListView>(outerList).physics,
      isA<BouncingScrollPhysics>(),
    );

    final horizontalScrollable = find.byWidgetPredicate(
      (widget) =>
          widget is Scrollable && widget.axisDirection == AxisDirection.right,
    );
    final position = tester
        .state<ScrollableState>(horizontalScrollable.first)
        .position;
    expect(position.pixels, 0);

    await tester.drag(outerList, const Offset(-500, 0));
    await tester.pump();
    expect(position.pixels, greaterThan(0));
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto conserva la fila con scroll sin desbordes', (
    tester,
  ) async {
    await pumpCurriculum(tester, const Size(320, 800));

    expect(find.text('MI MALLA CURRICULAR'), findsOneWidget);
    expect(find.text('SEMESTRE 1'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
