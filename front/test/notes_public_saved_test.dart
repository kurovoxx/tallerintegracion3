import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/public_notes_screen.dart';

const _a = '11111111-1111-4111-8111-111111111111';
const _b = '22222222-2222-4222-8222-222222222222';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

Map<String, Object> _stats(
  String id, {
  int likes = 0,
  int saves = 0,
  bool liked = false,
  bool saved = false,
}) => {
  'note_id': id,
  'likes_count': likes,
  'saves_count': saves,
  'is_liked': liked,
  'is_saved': saved,
};

Future<NotesApi> _api(MockClient client) async {
  await SessionManager.saveSession('jwt-test', {'id': 'u'});
  addTearDown(SessionManager.clear);
  return NotesApi(client: client, baseUrl: 'http://notes');
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  group('NotesApi likes/guardados idempotentes', () {
    test('like repetido (409) devuelve el estado real sin error', () async {
      final calls = <String>[];
      final api = await _api(
        MockClient((req) async {
          calls.add('${req.method} ${req.url.path}');
          if (req.method == 'POST') {
            return _json({
              'error': {'code': 'already_liked', 'message': 'Nota ya likeada'},
            }, 409);
          }
          return _json(_stats(_a, likes: 1, liked: true), 200);
        }),
      );
      final s = await api.setLiked(_a, true);
      expect(s.isLiked, isTrue);
      expect(s.likesCount, 1);
      expect(calls, ['POST /notes/$_a/like', 'GET /notes/$_a/stats']);
    });

    test('guardar usa los contadores del 201', () async {
      final api = await _api(
        MockClient((req) async {
          expect(req.method, 'POST');
          return _json(_stats(_a, saves: 3, saved: true), 201);
        }),
      );
      final s = await api.setSaved(_a, true);
      expect(s!.isSaved, isTrue);
      expect(s.savesCount, 3);
    });

    test('quitar de guardados un apunte borrado devuelve null', () async {
      final api = await _api(
        MockClient((req) async {
          if (req.method == 'DELETE') return http.Response('', 204);
          return _json({
            'error': {'code': 'note_deleted', 'message': 'x'},
          }, 410);
        }),
      );
      expect(await api.setSaved(_a, false), isNull);
    });
  });

  test('listSaved envía filtros y parsea no disponibles', () async {
    final api = await _api(
      MockClient((req) async {
        expect(req.url.path, '/notes/saved');
        expect(req.url.queryParameters['scope'], 'others');
        expect(req.url.queryParameters['q'], 'integral');
        expect(req.url.queryParameters['availability'], isNull);
        return _json({
          'items': [
            {
              'saved_id': 's1',
              'note_id': _a,
              'title': 'Integrales',
              'is_available': true,
              'likes_count': 2,
              'is_saved': true,
            },
            {
              'saved_id': 's2',
              'note_id': _b,
              'title': 'Borrado',
              'is_available': false,
              'unavailable_reason': 'deleted',
              'is_saved': true,
            },
          ],
          'next_cursor': 's2',
        }, 200);
      }),
    );
    final page = await api.listSaved(
      filter: const SavedNotesFilter(query: 'integral', scope: 'others'),
    );
    expect(page.items, hasLength(2));
    expect(page.items.first.interactions.likesCount, 2);
    expect(page.items.last.isAvailable, isFalse);
    expect(page.items.last.isDeleted, isTrue);
    expect(page.hasMore, isTrue);
  });

  testWidgets('públicos: like y filtro Guardados / No guardados', (
    tester,
  ) async {
    final api = await _api(
      MockClient((req) async {
        if (req.url.path == '/notes/public') {
          return _json({
            'items': [
              {
                'id': _a,
                'user_id': 'autor-uno',
                'title': 'Álgebra',
                ..._stats(_a, saved: true),
              },
              {
                'id': _b,
                'user_id': 'autor-dos',
                'title': 'Redes',
                ..._stats(_b),
              },
            ],
            'next_cursor': '',
          }, 200);
        }
        if (req.url.path == '/notes/$_b/like' && req.method == 'POST') {
          return _json(_stats(_b, likes: 1, liked: true), 201);
        }
        return _json({}, 404);
      }),
    );
    await tester.pumpWidget(MaterialApp(home: PublicNotesScreen(api: api)));
    await tester.pumpAndSettle();
    expect(find.text('Álgebra'), findsOneWidget);
    expect(find.text('Redes'), findsOneWidget);

    await tester.tap(find.byKey(const Key('like-$_b')));
    await tester.pumpAndSettle();
    expect(
      find.descendant(
        of: find.byKey(const Key('like-$_b')),
        matching: find.text('1'),
      ),
      findsOneWidget,
    );

    await tester.tap(find.byKey(const Key('filter-Guardados')));
    await tester.pumpAndSettle();
    expect(find.text('Álgebra'), findsOneWidget);
    expect(find.text('Redes'), findsNothing);

    await tester.tap(find.byKey(const Key('filter-No guardados')));
    await tester.pumpAndSettle();
    expect(find.text('Álgebra'), findsNothing);
    expect(find.text('Redes'), findsOneWidget);
  });
}
