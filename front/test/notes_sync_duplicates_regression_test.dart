import 'dart:async';
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

// Regresiones de la auditoría Sprint 3 (duplicados y persistencia de Notes).
//
// Causa raíz demostrada: la fila temporal (id epoch-ms) y la fila remota
// (UUID) podían coexistir en SQLite cuando el POST /notes no se confirmaba en
// el cliente aunque el backend sí creaba la nota (timeout de 8 s frente a un
// backend que tarda hasta ~30 s con Drive, o pantalla desmontada antes de la
// respuesta). El siguiente GET /notes/me insertaba el UUID y la lista mostraba
// la misma nota dos veces, también tras reiniciar la app.
void main() {
  const realId = '12345678-1234-4234-8234-123456789abc';
  const tempId = '1700000000000';
  const createKey = 'flutter-note-create-$tempId';

  String jwtFor(String userId) =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': userId, 'exp': 9999999999}))).replaceAll('=', '')}.test';

  Future<void> waitFor(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 150; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 20)),
      );
      await t.pump(const Duration(milliseconds: 100));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó');
  }

  // Igual que waitFor pero con el reloj virtual casi congelado: los timeouts
  // del cliente (virtuales) no se disparan mientras un mock bloquea el POST.
  Future<void> waitFrozen(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 400; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 5)),
      );
      await t.pump(const Duration(milliseconds: 10));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó (reloj congelado)');
  }

  /// Espera (IO real de Drift) hasta que [check] sea verdadero.
  Future<void> waitDb(WidgetTester t, Future<bool> Function() check) async {
    for (var i = 0; i < 150; i++) {
      final ok = await t.runAsync(check);
      if (ok == true) return;
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 20)),
      );
      await t.pump(const Duration(milliseconds: 50));
    }
    expect(await t.runAsync(check), isTrue, reason: 'estado SQLite no llegó');
  }

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Future<void> setupSession(WidgetTester t, {String user = 'owner'}) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwtFor(user), {'id': user});
    t.view.physicalSize = const Size(1400, 3000);
    t.view.devicePixelRatio = 1;
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
  }

  Future<void> pumpScreen(WidgetTester t, AppDatabase db) async {
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pump();
  }

  /// Nota local "temporal" con intención de creación persistida, tal como
  /// queda en SQLite después de un POST cuyo resultado no se confirmó.
  LocalNote pendingTemp({String title = 'Nota timeout', String? owner}) =>
      LocalNote(
        id: tempId,
        title: title,
        content: 'cuerpo',
        visibility: 'private',
        updatedAt: DateTime.now(),
        ownerUserId: owner ?? 'owner',
        createKey: createKey,
        createSnapshot: jsonEncode({
          'title': title,
          'content': 'cuerpo',
          'visibility': 'private',
        }),
      );

  Map<String, dynamic> listOf(Map<String, Map<String, dynamic>> remote) => {
    'notes': [
      for (final n in remote.values)
        {
          'id': n['id'],
          'title': n['title'],
          'version': n['version'],
          'user_id': n['user_id'],
        },
    ],
  };

  Map<String, dynamic> remoteNote(String id, String title) => {
    'id': id,
    'title': title,
    'user_id': 'owner',
    'version': 1,
    'content': 'cuerpo',
    'attachments': <Map<String, dynamic>>[],
  };

  testWidgets(
    '9/11. timeout tras crear en backend: al reabrir, UNA sincronización deja '
    'una sola nota (sin fila temporal + UUID) y no crea otra en el servidor',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);
      await repo.upsertNote(pendingTemp());

      // El backend YA creó la nota en el intento anterior (respuesta perdida).
      final remote = <String, Map<String, dynamic>>{
        realId: remoteNote(realId, 'Nota timeout'),
      };
      final postKeys = <String>[];
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'POST' && r.url.path == '/notes') {
          final key = r.headers['X-Idempotency-Key'] ?? '';
          postKeys.add(key);
          if (key == createKey) {
            // Replay idempotente de la intención ya completada.
            return json({'note_id': realId, 'version': 1}, 200);
          }
          remote['99999999-9999-4999-8999-999999999999'] = remoteNote(
            '99999999-9999-4999-8999-999999999999',
            'DUPLICADO',
          );
          return json({
            'note_id': '99999999-9999-4999-8999-999999999999',
            'version': 1,
          }, 201);
        }
        if (r.url.path == '/notes/me') return json(listOf(remote));
        if (r.url.path == '/notes/$realId') return json(remote[realId]!);
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await waitDb(t, () async {
        final rows = await repo.getAllNotes(ownerId: 'owner');
        return rows.length == 1 && rows.single.id == realId;
      });
      await waitFor(t, () => find.text('Nota timeout').evaluate().isNotEmpty);
      await t.pump(const Duration(milliseconds: 300));

      expect(find.text('Nota timeout'), findsOneWidget);
      expect(postKeys, [createKey], reason: 'un solo POST, misma clave');
      expect(remote.length, 1, reason: 'no se crea una segunda nota remota');
      final stored = await t.runAsync(() => repo.getAllNotes(ownerId: 'owner'));
      expect(stored!.single.id, realId);
      expect(stored.single.createKey, isNull, reason: 'intención cerrada');
      expect(stored.single.content, 'cuerpo');

      // 11. Recargar (nueva pantalla, misma BD): sigue habiendo una sola.
      await t.pumpWidget(const SizedBox());
      await pumpScreen(t, db);
      await waitFor(t, () => find.text('Nota timeout').evaluate().isNotEmpty);
      await t.pump(const Duration(milliseconds: 300));
      expect(find.text('Nota timeout'), findsOneWidget);
      expect(postKeys.length, 1, reason: 'la intención ya está cerrada');
      await t.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    '1. cambiar de ventana con el POST en vuelo: la fila temporal se promueve '
    'igual y al volver hay una sola nota',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);

      final postGate = Completer<void>();
      final remote = <String, Map<String, dynamic>>{};
      var posts = 0;
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'POST' && r.url.path == '/notes') {
          posts++;
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          await postGate.future;
          remote[realId] = remoteNote(realId, body['title'] as String);
          return json({'note_id': realId, 'version': 1}, 201);
        }
        if (r.url.path == '/notes/me') return json(listOf(remote));
        if (r.url.path == '/notes/$realId') return json(remote[realId]!);
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await t.pumpAndSettle();
      final headerBtn = find.widgetWithText(NeobrutalistButton, 'Nueva nota');
      if (headerBtn.evaluate().isNotEmpty) {
        await t.tap(headerBtn);
      } else {
        await t.tap(find.text('CREAR PRIMERA NOTA'));
      }
      await t.pumpAndSettle();
      await t.enterText(find.widgetWithText(TextField, 'Título'), 'En vuelo');
      await t.enterText(
        find.widgetWithText(TextField, 'Contenido Markdown'),
        'cuerpo',
      );
      await t.tap(find.widgetWithText(NeobrutalistButton, 'GUARDAR'));
      await waitFrozen(t, () => posts == 1);

      // El usuario navega a otra ventana: la pantalla se desmonta.
      await t.pumpWidget(const SizedBox());
      postGate.complete();

      await waitDb(t, () async {
        final rows = await repo.getAllNotes(ownerId: 'owner');
        return rows.length == 1 && rows.single.id == realId;
      });

      // Volver a la ventana de notas: una sola tarjeta y ningún POST extra.
      await pumpScreen(t, db);
      await waitFor(t, () => find.text('En vuelo').evaluate().isNotEmpty);
      await t.pump(const Duration(milliseconds: 300));
      expect(find.text('En vuelo'), findsOneWidget);
      expect(posts, 1);
      expect(remote.length, 1);
      await t.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    '3/6. crear offline, reiniciar y reconectar: se crea una sola vez con la '
    'clave persistida',
    (t) async {
      await setupSession(t);
      final dbFile = File(
        '${Directory.systemTemp.path}/notes_regr_offline_'
        '${DateTime.now().microsecondsSinceEpoch}.sqlite',
      );
      addTearDown(() {
        try {
          dbFile.deleteSync();
        } catch (_) {}
      });
      final db = AppDatabase.forTesting(NativeDatabase(dbFile));
      final repo = LocalNotesRepository(db);

      var online = false;
      final remote = <String, Map<String, dynamic>>{};
      final postKeys = <String>[];
      notesHttpClientOverride = MockClient((r) async {
        if (!online) throw const SocketException('sin red');
        if (r.method == 'POST' && r.url.path == '/notes') {
          postKeys.add(r.headers['X-Idempotency-Key'] ?? '');
          final body = jsonDecode(r.body) as Map<String, dynamic>;
          remote[realId] = remoteNote(realId, body['title'] as String);
          return json({'note_id': realId, 'version': 1}, 201);
        }
        if (r.url.path == '/notes/me') return json(listOf(remote));
        if (r.url.path == '/notes/$realId') return json(remote[realId]!);
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await t.pumpAndSettle();
      final headerBtn = find.widgetWithText(NeobrutalistButton, 'Nueva nota');
      if (headerBtn.evaluate().isNotEmpty) {
        await t.tap(headerBtn);
      } else {
        await t.tap(find.text('CREAR PRIMERA NOTA'));
      }
      await t.pumpAndSettle();
      await t.enterText(find.widgetWithText(TextField, 'Título'), 'Offline');
      await t.enterText(
        find.widgetWithText(TextField, 'Contenido Markdown'),
        'cuerpo',
      );
      await t.tap(find.widgetWithText(NeobrutalistButton, 'GUARDAR'));
      await waitFor(t, () => find.text('SOLO LOCAL').evaluate().isNotEmpty);

      final offlineRows = await t.runAsync(
        () => repo.getAllNotes(ownerId: 'owner'),
      );
      expect(offlineRows!.single.createKey, isNotNull);
      final persistedKey = offlineRows.single.createKey!;

      // Reinicio: se destruye la pantalla y se reabre la misma BD en archivo.
      await t.pumpWidget(const SizedBox());
      await t.runAsync(db.close);
      final db2 = AppDatabase.forTesting(NativeDatabase(dbFile));
      addTearDown(() async {
        try {
          await db2.close();
        } catch (_) {}
      });
      final repo2 = LocalNotesRepository(db2);
      online = true;
      await pumpScreen(t, db2);
      await waitDb(t, () async {
        final rows = await repo2.getAllNotes(ownerId: 'owner');
        return rows.length == 1 && rows.single.id == realId;
      });
      await waitFor(t, () => find.text('Offline').evaluate().isNotEmpty);
      await t.pump(const Duration(milliseconds: 300));
      expect(find.text('Offline'), findsOneWidget);
      expect(postKeys, [persistedKey]);
      expect(remote.length, 1);
      await t.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    '8. dos pantallas sincronizando a la vez sobre la misma BD: un solo POST y '
    'una sola fila',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);
      await repo.upsertNote(pendingTemp(title: 'Concurrente'));

      final postGate = Completer<void>();
      final remote = <String, Map<String, dynamic>>{};
      var posts = 0;
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'POST' && r.url.path == '/notes') {
          posts++;
          await postGate.future;
          remote[realId] = remoteNote(realId, 'Concurrente');
          return json({'note_id': realId, 'version': 1}, 201);
        }
        if (r.url.path == '/notes/me') return json(listOf(remote));
        if (r.url.path == '/notes/$realId') return json(remote[realId]!);
        return json({}, 404);
      });

      // Shell + vista por materia abiertas a la vez (misma base compartida).
      await t.pumpWidget(
        MaterialApp(
          home: Column(
            children: [
              Expanded(child: AllNotesScreen(database: db)),
              Expanded(child: AllNotesScreen(database: db)),
            ],
          ),
        ),
      );
      await waitFrozen(t, () => posts >= 1);
      // Ventana para que la segunda pantalla también intente su sync.
      for (var i = 0; i < 20; i++) {
        await t.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 5)),
        );
        await t.pump(const Duration(milliseconds: 10));
      }
      expect(posts, 1, reason: 'el segundo intento se une al POST en vuelo');
      postGate.complete();

      await waitDb(t, () async {
        final rows = await repo.getAllNotes(ownerId: 'owner');
        return rows.length == 1 && rows.single.id == realId;
      });
      await waitFor(t, () => find.text('Concurrente').evaluate().length == 2);
      await t.pump(const Duration(milliseconds: 300));
      // Una tarjeta por pantalla (2 pantallas), nunca duplicada dentro de una.
      expect(find.text('Concurrente'), findsNWidgets(2));
      expect(posts, 1);
      expect(remote.length, 1);
      await t.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    '5. borrar una nota SOLO LOCAL cuyo POST expiró pero sí se creó: se borra '
    'en el servidor y no resucita',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);
      await repo.upsertNote(pendingTemp(title: 'Fantasma'));

      // Primer render offline (la nota queda local), luego vuelve la red.
      var online = false;
      final remote = <String, Map<String, dynamic>>{
        realId: remoteNote(realId, 'Fantasma'),
      };
      final deletes = <String>[];
      notesHttpClientOverride = MockClient((r) async {
        if (!online) throw const SocketException('sin red');
        if (r.method == 'POST' && r.url.path == '/notes') {
          return json({'note_id': realId, 'version': 1}, 200);
        }
        if (r.method == 'DELETE') {
          deletes.add(r.url.path);
          remote.remove(realId);
          return http.Response('', 204);
        }
        if (r.url.path == '/notes/me') return json(listOf(remote));
        if (r.url.path == '/notes/$realId') {
          return remote.containsKey(realId)
              ? json(remote[realId]!)
              : json({}, 404);
        }
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await waitFor(t, () => find.text('Fantasma').evaluate().isNotEmpty);

      online = true;
      await t.tap(find.text('Fantasma'));
      await waitFor(t, () => find.text('Eliminar').evaluate().isNotEmpty);
      final list = find.descendant(
        of: find.byType(DraggableScrollableSheet),
        matching: find.byType(ListView),
      );
      await t.dragUntilVisible(
        find.text('Eliminar'),
        list.last,
        const Offset(0, -300),
      );
      await t.pumpAndSettle();
      await t.tap(find.text('Eliminar'));
      await t.pumpAndSettle();
      await t.tap(find.widgetWithText(NeobrutalistButton, 'ELIMINAR'));
      await waitFor(t, () => deletes.isNotEmpty);
      await waitFor(t, () => find.text('Fantasma').evaluate().isEmpty);

      expect(deletes, ['/notes/$realId']);
      expect(remote, isEmpty);

      // Reabrir: nada que resucitar, ni la fila temporal ni la remota.
      await t.pumpWidget(const SizedBox());
      await pumpScreen(t, db);
      await t.pump(const Duration(milliseconds: 500));
      await waitFor(t, () => find.text('Fantasma').evaluate().isEmpty);
      final rows = await t.runAsync(() => repo.getAllNotes(ownerId: 'owner'));
      expect(rows, isEmpty);
      await t.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    '7. cambio de cuenta con la pantalla abierta: no sincroniza con el token '
    'nuevo ni sella sus notas con el dueño anterior',
    (t) async {
      await setupSession(t, user: 'owner');
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);

      final listCalls = <String>[];
      notesHttpClientOverride = MockClient((r) async {
        if (r.url.path == '/notes/me') {
          final auth = r.headers['Authorization'] ?? '';
          listCalls.add(auth);
          if (auth.contains(jwtFor('intruso'))) {
            return json({
              'notes': [
                {
                  'id': '88888888-8888-4888-8888-888888888888',
                  'title': 'Nota de otra cuenta',
                  'version': 1,
                },
              ],
            });
          }
          return json({'notes': <Object>[]});
        }
        if (r.url.path == '/notes/reconcile') {
          return json({
            'removed_notes': 0,
            'removed_attachments': 0,
            'pending': 0,
          });
        }
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await waitFor(t, () => listCalls.isNotEmpty);
      await t.pumpAndSettle();

      // Otra cuenta inicia sesión mientras la pantalla sigue montada.
      await SessionManager.saveSession(jwtFor('intruso'), {'id': 'intruso'});
      final label = find.text('REFRESCAR / SINCRONIZAR DRIVE');
      if (label.evaluate().isNotEmpty) {
        await t.tap(label.first);
      } else {
        await t.tap(find.byTooltip('Refrescar / sincronizar Drive'));
      }
      await t.pump(const Duration(milliseconds: 500));

      expect(
        listCalls.where((a) => a.contains(jwtFor('intruso'))),
        isEmpty,
        reason: 'no se sincroniza con la sesión de otra cuenta',
      );
      final ownerRows = await t.runAsync(
        () => repo.getAllNotes(ownerId: 'owner'),
      );
      expect(ownerRows, isEmpty);
      expect(find.text('Nota de otra cuenta'), findsNothing);
      await t.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    'persistencia: GET /notes/me paginado trae todas las notas (no solo 50)',
    (t) async {
      await setupSession(t);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(db.close);
      final repo = LocalNotesRepository(db);

      String idFor(int i) =>
          '00000000-0000-4000-8000-${i.toString().padLeft(12, '0')}';
      final cursors = <String?>[];
      notesHttpClientOverride = MockClient((r) async {
        if (r.url.path == '/notes/me') {
          final cursor = r.url.queryParameters['cursor'];
          cursors.add(cursor);
          if (cursor == null) {
            return json({
              'notes': [
                for (var i = 0; i < 50; i++)
                  {'id': idFor(i), 'title': 'Nota $i', 'version': 1},
              ],
              'next_cursor': idFor(49),
            });
          }
          return json({
            'notes': [
              for (var i = 50; i < 55; i++)
                {'id': idFor(i), 'title': 'Nota $i', 'version': 1},
            ],
            'next_cursor': '',
          });
        }
        return json({}, 404);
      });

      await pumpScreen(t, db);
      await waitDb(t, () async {
        final rows = await repo.getAllNotes(ownerId: 'owner');
        return rows.length == 55;
      });
      expect(cursors, [null, idFor(49)]);
      await t.pumpWidget(const SizedBox());
    },
  );
}
