// Copiar apuntes de otros usuarios exige Google Drive conectado. Antes, sin
// Drive, el backend dejaba un clon 'failed_sync' sin archivo que la app
// mostraba como apunte copiado (ilegible). Estas pruebas fijan:
//  - el mensaje claro de 403 drive_not_connected en todos los puntos de copia,
//    sin mensajes de éxito;
//  - que la sincronización no muestra ni conserva copias incompletas y no
//    anuncia éxitos inexistentes.
import 'support/service_uri.dart';
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
import 'package:taller_integracion_front/features/notes/note_id_search_screen.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/public_note_preview_screen.dart';
import 'package:taller_integracion_front/features/notes/saved_notes_screen.dart';

const _orig = '11111111-1111-4111-8111-111111111111';
const _ghost = '99999999-9999-4999-8999-999999999999';
const _mine = '12345678-1234-4234-8234-123456789abc';

http.Response _json(Object b, [int status = 200]) => http.Response(
  jsonEncode(b),
  status,
  headers: {'content-type': 'application/json; charset=utf-8'},
);

final _driveNotConnected = _json({
  'error': {
    'code': 'drive_not_connected',
    'message':
        'Para copiar apuntes de otros usuarios necesitas conectar tu Google Drive',
  },
}, 403);

Map<String, Object?> _detail() => {
  'id': _orig,
  'user_id': 'autor',
  'title': 'Apunte ajeno',
  'content': '# Hola',
  'visibility': 'public',
  'is_own': false,
  'attachments': [],
};

/// API que rechaza la copia por falta de Drive y registra los intentos.
NotesApi _api(List<String> copies) => NotesApi(
  baseUrl: 'http://notes',
  client: MockClient((r) async {
    if (r.method == 'POST' && r.url.path == '/notes/$_orig/copy') {
      copies.add(r.url.path);
      return _driveNotConnected;
    }
    if (r.url.path == '/notes/$_orig') return _json(_detail());
    if (r.url.path == '/notes/saved') {
      return _json({
        'items': [
          {
            'saved_id': 's1',
            'note_id': _orig,
            'title': 'Apunte ajeno',
            'is_available': true,
            'user_id': 'autor',
            'note_id_stats': _orig,
            'likes_count': 0,
            'saves_count': 1,
            'is_liked': false,
            'is_saved': true,
          },
        ],
        'next_cursor': '',
      });
    }
    return _json({}, 404);
  }),
);

void _expectNoSuccess() {
  for (final success in <String>[
    'Copia guardada en tus apuntes y en tu Drive.',
    'Copia guardada exitosamente en tu Drive',
    'COPIADO EN TUS APUNTES',
    'Copiado en tus apuntes',
  ]) {
    expect(find.text(success), findsNothing, reason: success);
  }
}

