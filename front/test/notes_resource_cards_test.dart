import 'dart:convert';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// 8. Cada attachment físico aparece UNA vez y su nombre humano UNA vez
// (sin franja duplicada sobre la miniatura).
// 11. La X de borrado sigue disponible en Editar (cobertura extra).
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

  String jwt() =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.test';

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Future<AppDatabase> setup(
    WidgetTester t, {
    required String content,
    required List<Map<String, dynamic>> atts,
  }) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwt(), {'id': 'owner'});
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    final repo = LocalNotesRepository(db);
    await repo.upsertNote(
      LocalNote(
        id: note,
        title: 'Recursos',
        content: content,
        visibility: 'private',
        updatedAt: DateTime.now(),
        ownerUserId: 'owner',
        version: 2,
      ),
    );
    notesHttpClientOverride = MockClient((r) async {
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': note, 'title': 'Recursos', 'version': 2},
          ],
        });
      }
      if (r.url.path == '/notes/$note') {
        return json({
          'id': note,
          'title': 'Recursos',
          'user_id': 'owner',
          'version': 2,
          'content': content,
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
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await waitFor(t, () => find.text('Recursos').evaluate().isNotEmpty);
    await t.tap(find.text('Recursos'));
    await waitFor(
      t,
      () => find.text('RECURSOS ADJUNTOS').evaluate().isNotEmpty,
    );
    return db;
  }

  Map<String, dynamic> att({
    required String id,
    required String fileType,
    required String fileName,
    required String ext,
  }) => {
    'id': id,
    'note_id': note,
    'file_type': fileType,
    'file_name': fileName,
    'external_file_id': ext,
  };

  testWidgets('imagen: nombre humano una sola vez en la tarjeta', (t) async {
    await setup(
      t,
      content: 'texto\n\n![foto](attachment:$image)',
      atts: [
        att(
          id: image,
          fileType: 'image/png',
          fileName: 'foto.png',
          ext: 'ext-img',
        ),
      ],
    );
    // Una tarjeta, un nombre (sin duplicado overlay + pie).
    expect(find.text('foto.png'), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('pdf: nombre una vez + línea útil de tipo', (t) async {
    await setup(
      t,
      content: 'texto\n\n[guia](attachment:$pdf)',
      atts: [
        att(
          id: pdf,
          fileType: 'application/pdf',
          fileName: 'guia.pdf',
          ext: 'ext-pdf',
        ),
      ],
    );
    expect(find.text('guia.pdf'), findsOneWidget);
    // La segunda línea aporta tipo, no repite el nombre.
    expect(find.textContaining('PDF'), findsWidgets);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('X de borrado sigue disponible en Editar', (t) async {
    await setup(
      t,
      content: 'texto\n\n![foto](attachment:$image)',
      atts: [
        att(
          id: image,
          fileType: 'image/png',
          fileName: 'foto.png',
          ext: 'ext-img',
        ),
      ],
    );
    await t.scrollUntilVisible(
      find.text('Editar'),
      250,
      scrollable: find.byType(Scrollable).last,
    );
    await t.tap(find.text('Editar'));
    await t.pumpAndSettle();
    expect(find.text('ARCHIVOS ADJUNTOS (1)'), findsOneWidget);
    expect(find.byKey(ValueKey('remove-$image')), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });
}
