import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/course_windows.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// Consolidación UX (tester): ventanas con back + crear-apunte con preset
// que cancela de vuelta a Materiales. Solo tests.
void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  // Sin pumpAndSettle: I/O real (Drift) no asienta; ventanas por tramos.
  Future<void> pumpWindows(WidgetTester tester) async {
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 300)),
    );
    for (var i = 0; i < 10; i++) {
      await tester.pump(const Duration(milliseconds: 250));
    }
  }

  Future<void> pumpOnCourse(
    WidgetTester tester,
    Widget window,
    String marker,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (ctx) => Scaffold(
            body: Center(
              child: TextButton(
                onPressed: () => Navigator.of(ctx).push(
                  MaterialPageRoute(builder: (_) => window),
                ),
                child: const Text('CURSO'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('CURSO'));
    await pumpWindows(tester);
    expect(find.text(marker), findsOneWidget);
  }

  testWidgets('EvaluationsWindow: back vuelve al curso', (tester) async {
    await pumpOnCourse(
      tester,
      const EvaluationsWindow(subjectId: 's1', subjectName: 'Redes'),
      'EVALUACIONES · REDES',
    );
    await tester.tap(find.byType(BackButton));
    await tester.pumpAndSettle();
    expect(find.text('CURSO'), findsOneWidget);
    expect(find.text('EVALUACIONES · REDES'), findsNothing);
  });

  testWidgets('Ver Materiales abre apuntes directo con preset y back',
      (tester) async {
    // Contrato del botón Ver Materiales (courses_screen): AllNotesScreen
    // con preset de materia + back, sin ventana intermedia.
    await tester.pumpWidget(
      const MaterialApp(
        home: AllNotesScreen(
          initialSubjectCode: 'INF-301',
          initialSubjectId: 'mat-1',
          initialSubjectName: 'Redes',
          showBackButton: true,
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('NOTAS REDES'), findsOneWidget);
    expect(find.byTooltip('Volver'), findsOneWidget);
  });
}
