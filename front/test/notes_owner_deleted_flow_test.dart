import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/note_id_search_screen.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/saved_notes_screen.dart';

const _noteId = '11111111-1111-4111-8111-111111111111';
const _title = 'Cálculo II - Series';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

http.Response _gone() => _json({
  'error': {
    'code': 'note_deleted',
    'message': 'Apunte no disponible (eliminado por el autor)',
  },
}, 410);

/// Backend de Notes visto desde el usuario B. [ownerDeletes] simula que el
/// autor (A) borra su apunte público mientras B lo tiene guardado.
class _NotesBackend {
  bool deleted = false;
  bool saved = true;
  final List<String> calls = [];

  void ownerDeletes() => deleted = true;

  Future<http.Response> handle(http.Request req) async {
    calls.add('${req.method} ${req.url.path}');
    final path = req.url.path;
    if (path == '/notes/saved') {
      if (!saved) return _json({'items': [], 'next_cursor': ''}, 200);
      return _json({
        'items': [
          deleted
              ? {
                  'saved_id': 's1',
                  'note_id': _noteId,
                  'title': _title,
                  'is_available': false,
                  'unavailable_reason': 'deleted',
                  'is_saved': true,
                  'deleted_at': '2026-11-05T20:00:00Z',
                }
              : {
                  'saved_id': 's1',
                  'note_id': _noteId,
                  'title': _title,
                  'is_available': true,
                  'user_id': 'aaaaaaaa-0000-4000-8000-00000000000a',
                  'likes_count': 0,
                  'saves_count': 1,
                  'is_saved': true,
                },
        ],
        'next_cursor': '',
      }, 200);
    }
    if (path == '/notes/$_noteId/save' && req.method == 'DELETE') {
      saved = false;
      return http.Response('', 204);
    }
    if (deleted) return _gone();
    if (path == '/notes/$_noteId' && req.method == 'GET') {
      return _json({
        'id': _noteId,
        'title': _title,
        'content': '# Series',
        'visibility': 'public',
      }, 200);
    }
    return _json({}, 404);
  }
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  testWidgets(
    'A borra el apunte que B guardó: B lo ve no disponible y solo puede quitarlo',
    (tester) async {
      await SessionManager.saveSession('jwt-b', {'id': 'b'});
      addTearDown(SessionManager.clear);
      final backend = _NotesBackend();
      final api = NotesApi(
        client: MockClient(backend.handle),
        baseUrl: 'http://n',
      );

      // 1. B ve su guardado disponible, con "Copiar a mis apuntes".
      await tester.pumpWidget(MaterialApp(home: SavedNotesScreen(api: api)));
      await tester.pumpAndSettle();
      expect(find.text(_title), findsOneWidget);
      expect(find.byKey(const Key('copy-$_noteId')), findsOneWidget);

      // 2. A elimina el apunte; B recarga sus guardados.
      backend.ownerDeletes();
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(MaterialApp(home: SavedNotesScreen(api: api)));
      await tester.pumpAndSettle();
      final card = find.byKey(const Key('saved-unavailable-$_noteId'));
      expect(card, findsOneWidget);
      expect(
        find.descendant(
          of: card,
          matching: find.text('APUNTE NO DISPONIBLE (ELIMINADO POR EL AUTOR)'),
        ),
        findsOneWidget,
      );
      expect(find.byKey(const Key('copy-$_noteId')), findsNothing);
      expect(find.byKey(const Key('like-$_noteId')), findsNothing);
      expect(find.byKey(const Key('unsave-$_noteId')), findsOneWidget);

      // 3. Si B intenta abrirlo por ID: aviso controlado, sin crash.
      await tester.pumpWidget(
        MaterialApp(
          home: NoteIdSearchScreen(api: api, initialQuery: _noteId),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('APUNTE NO DISPONIBLE'), findsOneWidget);
      expect(find.textContaining('su autor lo eliminó'), findsOneWidget);
      expect(find.byKey(const Key('note-search-result')), findsNothing);
      expect(tester.takeException(), isNull);

      // 4. Copiarlo responde el error controlado (410 note_deleted).
      await expectLater(
        api.copyNote(_noteId),
        throwsA(
          isA<NotesApiException>()
              .having((e) => e.statusCode, 'status', 410)
              .having((e) => e.code, 'code', 'note_deleted'),
        ),
      );

      // 5. Quitarlo de guardados funciona y deja la lista vacía.
      await tester.pumpWidget(MaterialApp(home: SavedNotesScreen(api: api)));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('unsave-$_noteId')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('saved-unavailable-$_noteId')), findsNothing);
      expect(backend.calls, contains('DELETE /notes/$_noteId/save'));
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(MaterialApp(home: SavedNotesScreen(api: api)));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('saved-notes-empty')), findsOneWidget);
    },
  );
}
