import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/grade_calculator_screen.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  Future<void> pumpAt(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: GradeCalculatorScreen(
              embedded: true,
              initialCourse: 'Redes de Computadores',
            ),
          ),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 350));
  }

  for (final size in const <Size>[
    Size(360, 800),
    Size(390, 844),
    Size(1280, 800),
    Size(1920, 1080),
  ]) {
    testWidgets(
      'simulador sin overflow en ${size.width.toInt()}x${size.height.toInt()}',
      (tester) async {
        await pumpAt(tester, size);

        expect(find.text('SIMULACIÓN'), findsOneWidget);
        expect(find.text('NOTA OBJETIVO'), findsOneWidget);
        expect(find.text('MODO DE CÁLCULO'), findsOneWidget);
        expect(find.text('NOTAS DEL SIMULADOR'), findsOneWidget);
        expect(find.text('ELIMINAR ÚLTIMA'), findsOneWidget);
        expect(find.text('CALCULAR PROMEDIO'), findsOneWidget);
        expect(find.text('RESULTADO DE LA SIMULACIÓN'), findsOneWidget);
        expect(tester.takeException(), isNull);
      },
    );
  }
}
