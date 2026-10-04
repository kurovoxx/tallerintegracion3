import 'dart:async';
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
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/note_file_picker.dart';
import 'package:taller_integracion_front/features/notes/note_file_picker.dart';

// Botón REFRESCAR / SINCRONIZAR DRIVE: solo procesa pendientes.
// 1. todo synced → 0 PATCH, GET /notes/me, mensaje, termina, sin abrir nota.
// 2. nota local pendiente → se sincroniza (1 POST, 0 PATCH a tempId).
// 3. attachment pendiente → se procesa (1 multipart + 1 PATCH).
// 4. error → _isReconciling vuelve a false + aviso humano.
// 5. doble pulsación → una sola operación.
// 6. nada requiere abrir una nota (implícito en 1-3 + asserts).
void main() {
  const uuid1 = '12345678-1234-4234-8234-123456789abc';
  const uuid2 = '22345678-1234-4234-8234-123456789abc';
  const realNew = '32345678-1234-4234-8234-123456789abc';
  const attId = 'abcdefab-1234-4234-8234-123456789abc';
  final pngBytes = base64Decode(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  );

  String jwtFor(String userId) =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': userId, 'exp': 9999999999}))).replaceAll('=', '')}.test';

  Future<void> waitFor(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 80; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 30)),
      );
      await t.pump(const Duration(milliseconds: 100));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó');
  }

  Finder syncButton() => find.widgetWithText(
    NeobrutalistButton,
    'REFRESCAR / SINCRONIZAR DRIVE',
  );

  Future<AppDatabase> seedDb(List<LocalNote> notes) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwtFor('owner'), {'id': 'owner'});
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    final repo = LocalNotesRepository(db);
    for (final n in notes) {
      await repo.upsertNote(n);
    }
    return db;
  }

  LocalNote note({
    required String id,
    required String title,
    String content = 'contenido',
    int? version,
  }) => LocalNote(
    id: id,
    title: title,
    content: content,
    visibility: 'private',
    updatedAt: DateTime.now(),
    ownerUserId: 'owner',
    version: version,
  );

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Future<void> pumpScreen(WidgetTester t, AppDatabase db) async {
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    addTearDown(() async {
      notesHttpClientOverride = null;
      debugClearPendingAttachments();
      await SessionManager.clear();
    });
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pumpAndSettle();
  }

  Future<void> tapSync(WidgetTester t) async {
    await t.ensureVisible(syncButton());
    await t.tap(syncButton());
    await t.pump();
  }

  Future<void> waitIdle(WidgetTester t) async {
    await waitFor(t, () => syncButton().evaluate().isNotEmpty);
  }

  testWidgets('1. todo synced: 0 PATCH, refresh, mensaje, sin abrir nota', (
    t,
  ) async {
    final calls = <String>[];
    final db = await seedDb([
      note(id: uuid1, title: 'Una', version: 2),
      note(id: uuid2, title: 'Dos', version: 3),
    ]);
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': uuid1, 'title': 'Una', 'version': 2},
            {'id': uuid2, 'title': 'Dos', 'version': 3},
          ],
        });
      }
      return json({}, 404);
    });
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    // Sin PATCH a ninguna nota: nada que reconciliar.
    expect(calls.where((c) => c.startsWith('PATCH')), isEmpty);
    // Refresh real sí ocurre.
    expect(calls, contains('GET /notes/me'));
    expect(find.text('Tus notas están sincronizadas.'), findsOneWidget);
    // Ningún detalle abierto.
    expect(find.text('Editar'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('2. nota local pendiente se sincroniza sin PATCH a tempId', (
    t,
  ) async {
    const tempId = '1790998000001';
    final calls = <String>[];
    final db = await seedDb([note(id: tempId, title: 'Local')]);
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes') {
        return json({'note_id': realNew, 'version': 1}, 201);
      }
      if (r.url.path == '/notes/$realNew') {
        return json({
          'id': realNew,
          'title': 'Local',
          'user_id': 'owner',
          'version': 1,
          'content': 'contenido',
          'attachments': [],
        });
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': realNew, 'title': 'Local', 'version': 1},
          ],
        });
      }
      return json({}, 404);
    });
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    expect(
      calls.where((c) => c == 'POST /notes').length,
      1,
    );
    // Ningún PATCH (ni a tempId ni a UUID): sin cambios que empujar.
    expect(calls.where((c) => c.startsWith('PATCH')), isEmpty);
    expect(find.text('Editar'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('3. attachment pendiente se procesa (multipart + PATCH)', (
    t,
  ) async {
    final calls = <String>[];
    final db = await seedDb([
      note(
        id: uuid1,
        title: 'Con pendiente',
        content: 'texto\n\n![foto](attachment-pending://px)',
        version: 2,
      ),
    ]);
    debugInjectPendingAttachment(
      'attachment-pending://px',
      PickedNoteFile('foto.png', pngBytes, 'image/png'),
    );
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes/$uuid1/attachments') {
        return json({'attachment_id': attId}, 201);
      }
      if (r.method == 'PATCH') {
        return json({'version': 3, 'title': 'Con pendiente'});
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': uuid1, 'title': 'Con pendiente', 'version': 3},
          ],
        });
      }
      return json({}, 404);
    });
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    expect(
      calls.where((c) => c == 'POST /notes/$uuid1/attachments').length,
      1,
    );
    expect(
      calls.where((c) => c == 'PATCH /notes/$uuid1').length,
      1,
    );
    expect(calls, contains('GET /notes/me'));
    expect(find.text('Editar'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('4. error libera indicador y avisa en humano', (t) async {
    const tempId = '1790998000002';
    final calls = <String>[];
    final db = await seedDb([note(id: tempId, title: 'Falla')]);
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes') {
        return json({}, 500);
      }
      if (r.url.path == '/notes/me') {
        return json({'notes': []});
      }
      return json({}, 404);
    });
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    // Indicador liberado: el botón vuelve a estar disponible.
    expect(syncButton(), findsOneWidget);
    expect(find.textContaining('Sincronización parcial'), findsOneWidget);
    expect(find.text('Editar'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('5. doble pulsación no duplica la operación', (t) async {
    const tempId = '1790998000003';
    final calls = <String>[];
    final db = await seedDb([note(id: tempId, title: 'Doble')]);
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes') {
        // Retraso virtual: segunda pulsación cae con el sync en curso.
        await Future.delayed(const Duration(seconds: 2));
        return json({'note_id': realNew, 'version': 1}, 201);
      }
      if (r.url.path == '/notes/$realNew') {
        return json({
          'id': realNew,
          'title': 'Doble',
          'user_id': 'owner',
          'version': 1,
          'content': 'contenido',
          'attachments': [],
        });
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': realNew, 'title': 'Doble', 'version': 1},
          ],
        });
      }
      return json({}, 404);
    });
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    // Segunda pulsación con el sync en curso: el botón muestra
    // SINCRONIZANDO… y está deshabilitado (onPressed null) → no-op.
    await t.tap(
      find.widgetWithText(NeobrutalistButton, 'SINCRONIZANDO…'),
    );
    await t.pump();
    await waitIdle(t);
    expect(
      calls.where((c) => c == 'POST /notes').length,
      1,
    );
    await t.pumpWidget(const SizedBox());
  });
}
