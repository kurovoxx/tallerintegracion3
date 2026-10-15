import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/course_windows.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

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

  testWidgets('Flujo directo: tap abre visor sin AllNotesScreen intermedia',
      (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: MaterialsWindow(
          subjectId: 'mat-1',
          subjectCode: 'INF-301',
          subjectName: 'Redes',
          initialNotes: [
            RemoteNote(
              id: 'n1',
              title: 'Apunte 1',
              content: 'Contenido directo',
              visibility: 'private',
              updatedAt: DateTime(2026, 10, 1),
              subjectId: 'mat-1',
            ),
          ],
        ),
      ),
    );
    await tester.pump();
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
    expect(find.text('ABRIR APUNTES'), findsNothing);

    await tester.tap(find.text('Apunte 1'));
    await tester.pumpAndSettle();
    // Visor directo, sin pasar por AllNotesScreen.
    expect(find.byType(NoteViewerWindow), findsOneWidget);
    expect(find.byType(AllNotesScreen), findsNothing);
    expect(find.text('Contenido directo'), findsOneWidget);

    // Back del visor vuelve a Materiales.
    await tester.tap(find.byType(BackButton));
    await tester.pumpAndSettle();
    expect(find.byType(NoteViewerWindow), findsNothing);
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
  });
}
