import 'dart:convert';
import 'dart:io';

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

// Fase 1: creación idempotente (clave estable + snapshot) y tombstones
// persistentes de borrado.
void main() {
  const realId = '12345678-1234-4234-8234-123456789abc';
  const tempId = '1700000000000';

  String jwtFor(String userId) =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': userId, 'exp': 9999999999}))).replaceAll('=', '')}.test';

  Future<void> waitFor(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 120; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 20)),
      );
      await t.pump(const Duration(milliseconds: 100));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó');
  }

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Future<void> pumpScreen(WidgetTester t, AppDatabase db) async {
    t.view.physicalSize = const Size(1400, 3000);
    t.view.devicePixelRatio = 1;
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pumpAndSettle();
  }

  Future<void> tapSync(WidgetTester t) async {
    final label = find.text('REFRESCAR / SINCRONIZAR DRIVE');
    if (label.evaluate().isNotEmpty) {
      await t.tap(label.first);
    } else {
      await t.tap(find.byTooltip('Refrescar / sincronizar Drive'));
    }
    await t.pump();
  }

  Future<void> scrollSheetTo(WidgetTester t, Finder target) async {
    final list = find.descendant(
      of: find.byType(DraggableScrollableSheet),
      matching: find.byType(ListView),
    );
    await t.dragUntilVisible(target, list.last, const Offset(0, -300));
    await t.pumpAndSettle();
  }

  Future<void> setupSession(WidgetTester t) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwtFor('owner'), {'id': 'owner'});
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
  }

  testWidgets(
    'CREATE: timeout tras crear en backend → reintento reutiliza la misma '
    'clave y no duplica nota ni archivo',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);
      await repo.upsertNote(
        LocalNote(
          id: tempId,
          title: 'Idem retry',
          content: 'cuerpo v1',
          visibility: 'private',
          updatedAt: DateTime.now(),
          ownerUserId: 'owner',
        ),
      );

      final postKeys = <String>[];
      final remote = <String, Map<String, dynamic>>{};
      var version = 1;
      var postCount = 0;
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'POST' && r.url.path == '/notes') {
          postCount++;
          postKeys.add(r.headers['X-Idempotency-Key'] ?? '');
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          // El backend crea la nota pero la PRIMERA respuesta se pierde (503):
          // este es el escenario que antes generaba una segunda nota/archivo.
          remote[realId] = {
            'id': realId,
            'title': body['title'],
            'user_id': 'owner',
            'version': version,
            'content': body['content'],
            'attachments': <Map<String, dynamic>>[],
          };
          if (postCount == 1) return json({}, 503);
          // Replay idempotente: misma clave+payload => mismo recurso.
          return json({'note_id': realId, 'version': version}, 201);
        }
        if (r.method == 'PATCH') {
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          version++;
          remote[realId]!['content'] = body['content'];
          remote[realId]!['title'] = body['title'];
          remote[realId]!['version'] = version;
          return json({'version': version, 'title': body['title']});
        }
        if (r.url.path == '/notes/me') {
          return json({
            'notes': [
              for (final n in remote.values)
                {'id': n['id'], 'title': n['title'], 'version': n['version']},
            ],
          });
        }
        if (r.url.path == '/notes/reconcile') {
          return json({
            'removed_notes': 0,
            'removed_attachments': 0,
            'pending': 0,
          });
        }
        if (r.url.path == '/notes/$realId') return json(remote[realId]!);
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await waitFor(t, () => find.text('Idem retry').evaluate().isNotEmpty);

      await tapSync(t);
      await waitFor(t, () => postCount >= 1);
      await waitFor(t, () => find.text('Idem retry').evaluate().isNotEmpty);

      // Reintento (REFRESCAR): misma intención → misma clave.
      await tapSync(t);
      await waitFor(
        t,
        () => postCount >= 2 && find.text('Idem retry').evaluate().isNotEmpty,
      );
      await waitFor(t, () => callsContainOnlyOneNote(remote) && postCount >= 2);

      expect(postKeys.length, greaterThanOrEqualTo(2));
      expect(postKeys.first, isNotEmpty);
      expect(postKeys[1], postKeys.first, reason: 'la clave debe ser estable');
      expect(remote.length, 1, reason: 'un solo archivo/nota en el backend');

      final stored = await t.runAsync(() => repo.getAllNotes(ownerId: 'owner'));
      expect(stored, isNotNull);
      expect(stored!.length, 1, reason: 'no debe quedar la fila temporal');
      expect(stored.single.id, realId);

      // Doble sincronización sin cambios: no vuelve a crear.
      final before = postCount;
      await tapSync(t);
      await waitFor(t, () => postCount >= before);
      await t.pump(const Duration(milliseconds: 300));
      expect(postCount, before);
      expect(remote.length, 1);

      await t.pumpWidget(const SizedBox());
      await db.close();
    },
  );

  testWidgets(
    'CREATE: edición con CREATE pendiente crea con el snapshot original y '
    'luego PATCH del contenido vigente',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);
      await repo.upsertNote(
        LocalNote(
          id: tempId,
          title: 'Edición pendiente',
          content: 'contenido original',
          visibility: 'private',
          updatedAt: DateTime.now(),
          ownerUserId: 'owner',
        ),
      );

      final postBodies = <Map<String, dynamic>>[];
      final patchBodies = <Map<String, dynamic>>[];
      var createSucceeded = false;
      final remote = <String, Map<String, dynamic>>{};
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'POST' && r.url.path == '/notes') {
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          postBodies.add(body);
          if (!createSucceeded) {
            // Primer intento: no llega a crear (fallo previo a Drive).
            return json({}, 503);
          }
          remote[realId] = {
            'id': realId,
            'title': body['title'],
            'user_id': 'owner',
            'version': 1,
            'content': body['content'],
            'attachments': <Map<String, dynamic>>[],
          };
          return json({'note_id': realId, 'version': 1}, 201);
        }
        if (r.method == 'PATCH') {
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          patchBodies.add(body);
          remote[realId]!['content'] = body['content'];
          remote[realId]!['version'] = 2;
          return json({'version': 2, 'title': body['title']});
        }
        if (r.url.path == '/notes/me') {
          return json({
            'notes': [
              for (final n in remote.values)
                {'id': n['id'], 'title': n['title'], 'version': n['version']},
            ],
          });
        }
        if (r.url.path == '/notes/reconcile') {
          return json({
            'removed_notes': 0,
            'removed_attachments': 0,
            'pending': 0,
          });
        }
        if (r.url.path == '/notes/$realId') return json(remote[realId]!);
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await waitFor(
        t,
        () => find.text('Edición pendiente').evaluate().isNotEmpty,
      );

      // Primer intento falla; la nota sigue local. La sincronización
      // automática posterior al REFRESCAR reintenta la MISMA intención, por
      // eso puede haber más de un POST (todos con el snapshot original).
      await tapSync(t);
      await waitFor(t, () => postBodies.isNotEmpty);
      await waitFor(
        t,
        () => find
            .text('Reconciliando notas con Google Drive…')
            .evaluate()
            .isEmpty,
      );

      // Editar mientras el CREATE sigue pendiente.
      await t.tap(find.text('Edición pendiente'));
      await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
      await scrollSheetTo(t, find.text('Editar'));
      await t.tap(find.text('Editar'));
      await t.pumpAndSettle();
      final fields = find.byType(TextField);
      await t.enterText(fields.last, 'contenido editado offline');
      await t.ensureVisible(find.text('GUARDAR'));
      await t.tap(find.text('GUARDAR'));
      await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
      // Cerrar el detalle para volver a la lista.
      await t.tapAt(const Offset(20, 20));
      await t.pumpAndSettle();

      createSucceeded = true;
      await tapSync(t);
      await waitFor(t, () => postBodies.length >= 2 && patchBodies.isNotEmpty);

      expect(
        postBodies[0]['content'],
        'contenido original',
        reason: 'el snapshot congela el payload de la primera intención',
      );
      for (final body in postBodies) {
        expect(
          body['content'],
          'contenido original',
          reason: 'todo reintento usa el mismo payload (misma clave)',
        );
      }
      expect(patchBodies.single['content'], 'contenido editado offline');

      final stored = await t.runAsync(() => repo.getAllNotes(ownerId: 'owner'));
      expect(stored, isNotNull);
      expect(stored!.single.content, 'contenido editado offline');

      await t.pumpWidget(const SizedBox());
      await db.close();
    },
  );

  testWidgets('DELETE offline persiste tombstone, no resucita tras reiniciar y '
      'reintenta el borrado remoto al sincronizar', (t) async {
    await setupSession(t);
    // BD en archivo: permite "reiniciar" la app reabriendo la misma base
    // (el State cierra la conexión al liberarse).
    final dbFile = File(
      '${Directory.systemTemp.path}/notes_phase1_'
      '${DateTime.now().microsecondsSinceEpoch}.sqlite',
    );
    addTearDown(() {
      try {
        dbFile.deleteSync();
      } catch (_) {}
    });
    final db = AppDatabase.forTesting(NativeDatabase(dbFile));
    addTearDown(() async {
      try {
        await db.close();
      } catch (_) {}
    });
    final repo = LocalNotesRepository(db);
    await repo.upsertNote(
      LocalNote(
        id: realId,
        title: 'Borrada offline',
        content: 'cuerpo',
        visibility: 'private',
        updatedAt: DateTime.now(),
        ownerUserId: 'owner',
      ),
    );

    var exists = true;
    var offline = true;
    var deleteCalls = 0;
    notesHttpClientOverride = MockClient((r) async {
      if (r.method == 'DELETE' && r.url.path == '/notes/$realId') {
        deleteCalls++;
        if (offline) throw Exception('sin conexión');
        exists = false;
        return http.Response('', 204);
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': exists
              ? [
                  {'id': realId, 'title': 'Borrada offline', 'version': 1},
                ]
              : [],
        });
      }
      if (r.url.path == '/notes/reconcile') {
        return json({
          'removed_notes': 0,
          'removed_attachments': 0,
          'pending': 0,
        });
      }
      if (r.url.path == '/notes/$realId') {
        return json({
          'id': realId,
          'title': 'Borrada offline',
          'user_id': 'owner',
          'version': 1,
          'content': 'cuerpo',
          'attachments': <Map<String, dynamic>>[],
        });
      }
      return json({}, 404);
    });

    await pumpScreen(t, db);
    await waitFor(t, () => find.text('Borrada offline').evaluate().isNotEmpty);

    await t.tap(find.text('Borrada offline'));
    await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
    await scrollSheetTo(t, find.text('Eliminar'));
    await t.tap(find.text('Eliminar'));
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(NeobrutalistButton, 'ELIMINAR'));
    await waitFor(t, () => deleteCalls == 1);
    await waitFor(t, () => find.text('Borrada offline').evaluate().isEmpty);

    var tombstones = await t.runAsync(
      () => repo.getTombstones(ownerId: 'owner'),
    );
    expect(tombstones, isNotNull);
    expect(tombstones!.single.id, realId);
    expect(tombstones.single.remotePending, isTrue);
    expect(await t.runAsync(() => repo.getAllNotes(ownerId: 'owner')), isEmpty);

    // "Reiniciar": nueva pantalla con la misma BD en archivo. El backend aún
    // devuelve la nota, pero el tombstone persistente impide la resurrección.
    await t.pumpWidget(const SizedBox());
    await db.close();
    final db2 = AppDatabase.forTesting(NativeDatabase(dbFile));
    addTearDown(() async {
      try {
        await db2.close();
      } catch (_) {}
    });
    final repo2 = LocalNotesRepository(db2);
    await pumpScreen(t, db2);
    await t.pump(const Duration(milliseconds: 500));
    expect(find.text('Borrada offline'), findsNothing);

    // Reconectar y sincronizar: reintenta DELETE y converge el tombstone.
    offline = false;
    await tapSync(t);
    await waitFor(
      t,
      () => deleteCalls >= 2 && find.text('Borrada offline').evaluate().isEmpty,
    );
    for (var i = 0; i < 120; i++) {
      final ts = await t.runAsync(() => repo2.getTombstones(ownerId: 'owner'));
      if (ts != null && ts.isEmpty) break;
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 20)),
      );
      await t.pump(const Duration(milliseconds: 100));
    }

    tombstones = await t.runAsync(() => repo2.getTombstones(ownerId: 'owner'));
    expect(tombstones, isEmpty, reason: 'el servidor ya no la devuelve');
    expect(exists, isFalse);

    await t.pumpWidget(const SizedBox());
  });
}

bool callsContainOnlyOneNote(Map<String, Map<String, dynamic>> remote) =>
    remote.length == 1;
