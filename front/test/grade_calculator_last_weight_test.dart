import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/grade_calculator_screen.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  Future<void> pumpCalculator(
    WidgetTester tester, {
    bool embedded = false,
  }) async {
    tester.view.physicalSize = const Size(1400, 1800);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        home: embedded
            ? const Scaffold(body: GradeCalculatorScreen(embedded: true))
            : const GradeCalculatorScreen(embedded: false),
      ),
    );
    await tester.pumpAndSettle();
  }

  String weightText(WidgetTester tester, String key) {
    final field = tester.widget<TextField>(find.byKey(ValueKey(key)));
    return field.controller?.text ?? '';
  }

  Finder normalWeightFields() {
    return find.byWidgetPredicate((widget) {
      final key = widget.key;
      return widget is TextField &&
          key is ValueKey<String> &&
          key.value.startsWith('weight-normal-');
    });
  }

  testWidgets('solo la última nota normal absorbe el cambio de peso', (
    tester,
  ) async {
    await pumpCalculator(tester);

    expect(weightText(tester, 'weight-normal-0'), '100');
    expect(weightText(tester, 'weight-normal-4'), '');

    await tester.enterText(find.byKey(const ValueKey('weight-normal-0')), '30');
    await tester.pump();

    expect(weightText(tester, 'weight-normal-0'), '30');
    expect(weightText(tester, 'weight-normal-1'), '');
    expect(weightText(tester, 'weight-normal-2'), '');
    expect(weightText(tester, 'weight-normal-3'), '');
    expect(weightText(tester, 'weight-normal-4'), '70');

    await tester.enterText(find.byKey(const ValueKey('weight-normal-1')), '20');
    await tester.pump();

    expect(weightText(tester, 'weight-normal-4'), '50');
    expect(weightText(tester, 'weight-normal-0'), '30');

    // Editar la última no debe recalcular las anteriores.
    await tester.enterText(find.byKey(const ValueKey('weight-normal-4')), '25');
    await tester.pump();

    expect(weightText(tester, 'weight-normal-0'), '30');
    expect(weightText(tester, 'weight-normal-1'), '20');
    expect(weightText(tester, 'weight-normal-3'), '');
    expect(weightText(tester, 'weight-normal-4'), '25');
  });

  testWidgets('agregar y eliminar notas actualiza cuál es la última', (
    tester,
  ) async {
    await pumpCalculator(tester);

    await tester.enterText(find.byKey(const ValueKey('weight-normal-0')), '30');
    await tester.pump();
    expect(weightText(tester, 'weight-normal-4'), '70');

    await tester.ensureVisible(find.text('AGREGAR NOTA'));
    await tester.tap(find.text('AGREGAR NOTA'));
    await tester.pumpAndSettle();

    var fields = tester.widgetList<TextField>(normalWeightFields()).toList();
    expect(fields, hasLength(6));
    final addedKey = (fields.last.key! as ValueKey<String>).value;
    expect(weightText(tester, addedKey), '0');
    expect(weightText(tester, 'weight-normal-4'), '70');

    await tester.ensureVisible(find.text('ELIMINAR ÚLTIMA'));
    await tester.tap(find.text('ELIMINAR ÚLTIMA'));
    await tester.pumpAndSettle();

    fields = tester.widgetList<TextField>(normalWeightFields()).toList();
    expect(fields, hasLength(5));
    expect(weightText(tester, 'weight-normal-4'), '70');
    expect(weightText(tester, 'weight-normal-0'), '30');
  });

  testWidgets('subnotas: la última absorbe y el rol se mueve al agregar', (
    tester,
  ) async {
    await pumpCalculator(tester);

    await tester.ensureVisible(find.text('SUBDIVIDIR NOTA').first);
    await tester.tap(find.text('SUBDIVIDIR NOTA').first);
    await tester.pumpAndSettle();

    expect(weightText(tester, 'weight-normal-0-child-1'), '50');
    expect(weightText(tester, 'weight-normal-0-child-2'), '50');

    await tester.enterText(
      find.byKey(const ValueKey('weight-normal-0-child-1')),
      '80',
    );
    await tester.pump();
    expect(weightText(tester, 'weight-normal-0-child-2'), '20');

    // Editar la última subnota no toca la anterior.
    await tester.enterText(
      find.byKey(const ValueKey('weight-normal-0-child-2')),
      '30',
    );
    await tester.pump();
    expect(weightText(tester, 'weight-normal-0-child-1'), '80');

    await tester.ensureVisible(find.text('AGREGAR SUBNOTA'));
    await tester.tap(find.text('AGREGAR SUBNOTA'));
    await tester.pumpAndSettle();

    final childFields = tester
        .widgetList<TextField>(
          find.byWidgetPredicate((widget) {
            final key = widget.key;
            return widget is TextField &&
                key is ValueKey<String> &&
                key.value.startsWith('weight-normal-0-child-');
          }),
        )
        .toList();
    expect(childFields, hasLength(3));
    final addedChildKey = (childFields.last.key! as ValueKey<String>).value;
    expect(weightText(tester, addedChildKey), '0');

    await tester.ensureVisible(find.text('ELIMINAR'));
    await tester.tap(find.text('ELIMINAR'));
    await tester.pumpAndSettle();

    // Vuelve a haber dos subnotas: la última absorbe el peso eliminado.
    expect(weightText(tester, 'weight-normal-0-child-1'), '80');
    expect(weightText(tester, 'weight-normal-0-child-2'), '20');
  });

  testWidgets('final/examen: el examen es la última nota', (tester) async {
    await pumpCalculator(tester);

    await tester.tap(find.byType(Switch));
    await tester.pumpAndSettle();

    expect(weightText(tester, 'weight-final'), '70');
    expect(weightText(tester, 'weight-exam'), '30');

    await tester.enterText(find.byKey(const ValueKey('weight-final')), '80');
    await tester.pump();
    expect(weightText(tester, 'weight-final'), '80');
    expect(weightText(tester, 'weight-exam'), '20');

    // Editar el examen (última) no reescribe la nota final.
    await tester.enterText(find.byKey(const ValueKey('weight-exam')), '40');
    await tester.pump();
    expect(weightText(tester, 'weight-final'), '80');
    expect(weightText(tester, 'weight-exam'), '40');
  });

  testWidgets('la calculadora embebida usa la misma regla', (tester) async {
    await pumpCalculator(tester, embedded: true);

    await tester.enterText(find.byKey(const ValueKey('weight-normal-0')), '30');
    await tester.pump();

    expect(weightText(tester, 'weight-normal-0'), '30');
    expect(weightText(tester, 'weight-normal-4'), '70');
  });
}
