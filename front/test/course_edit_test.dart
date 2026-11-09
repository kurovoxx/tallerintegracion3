import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/courses_service.dart';
import 'package:taller_integracion_front/features/academic/courses_screen.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    CoursesService.instance.reset();
  });

  tearDown(() {
    CoursesService.instance.reset();
  });

  Future<void> pumpCourses(WidgetTester tester) async {
    tester.view.physicalSize = const Size(1400, 1800);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: CoursesScreen()));
    await tester.pump(const Duration(milliseconds: 700));
    await tester.pump(const Duration(milliseconds: 300));
  }

  Future<void> openCourseDetail(WidgetTester tester) async {
    await tester.tap(find.text('CÁLCULO III').first);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 350));
  }

  Finder dialogFields() {
    return find.descendant(
      of: find.byType(AlertDialog),
      matching: find.byType(TextField),
    );
  }

  testWidgets('Editar curso abre el formulario precargado', (tester) async {
    await pumpCourses(tester);
    await openCourseDetail(tester);

    expect(find.text('EDITAR CURSO'), findsOneWidget);
    await tester.tap(find.text('EDITAR CURSO'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.text('GUARDAR CAMBIOS'), findsOneWidget);
    final fields = dialogFields();
    expect(fields, findsNWidgets(4));
    expect(
      tester.widget<TextField>(fields.at(0)).controller!.text,
      'Cálculo III',
    );
    expect(tester.widget<TextField>(fields.at(1)).controller!.text, 'INF-1111');
    expect(tester.widget<TextField>(fields.at(2)).controller!.text, '5');
    expect(tester.widget<TextField>(fields.at(3)).controller!.text, 'Prof. 1');
    expect(
      find.descendant(
        of: find.byType(AlertDialog),
        matching: find.text('Lun / Mié · 10:00–11:30'),
      ),
      findsOneWidget,
    );
    expect(
      find.descendant(
        of: find.byType(AlertDialog),
        matching: find.text('SALA CJP11-204'),
      ),
      findsOneWidget,
    );
  });

  testWidgets('Editar curso guarda cambios y refresca el detalle', (
    tester,
  ) async {
    await pumpCourses(tester);
    await openCourseDetail(tester);

    await tester.tap(find.text('EDITAR CURSO'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    final fields = dialogFields();
    await tester.enterText(fields.at(0), 'Cálculo IV');
    await tester.enterText(fields.at(3), 'Prof. Nuevo');
    await tester.tap(find.text('GUARDAR CAMBIOS'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.text('CÁLCULO IV'), findsOneWidget);
    expect(find.text('Prof. Nuevo'), findsOneWidget);
    expect(CoursesService.instance.courses.first['name'], 'Cálculo IV');
  });
}
