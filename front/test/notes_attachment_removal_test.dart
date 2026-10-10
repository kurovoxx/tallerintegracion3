import 'dart:convert';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// Matriz borrado individual de adjuntos (siempre activo en Editar propio):
// 1. X visible por attachment + cabecera ARCHIVOS ADJUNTOS.
// 2. X también visible con el constructor por defecto (sin flag).
// 3. marcar uno no marca el otro.
// 5. deshacer restaura (toggle) y GUARDAR no llama DELETE.
// 12. PDF funciona igual que imagen.
// 11. recarga no hace reaparecer lo eliminado.
// (6,7,8,9,10,13,14 ya cubiertos en notes_delete_flow_test.dart y
// notes_edit_lifecycle_test.dart.)
void main() {
  const note = '12345678-1234-4234-8234-123456789abc';
  const image = 'abcdefab-1234-4234-8234-123456789abc';
  const pdf = 'abcdefab-1234-4234-8234-123456789abd';

  Future<void> waitFor(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 60; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 30)),
      );
      await t.pump(const Duration(milliseconds: 100));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó');
  }

  MockClient buildMock(
    List<String> calls,
    List<Map<String, dynamic>> atts,
    String Function() getContent,
    void Function(String) setContent,
    int Function() getVersion,
    void Function() bumpVersion,
  ) {
    http.Response json(Object b, [int status = 200]) => http.Response(
      jsonEncode(b),
      status,
      headers: {'content-type': 'application/json; charset=utf-8'},
    );
    return MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'PATCH') {
        final b = jsonDecode(r.body);
        expect(b['version'], getVersion());
        bumpVersion();
        setContent(b['content']);
        return json({'version': getVersion(), 'title': 'Prueba borrado'});
      }
      if (r.method == 'DELETE') {
        final id = r.url.pathSegments.last;
        atts.removeWhere((a) => a['id'] == id);
        return http.Response('', 204);
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': note, 'title': 'Prueba borrado', 'version': getVersion()},
          ],
        });
      }
      if (r.url.path == '/notes/$note') {
        return json({
          'id': note,
          'title': 'Prueba borrado',
          'user_id': 'owner',
          'version': getVersion(),
          'content': getContent(),
          'attachments': atts,
        });
      }
      if (r.url.path.endsWith('/content')) {
        return http.Response.bytes(
          base64Decode(
            'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
          ),
          200,
          headers: {'content-type': 'image/png'},
        );
      }
      return json({}, 404);
    });
  }

  Future<void> openNote(WidgetTester t) async {
    await t.tap(find.text('Prueba borrado'));
    await waitFor(
      t,
      () => find.text('RECURSOS ADJUNTOS').evaluate().isNotEmpty,
    );
  }

  Future<void> enterEdit(WidgetTester t) async {
    await t.scrollUntilVisible(
      find.text('Editar'),
      250,
      scrollable: find.byType(Scrollable).last,
    );
    await t.tap(find.text('Editar'));
    await t.pumpAndSettle();
  }

  Future<void> mark(WidgetTester t, String id) async {
    final f = find.byKey(ValueKey('remove-$id'));
    await t.ensureVisible(f);
    await t.tap(f);
    await t.pump();
  }

  testWidgets('X individual, undo y paridad PDF en Editar', (t) async {
    SharedPreferences.setMockInitialValues({});
    final jwt =
        'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.test';
    await SessionManager.saveSession(jwt, {'id': 'owner'});
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    var version = 1;
    var content =
        'antes\n\n![foto](attachment:$image)\n\ndespués\n\n[guia.pdf](attachment:$pdf)';
    final atts = <Map<String, dynamic>>[
      {
        'id': image,
        'note_id': note,
        'file_type': 'image/png',
        'file_name': 'foto.png',
        'external_file_id': 'img',
      },
      {
        'id': pdf,
        'note_id': note,
        'file_type': 'application/pdf',
        'file_name': 'guia.pdf',
        'external_file_id': 'pdf',
      },
    ];
    final calls = <String>[];
    notesHttpClientOverride = buildMock(
      calls,
      atts,
      () => content,
      (c) => content = c,
      () => version,
      () => version++,
    );
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);

    Future<void> pumpHome(AppDatabase database) async {
      await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: database)));
      await waitFor(t, () => find.text('Prueba borrado').evaluate().isNotEmpty);
    }

    await pumpHome(db);
    await openNote(t);
    await enterEdit(t);

    // 1. Cabecera renombrada + X individual por attachment.
    expect(find.text('ARCHIVOS ADJUNTOS (2)'), findsOneWidget);
    expect(find.byKey(ValueKey('remove-$image')), findsOneWidget);
    expect(find.byKey(ValueKey('remove-$pdf')), findsOneWidget);
    // Lectura no expone X: las keys solo viven en Editar (ver test 2).

    // 3. Marcar uno no marca el otro.
    await mark(t, image);
    expect(find.textContaining('SE QUITARÁ'), findsOneWidget);

    // 5. Deshacer restaura: segundo tap quita la marca.
    await mark(t, image);
    expect(find.textContaining('SE QUITARÁ'), findsNothing);
    await t.ensureVisible(find.text('GUARDAR'));
    await t.tap(find.text('GUARDAR'));
    await waitFor(t, () => calls.any((c) => c.startsWith('PATCH')));
    await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
    expect(calls.where((c) => c.startsWith('DELETE')), isEmpty);
    expect(atts.length, 2);

    // 12. PDF: marcar solo el PDF y guardar elimina solo ese.
    // La hoja sigue abierta tras GUARDAR: se entra a Editar directo.
    await enterEdit(t);
    await mark(t, pdf);
    expect(find.textContaining('SE QUITARÁ'), findsOneWidget);
    await t.ensureVisible(find.text('GUARDAR'));
    await t.tap(find.text('GUARDAR'));
    await waitFor(
      t,
      () =>
          find.text('Editar').evaluate().isNotEmpty &&
          calls.where((c) => c == 'GET /notes/$note').length >= 2,
    );
    expect(
      calls.indexOf('PATCH /notes/$note'),
      lessThan(calls.indexOf('DELETE /notes/$note/attachments/$pdf')),
    );
    expect(atts.length, 1);
    expect(atts.single['id'], image);
    expect(content, isNot(contains('attachment:$pdf')));
    expect(content, contains('attachment:$image'));

    // 11. Recarga (pantalla nueva con BD local fresca: la verdad la pone
    // el servidor mock, que ya no tiene el PDF): no reaparece, la imagen sí.
    // Nota: AllNotesScreen cierra la BD recibida en dispose, por eso se usa
    // una BD fresca en lugar de reinyectar la cerrada.
    await t.pumpWidget(const SizedBox());
    await t.pumpAndSettle();
    final dbFresh = AppDatabase.forTesting(NativeDatabase.memory());
    await pumpHome(dbFresh);
    await openNote(t);
    expect(find.text('guia.pdf'), findsNothing);
    await enterEdit(t);
    expect(find.text('ARCHIVOS ADJUNTOS (1)'), findsOneWidget);
    expect(find.byKey(ValueKey('remove-$pdf')), findsNothing);
    expect(find.byKey(ValueKey('remove-$image')), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('sin flag: X siempre visible en Editar propio', (t) async {
    SharedPreferences.setMockInitialValues({});
    final jwt =
        'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.test';
    await SessionManager.saveSession(jwt, {'id': 'owner'});
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    var version = 1;
    var content = 'texto\n\n![foto](attachment:$image)';
    final atts = <Map<String, dynamic>>[
      {
        'id': image,
        'note_id': note,
        'file_type': 'image/png',
        'file_name': 'foto.png',
        'external_file_id': 'img',
      },
    ];
    final calls = <String>[];
    notesHttpClientOverride = buildMock(
      calls,
      atts,
      () => content,
      (c) => content = c,
      () => version,
      () => version++,
    );
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);

    // Sin flag: la X está disponible normalmente en Editar propio.
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await waitFor(t, () => find.text('Prueba borrado').evaluate().isNotEmpty);
    await openNote(t);
    await enterEdit(t);
    expect(find.text('ARCHIVOS ADJUNTOS (1)'), findsOneWidget);
    expect(find.byKey(ValueKey('remove-$image')), findsOneWidget);
    expect(find.textContaining('SE QUITARÁ'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });
}
