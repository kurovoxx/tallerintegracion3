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

  testWidgets('MaterialsWindow: back vuelve al curso', (tester) async {
    await pumpOnCourse(
      tester,
      const MaterialsWindow(subjectId: 'mat-1', subjectCode: 'INF-301', subjectName: 'Redes'),
      'MATERIALES · REDES',
    );
    await tester.tap(find.byType(BackButton));
    await tester.pumpAndSettle();
    expect(find.text('CURSO'), findsOneWidget);
    expect(find.text('MATERIALES · REDES'), findsNothing);
  });

  testWidgets('Crear manual hereda preset de materia y vuelve',
      (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: MaterialsWindow(subjectId: 'mat-1', subjectCode: 'INF-301', subjectName: 'Redes'),
      ),
    );
    await pumpWindows(tester);
    expect(find.text('MATERIALES · REDES'), findsOneWidget);

    await tester.tap(find.text('ABRIR APUNTES'));
    await pumpWindows(tester);

    // Vista que lista con preset, SIN auto-formulario encima.
    expect(find.byType(AllNotesScreen), findsOneWidget);
    final notes =
        tester.widget<AllNotesScreen>(find.byType(AllNotesScreen));
    expect(notes.initialSubjectCode, 'INF-301');
    expect(notes.initialSubjectId, 'mat-1');
    expect(notes.autoOpenCreate, isFalse);
    expect(find.text('NUEVA NOTA'), findsNothing);

    // El botón crear existe en desktop: viewport ancho y clic.
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pump(const Duration(milliseconds: 300));
    // NeobrutalistButton mayuscula el label ('Nueva nota' -> 'NUEVA NOTA').
    await tester.tap(find.text('NUEVA NOTA'));
    await pumpWindows(tester);
    expect(find.widgetWithText(TextField, '[INF-301] '), findsOneWidget);

    // Cancelar cierra el diálogo, queda AllNotesScreen.
    // (En desktop el botón crear muestra 'NUEVA NOTA': el diálogo se
    // detecta por su campo con preset, que debe desaparecer.)
    await tester.tap(find.text('CANCELAR'));
    await pumpWindows(tester);
    expect(find.widgetWithText(TextField, '[INF-301] '), findsNothing);
    expect(find.byType(AllNotesScreen), findsOneWidget);

    // Back vuelve a Materiales (no al home ni cierra la app).
    await tester.tap(find.byTooltip('Volver'));
    await pumpWindows(tester);
    expect(find.byType(AllNotesScreen), findsNothing);
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
  });
}
