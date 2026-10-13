import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/course_windows.dart';
import 'package:taller_integracion_front/features/academic/courses_screen.dart';
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

  testWidgets('MaterialsWindow sin apuntes: estado vacío, no crash', (tester) async {
    await pumpSettled(
      tester,
      const MaterialsWindow(subjectCode: 'INF-301', subjectName: 'Redes'),
    );
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
    expect(find.text('CREAR APUNTE'), findsOneWidget);
    expect(find.textContaining('SIN APUNTES'), findsOneWidget);
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

  testWidgets('Crear apunte vuelve a Materiales (no al home)', (tester) async {
    await pumpSettled(
      tester,
      const MaterialsWindow(subjectCode: 'INF-301', subjectName: 'Redes'),
    );
    await tester.tap(find.text('CREAR APUNTE'));
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    expect(find.text('TODAS LAS NOTAS'), findsOneWidget);
    // Cierra el diálogo de crear y vuelve con el back del header.
    await tester.tap(find.text('CANCELAR'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.tap(find.byTooltip('Volver'));
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 300));
    }
    expect(find.text('MATERIALES · REDES'), findsOneWidget);
  });

  testWidgets('Detalle muestra solo ventanas, sin tabs viejos', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: CoursesScreen()));
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
}