void main() {
  group('isIncompleteNoteCopy', () {
    test('clon sin archivo = copia incompleta', () {
      expect(
        isIncompleteNoteCopy({'id': _ghost, 'forked_from_note_id': _orig}),
        isTrue,
      );
      expect(
        isIncompleteNoteCopy({
          'id': _ghost,
          'forked_from_note_id': _orig,
          'external_file_id': ' ',
          'sync_status': 'failed_sync',
        }),
        isTrue,
      );
    });
    test('copia completa o apunte propio no lo son', () {
      expect(
        isIncompleteNoteCopy({
          'id': _ghost,
          'forked_from_note_id': _orig,
          'external_file_id': 'drive-1',
          'sync_status': 'synced',
        }),
        isFalse,
      );
      // Apunte propio sin archivo (creación pendiente): no se toca.
      expect(isIncompleteNoteCopy({'id': _mine}), isFalse);
    });
  });

  group('copiar sin Drive conectado', () {
    setUp(() async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession('jwt', {'id': 'u'});
    });
    tearDown(() async => SessionManager.clear());

    testWidgets('vista previa (perfil): explica y no anuncia éxito', (
      tester,
    ) async {
      final copies = <String>[];
      var copied = 0;
      await tester.pumpWidget(
        MaterialApp(
          home: PublicNotePreviewScreen(
            noteId: _orig,
            api: _api(copies),
            onNoteCopied: () => copied++,
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('note-preview-copy')));
      await tester.pumpAndSettle();
      expect(copies, hasLength(1));
      expect(find.text(kDriveRequiredToCopyMessage), findsOneWidget);
      expect(copied, 0);
      _expectNoSuccess();
      // El botón sigue disponible para reintentar tras conectar Drive.
      expect(find.text('COPIAR A MIS APUNTES'), findsOneWidget);
    });

    testWidgets('búsqueda por ID: explica y no anuncia éxito', (tester) async {
      final copies = <String>[];
      var copied = 0;
      await tester.pumpWidget(
        MaterialApp(
          home: NoteIdSearchScreen(
            api: _api(copies),
            initialQuery: _orig,
            onNoteCopied: () => copied++,
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('note-search-copy')));
      await tester.pumpAndSettle();
      expect(copies, hasLength(1));
      expect(find.text(kDriveRequiredToCopyMessage), findsOneWidget);
      expect(copied, 0);
      _expectNoSuccess();
    });

    testWidgets('guardados: explica y no anuncia éxito', (tester) async {
      final copies = <String>[];
      var copied = 0;
      await tester.pumpWidget(
        MaterialApp(
          home: SavedNotesScreen(
            api: _api(copies),
            onNoteCopied: () => copied++,
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('COPIAR A MIS APUNTES'));
      await tester.pumpAndSettle();
      expect(copies, hasLength(1));
      expect(find.text(kDriveRequiredToCopyMessage), findsOneWidget);
      expect(copied, 0);
      _expectNoSuccess();
    });
  });

  group('sincronización con una copia incompleta', () {
    String jwtFor(String userId) =>
        'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': userId, 'exp': 9999999999}))).replaceAll('=', '')}.test';

    Finder syncButton() => find.widgetWithText(
      NeobrutalistButton,
      'REFRESCAR / SINCRONIZAR DRIVE',
    );

    LocalNote local(String id, String title, {bool needsSync = false}) =>
        LocalNote(
          needsSync: needsSync,
          id: id,
          title: title,
          content: id == _ghost ? '' : 'contenido',
          visibility: 'private',
          updatedAt: DateTime.now(),
          ownerUserId: 'owner',
          version: 1,
        );

    Future<(AppDatabase, LocalNotesRepository)> seed(
      List<LocalNote> notes,
    ) async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession(jwtFor('owner'), {'id': 'owner'});
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      final repo = LocalNotesRepository(db);
      for (final n in notes) {
        await repo.upsertNote(n);
      }
      return (db, repo);
    }

    Future<void> pumpScreen(WidgetTester t, AppDatabase db) async {
      t.view.physicalSize = const Size(1400, 1800);
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.reset);
      addTearDown(() async {
        notesHttpClientOverride = null;
        debugClearPendingAttachments();
        await SessionManager.clear();
      });
      await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
      await t.pumpAndSettle();
    }

    Future<void> settle(WidgetTester t) async {
      for (var i = 0; i < 20; i++) {
        await t.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 30)),
        );
        await t.pump(const Duration(milliseconds: 100));
      }
    }

    MockClient server(List<String> calls, {http.Response? reconcile}) =>
        MockClient((r) async {
          calls.add('${r.method} ${r.url.servicePath}');
          if (r.method == 'POST' && r.url.servicePath == '/notes/reconcile') {
            if (reconcile != null) return reconcile;
            return _json({
              'removed_notes': 0,
              'removed_attachments': 0,
              'pending': 0,
              'removed_note_ids': [],
            });
          }
          if (r.url.servicePath == '/notes/me') {
            return _json({
              'notes': [
                {
                  'id': _mine,
                  'title': 'Mi apunte',
                  'version': 1,
                  'external_file_id': 'drive-mine',
                  'sync_status': 'synced',
                },
                {
                  'id': _ghost,
                  'title': 'Apunte ajeno',
                  'version': 1,
                  'forked_from_note_id': _orig,
                  'sync_status': 'failed_sync',
                },
              ],
            });
          }
          return _json({}, 404);
        });

    testWidgets(
      'no se muestra, se retira de la caché y el refresco no anuncia copias',
      (t) async {
        final (db, repo) = await seed([
          local(_mine, 'Mi apunte'),
          local(_ghost, 'Apunte ajeno'), // cacheado por la versión anterior
        ]);
        final calls = <String>[];
        notesHttpClientOverride = server(calls);
        await pumpScreen(t, db);
        await settle(t);

        expect(find.text('Mi apunte'), findsWidgets);
        expect(find.text('Apunte ajeno'), findsNothing);
        expect(await repo.getNoteById(_ghost), isNull);
        expect(await repo.getNoteById(_mine), isNotNull);
        // Sin tombstone: si la copia se completa, volverá a aparecer.
        final tombstones = await repo.getTombstones(ownerId: 'owner');
        expect(tombstones.where((x) => x.id == _ghost), isEmpty);

        // REFRESCAR: nada pendiente → mensaje neutro, ningún «copiado».
        await t.ensureVisible(syncButton());
        await t.tap(syncButton());
        await settle(t);
        expect(find.text('Tus notas están sincronizadas.'), findsOneWidget);
        expect(find.textContaining('copiad'), findsNothing);
        expect(find.textContaining('Copia guardada'), findsNothing);
        expect(find.text('Apunte ajeno'), findsNothing);
        // Nunca se intentó escribir sobre la copia incompleta.
        expect(calls.where((c) => c.contains(_ghost)), isEmpty);
        expect(t.takeException(), isNull);
      },
    );

    testWidgets('sin Drive, sincronizar no reporta apuntes con error', (
      t,
    ) async {
      final (db, _) = await seed([local(_mine, 'Mi apunte')]);
      notesHttpClientOverride = server(
        <String>[],
        reconcile: _json({
          'error': {
            'code': 'drive_not_connected',
            'message':
                'Para copiar apuntes de otros usuarios necesitas '
                'conectar tu Google Drive',
          },
        }, 403),
      );
      await pumpScreen(t, db);
      await settle(t);
      await t.ensureVisible(syncButton());
      await t.tap(syncButton());
      await settle(t);
      expect(find.textContaining('con error'), findsNothing);
      expect(find.textContaining('parcial'), findsNothing);
      expect(
        find.textContaining('Tus apuntes de este equipo están al día'),
        findsOneWidget,
      );
    });

    testWidgets('un error real del servicio sí se sigue informando', (t) async {
      final (db, _) = await seed([local(_mine, 'Mi apunte')]);
      notesHttpClientOverride = server(
        <String>[],
        reconcile: _json({
          'error': {'code': 'internal_error', 'message': 'x'},
        }, 500),
      );
      await pumpScreen(t, db);
      await settle(t);
      await t.ensureVisible(syncButton());
      await t.tap(syncButton());
      await settle(t);
      expect(find.textContaining('1 con error'), findsOneWidget);
    });

    testWidgets('con ediciones locales sin subir se conserva', (t) async {
      final (db, repo) = await seed([
        local(_ghost, 'Apunte ajeno', needsSync: true),
      ]);
      notesHttpClientOverride = server(<String>[]);
      await pumpScreen(t, db);
      await settle(t);
      expect(await repo.getNoteById(_ghost), isNotNull);
    });
  });
}
