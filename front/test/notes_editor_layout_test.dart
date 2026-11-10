import 'support/service_uri.dart';
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

// El diálogo NUEVA NOTA ocupa casi toda la pantalla y el editor + vista
// previa se estiran con el alto disponible, sin desbordes.
void main() {
  String jwt() =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.test';

  Future<void> openCreate(WidgetTester t, Size size) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwt(), {'id': 'owner'});
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    notesHttpClientOverride = MockClient((r) async {
      if (r.url.servicePath == '/notes/me') {
        return http.Response(
          jsonEncode({'notes': []}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response('{}', 404);
    });
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    t.view.physicalSize = size;
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pumpAndSettle();
    final headerBtn = find.widgetWithText(NeobrutalistButton, 'Nueva nota');
    if (headerBtn.evaluate().isNotEmpty) {
      await t.tap(headerBtn.first);
    } else {
      await t.tap(find.text('CREAR PRIMERA NOTA'));
    }
    await t.pumpAndSettle();
    expect(find.text('NUEVA NOTA'), findsWidgets);
  }

  testWidgets('desktop: diálogo casi pantalla completa y editor alto', (
    t,
  ) async {
    const size = Size(1440, 900);
    await openCreate(t, size);

    final dialog = t.getSize(find.byType(NeobrutalistDialog));
    expect(dialog.width, greaterThanOrEqualTo(size.width * 0.92));
    expect(dialog.height, greaterThanOrEqualTo(size.height * 0.92));

    expect(find.byKey(const Key('live-editor-split')), findsOneWidget);
    final preview = t.getSize(find.byKey(const Key('live-editor-preview')));
    final editor = t.getSize(
      find.widgetWithText(TextField, 'Contenido Markdown'),
    );
    // Antes la vista previa tenía un tope fijo de 280 px.
    expect(preview.height, greaterThan(350));
    expect(editor.height, closeTo(preview.height, 1));
    // Botones del pie visibles dentro de la ventana.
    final guardar = t.getRect(
      find.widgetWithText(NeobrutalistButton, 'GUARDAR'),
    );
    expect(guardar.bottom, lessThanOrEqualTo(size.height));
    expect(t.takeException(), isNull);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('móvil: diálogo a pantalla completa con pestañas', (t) async {
    const size = Size(390, 844);
    await openCreate(t, size);

    final dialog = t.getSize(find.byType(NeobrutalistDialog));
    expect(dialog.width, size.width);
    expect(find.byKey(const Key('live-editor-tabs')), findsOneWidget);
    final editor = t.getSize(
      find.widgetWithText(TextField, 'Contenido Markdown'),
    );
    expect(editor.height, greaterThan(200));
    expect(t.takeException(), isNull);
    await t.pumpWidget(const SizedBox());
  });
}
