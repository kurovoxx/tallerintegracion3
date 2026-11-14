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

  group('NotesApi perfil público', () {
    const publicId = 'a8kp92mqwx7r';

    test('usa /public-users/:public_id/notes y parsea notes', () async {
      final api = await _api(
        MockClient((req) async {
          expect(req.url.path, '/public-users/$publicId/notes');
          expect(req.url.queryParameters['limit'], '20');
          return _json({
            'notes': [
              {
                'id': _a,
                'title': 'Cálculo',
                'visibility': 'public',
                ..._stats(_a, likes: 2, saves: 1, liked: true),
                'created_at': '2026-11-01T10:00:00Z',
                'updated_at': '2026-11-02T10:00:00Z',
              },
            ],
            'next_cursor': 'next',
          }, 200);
        }),
      );
      final page = await api.listPublicByPublicId(publicId);
      expect(page.items, hasLength(1));
      expect(page.items.single.title, 'Cálculo');
      expect(page.items.single.interactions.isLiked, isTrue);
      expect(page.nextCursor, 'next');
    });

    test('404 se traduce a profile_not_found', () async {
      final api = await _api(
        MockClient((req) async {
          return _json({
            'error': {'code': 'profile_not_found', 'message': 'x'},
          }, 404);
        }),
      );
      await expectLater(
        api.listPublicByPublicId(publicId),
        throwsA(
          isA<NotesApiException>()
              .having((e) => e.statusCode, 'statusCode', 404)
              .having((e) => e.code, 'code', 'profile_not_found'),
        ),
      );
    });

    test('rechaza UUID sin llamar al backend', () async {
      var calls = 0;
      final api = await _api(
        MockClient((req) async {
          calls++;
          return _json({}, 200);
        }),
      );
      await expectLater(
        api.listPublicByPublicId('550e8400-e29b-41d4-a716-446655440000'),
        throwsA(
          isA<NotesApiException>().having(
            (e) => e.code,
            'code',
            'invalid_public_id',
          ),
        ),
      );
      expect(calls, 0);
    });
  });

  group('NotesApi copia y vista previa', () {
    test('already_copied tiene un mensaje claro', () {
      expect(
        notesFriendlyMessage(409, 'already_copied'),
        'Ya copiaste este apunte a tus apuntes.',
      );
    });

    test('NoteDetail conserva los adjuntos de GET /notes/{id}', () {
      final detail = NoteDetail.fromJson({
        'id': _a,
        'title': 'Con imagen',
        'visibility': 'public',
        'content': '![](attachment:$_b)',
        'attachments': [
          {'id': _b, 'file_type': 'image/png', 'file_name': 'a.png'},
          'basura',
        ],
      });
      expect(detail.attachments, hasLength(1));
      expect(detail.attachments.single['id'], _b);
      // copyWith (like/guardar) no debe perder los adjuntos del visor.
      expect(detail.copyWith(isLiked: true).attachments, hasLength(1));
    });
  });
}
