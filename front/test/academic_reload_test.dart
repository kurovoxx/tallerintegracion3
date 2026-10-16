import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/academic_mappers.dart';
import 'package:taller_integracion_front/features/academic/academic_models.dart';
import 'package:taller_integracion_front/features/academic/grade_calculator_screen.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('hasSavedNotes: vacío false, con nodos true', () {
    expect(hasSavedNotes([]), isFalse);
    final node = AcademicGradeNode(
      grade: AcademicGrade(
        id: 'g1',
        subjectId: 's1',
        name: 'Parcial',
        weight: 0.5,
      ),
      children: const [],
    );
    expect(hasSavedNotes([node]), isTrue);
  });

  test('mensajes de recarga documentan vacío y descarte local', () {
    expect(reloadEmptyMessage, 'Sin notas para cargar');
    expect(reloadSuccessMessage, contains('descartados'));
  });

  testWidgets('embebida muestra CARGAR NOTAS; genérica no', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: GradeCalculatorScreen(embedded: true),
          ),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('CARGAR NOTAS'), findsOneWidget);

    await tester.pumpWidget(
      const MaterialApp(home: GradeCalculatorScreen(embedded: false)),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('CARGAR NOTAS'), findsNothing);
  });

  testWidgets('Cargar sin sesión: error visible y local intacto, sin crash',
      (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: GradeCalculatorScreen(embedded: true),
          ),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 300));
    // Estado local previo: hay campos y el botón existe.
    expect(find.text('CARGAR NOTAS'), findsOneWidget);
    expect(find.text('CALCULAR PROMEDIO'), findsOneWidget);

    await tester.tap(find.text('CARGAR NOTAS'));
    await tester.pump();
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 300)),
    );
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 250));
    }

    // Sin crash: la calculadora sigue viva y el botón reintenta.
    expect(find.text('CARGAR NOTAS'), findsOneWidget);
    expect(find.text('CALCULAR PROMEDIO'), findsOneWidget);
    // Error visible (patrón existente): línea ERROR o mensaje de sesión.
    final errorVisible = find.textContaining('ERROR').evaluate().isNotEmpty ||
        find.textContaining('sesión').evaluate().isNotEmpty;
    expect(errorVisible, isTrue);
  });
}
