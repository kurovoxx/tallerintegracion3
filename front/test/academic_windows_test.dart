import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/academic/academic_models.dart';
import 'package:taller_integracion_front/features/academic/course_detail_tabs.dart';
import 'package:taller_integracion_front/features/academic/course_windows.dart';
import 'package:taller_integracion_front/features/academic/courses_screen.dart';
import 'package:taller_integracion_front/features/academic/grade_calculator_screen.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  // Sin pumpAndSettle: indicadores indeterminados + I/O real (Drift) no
  // asientan; se bombea por ventanas como en all_notes_screen_test.
  Future<void> pumpSettled(WidgetTester tester, Widget home) async {
    await tester.pumpWidget(MaterialApp(home: home));
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 300)),
    );
    for (var i = 0; i < 10; i++) {
      await tester.pump(const Duration(milliseconds: 250));
    }
  }

  testWidgets('UnitsWindow sin sesión: error visible con reintentar', (tester) async {
    await pumpSettled(
      tester,
      const UnitsWindow(subjectId: 's1', subjectName: 'Redes'),
    );
    expect(find.text('UNIDADES · REDES'), findsOneWidget);
    expect(find.text('REINTENTAR'), findsOneWidget);
  });

  testWidgets('Back de cada ventana vuelve al curso', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (ctx) => Scaffold(
            body: Center(
              child: TextButton(
                onPressed: () => Navigator.of(ctx).push(
                  MaterialPageRoute(
                    builder: (_) => const UnitsWindow(subjectId: 's1', subjectName: 'Redes'),
                  ),
                ),
                child: const Text('ABRIR'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('ABRIR'));
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    expect(find.text('UNIDADES · REDES'), findsOneWidget);
    await tester.tap(find.byType(BackButton));
    await tester.pumpAndSettle();
    expect(find.text('ABRIR'), findsOneWidget);
    expect(find.text('UNIDADES · REDES'), findsNothing);
  });

  testWidgets('MaterialsWindow sin sesión: error visible con reintentar', (tester) async {
    await pumpSettled(
      tester,
      const MaterialsWindow(subjectId: 's1', subjectCode: 'INF-301', subjectName: 'Redes'),
    );
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
    expect(find.text('ABRIR APUNTES'), findsOneWidget);
    expect(find.text('REINTENTAR'), findsOneWidget);
  });

  testWidgets('EvaluationsWindow sin sesión: error visible con reintentar', (tester) async {
    await pumpSettled(
      tester,
      const EvaluationsWindow(subjectId: 's1', subjectName: 'Redes'),
    );
    expect(find.text('EVALUACIONES · REDES'), findsOneWidget);
    expect(find.text('REINTENTAR'), findsOneWidget);
  });

  testWidgets('AllNotesScreen con back muestra volver; sin back no', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(home: AllNotesScreen(showBackButton: true)),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byTooltip('Volver'), findsOneWidget);

    await tester.pumpWidget(const MaterialApp(home: AllNotesScreen()));
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byTooltip('Volver'), findsNothing);
  });

  testWidgets('Abrir apuntes entra directo a la lista; back vuelve', (tester) async {
    await pumpSettled(
      tester,
      const MaterialsWindow(subjectId: 's1', subjectCode: 'INF-301', subjectName: 'Redes'),
    );
    await tester.tap(find.text('ABRIR APUNTES'));
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    // Vista que lista (sin auto-formulario encima).
    expect(find.text('NOTAS REDES'), findsOneWidget);
    expect(find.text('NUEVA NOTA'), findsNothing);
    // Back del header vuelve a Materiales (no al home).
    await tester.tap(find.byTooltip('Volver'));
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
  });

  testWidgets('Detalle muestra solo ventanas, sin tabs viejos', (tester) async {    await tester.pumpWidget(const MaterialApp(home: CoursesScreen()));
    await tester.pump(const Duration(milliseconds: 700));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.tap(find.text('CÁLCULO III').first);
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('VER UNIDADES'), findsOneWidget);
    expect(find.text('VER MATERIALES'), findsOneWidget);
    expect(find.text('VER EVALUACIONES'), findsOneWidget);
    expect(find.text('VER ASISTENCIA'), findsOneWidget);
    expect(find.text('Unidades y Contenidos'), findsNothing);
    expect(find.text('Materiales y Apuntes'), findsNothing);
    expect(find.text('Evaluaciones y Notas'), findsNothing);

    await tester.tap(find.text('VER UNIDADES'));
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    expect(find.textContaining('UNIDADES'), findsWidgets);
    await tester.tap(find.byType(BackButton));
    await tester.pumpAndSettle();
    expect(find.text('VER UNIDADES'), findsOneWidget);
  });

  testWidgets('Calculadora embebida preselecciona la materia', (tester) async {
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
    await tester.pump(const Duration(milliseconds: 300));
    // Materia antepuesta al dropdown y sin header propio (lo da la ventana).
    expect(find.text('REDES DE COMPUTADORES'), findsOneWidget);
    expect(find.text('CALCULADORA DE NOTAS'), findsNothing);
    expect(find.text('CALCULAR PROMEDIO'), findsOneWidget);
  });

  testWidgets('Apuntes titula NOTAS <materia>; por defecto TODAS', (tester) async {    await tester.pumpWidget(
      const MaterialApp(home: AllNotesScreen(initialSubjectName: 'Redes')),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('NOTAS REDES'), findsOneWidget);
    expect(find.text('TODAS LAS NOTAS'), findsNothing);

    await tester.pumpWidget(const MaterialApp(home: AllNotesScreen()));
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('TODAS LAS NOTAS'), findsOneWidget);
  });

  test('flattenRenameOptions aplana tops + hijas con compuesto', () {
    AcademicGrade grade(String id, String name) => AcademicGrade(
      id: id,
      subjectId: 's1',
      name: name,
      weight: 0.5,
    );
    final nodes = [
      AcademicGradeNode(
        grade: grade('g1', 'Parcial'),
        children: [grade('h1', 'A'), grade('h2', 'B')],
      ),
      AcademicGradeNode(grade: grade('g2', 'Examen'), children: []),
    ];
    final opts = flattenRenameOptions(nodes);
    expect(opts.map((o) => o.label), [
      'Parcial',
      'Parcial › A',
      'Parcial › B',
      'Examen',
    ]);
    expect(opts.map((o) => o.id), ['g1', 'h1', 'h2', 'g2']);
    expect(flattenRenameOptions([]), isEmpty);
  });

  testWidgets('Crear manual con filtro hereda preset y aparece', (tester) async {
    tester.view.physicalSize = const Size(1440, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(
      const MaterialApp(
        home: AllNotesScreen(
          initialSubjectCode: 'INF-999',
          initialSubjectId: 's9',
        ),
      ),
    );
    for (var i = 0; i < 8; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    // El formulario abre con el preset de materia, no vacío.
    await tester.tap(find.text('NUEVA NOTA'));
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('NUEVA NOTA'), findsWidgets);
    // Guardar tal cual (título con prefijo) y aparece listado y filtrado.
    await tester.tap(find.widgetWithText(NeobrutalistButton, 'GUARDAR'));
    for (var i = 0; i < 8; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    expect(find.textContaining('[INF-999]'), findsWidgets);
  });
}
