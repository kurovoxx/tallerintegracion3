import 'dart:convert';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// 12-15. Título obligatorio en NUEVA NOTA: error inline, sin cerrar,
// se limpia al escribir, crear funciona con título válido.
void main() {
  String jwt() =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.test';

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Future<List<String>> setup(WidgetTester t) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwt(), {'id': 'owner'});
    final calls = <String>[];
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes') {
        return json({
          'note_id': '12345678-1234-4234-8234-123456789abc',
          'version': 1,
        }, 201);
      }
      if (r.url.path == '/notes/me') {
        return json({'notes': []});
      }
      return json({}, 404);
    });
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pumpAndSettle();
    return calls;
  }

  Future<void> openCreate(WidgetTester t) async {
    final headerBtn = find.widgetWithText(NeobrutalistButton, 'Nueva nota');
    if (headerBtn.evaluate().isNotEmpty) {
      await t.tap(headerBtn);
    } else {
      await t.tap(find.text('CREAR PRIMERA NOTA'));
    }
    await t.pumpAndSettle();
  }

  Future<void> tapGuardar(WidgetTester t) async {
    await t.tap(find.widgetWithText(NeobrutalistButton, 'GUARDAR'));
    await t.pump();
  }

  testWidgets('12. GUARDAR vacío: no crea, modal abierto, error visible', (
    t,
  ) async {
    final calls = await setup(t);
    await openCreate(t);
    await tapGuardar(t);
    expect(find.text('El título es obligatorio.'), findsOneWidget);
    // Modal sigue abierto y no se llamó al backend.
    expect(find.text('NUEVA NOTA'), findsWidgets);
    expect(
      calls.where((c) => c == 'POST /notes'),
      isEmpty,
    );
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('13. título de espacios se trata como vacío', (t) async {
    final calls = await setup(t);
    await openCreate(t);
    await t.enterText(find.widgetWithText(TextField, 'Título'), '   ');
    await tapGuardar(t);
    expect(find.text('El título es obligatorio.'), findsOneWidget);
    expect(find.text('NUEVA NOTA'), findsWidgets);
    expect(
      calls.where((c) => c == 'POST /notes'),
      isEmpty,
    );
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('14. escribir título válido limpia el error', (t) async {
    await setup(t);
    await openCreate(t);
    await tapGuardar(t);
    expect(find.text('El título es obligatorio.'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextField, 'Título'), 'Mi nota');
    await t.pump();
    expect(find.text('El título es obligatorio.'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('15. título válido crea normalmente', (t) async {
    final calls = await setup(t);
    await openCreate(t);
    await t.enterText(find.widgetWithText(TextField, 'Título'), 'Mi nota');
    await t.enterText(
      find.widgetWithText(TextField, 'Contenido Markdown'),
      'contenido',
    );
    await tapGuardar(t);
    await t.pumpAndSettle();
    expect(
      calls.where((c) => c == 'POST /notes'),
      hasLength(1),
    );
    await t.pumpWidget(const SizedBox());
  });
}
