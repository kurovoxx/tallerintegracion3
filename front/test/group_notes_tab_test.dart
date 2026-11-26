import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;
import 'package:flutter/rendering.dart';
import 'support/service_uri.dart';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/notes_sharing_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/groups/group_notes_tab.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart'
    show NotesApi;

const _gid = '11111111-1111-1111-1111-111111111111';
const _noteId = '22222222-2222-4222-8222-222222222222';

/// Backend de notas fake con estado: compartir/unshare alternan el listado.
class _FakeNotesApi {
  bool shared;
  bool failList;
  String content;

  /// Estado de like real del backend falso (contador + corazón propio).
  int likes = 3;
  bool liked = false;

  /// true = POST/DELETE /like responde 500 (para probar la reversión).
  bool failLike = false;

  Map<String, Object> _stats() => {
    'note_id': _noteId,
    'likes_count': likes,
    'saves_count': 0,
    'is_liked': liked,
    'is_saved': false,
  };

  _FakeNotesApi({
    this.shared = false,
    this.failList = false,
    this.content = 'cuerpo markdown',
  });

  http.Response _json(int code, Object body) => http.Response(
    jsonEncode(body),
    code,
    headers: {'content-type': 'application/json'},
  );

  Future<http.Response> handle(http.Request req) async {
    final p = req.url.servicePath;
    if (req.method == 'GET' && p.endsWith('/groups/$_gid/notes')) {
      if (failList) {
        return _json(403, {
          'error': {'code': 'forbidden', 'message': 'denegado'},
        });
      }
      final notes = shared
          ? [
              {
                'id': _noteId,
                'title': 'Nota compartida',
                'visibility': 'private',
                'likes_count': likes,
                'is_liked': liked,
                'version': 1,
                'user_id': 'u',
              },
            ]
          : [];
      return _json(200, {'notes': notes, 'next_cursor': ''});
    }
    if (req.method == 'GET' && p.endsWith('/notes/me')) {
      return _json(200, {
        'notes': [
          {'id': _noteId, 'title': 'Mi nota', 'visibility': 'private'},
        ],
        'next_cursor': '',
      });
    }
    if (p.endsWith('/notes/$_noteId/like')) {
      if (failLike) {
        return _json(500, {
          'error': {'code': 'internal_error', 'message': 'Error interno'},
        });
      }
      if (req.method == 'POST') {
        if (liked) {
          return _json(409, {
            'error': {'code': 'already_liked', 'message': 'ya'},
          });
        }
        liked = true;
        likes++;
        return _json(201, _stats());
      }
      if (req.method == 'DELETE') {
        if (liked) {
          liked = false;
          likes--;
        }
        return http.Response('', 204);
      }
    }
    if (req.method == 'GET' && p.endsWith('/notes/$_noteId/stats')) {
      return _json(200, _stats());
    }
    if (req.method == 'POST' && p.endsWith('/notes/$_noteId/share')) {
      shared = true;
      return _json(201, {'shared_note_id': 's1'});
    }
    if (req.method == 'DELETE' && p.contains('/notes/shared/')) {
      shared = false;
      return http.Response('', 204);
    }
    if (req.method == 'GET' && p.endsWith('/notes/$_noteId')) {
      return _json(200, {
        'id': _noteId,
        'title': 'Nota compartida',
        'content': content,
        'visibility': 'private',
      });
    }
    return http.Response('not found', 404);
  }
}

Future<void> _pumpTab(WidgetTester tester, NotesSharingService service) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: GroupNotesTab(groupId: _gid, service: service),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  group('likes en notas del grupo', () {
    Future<_FakeNotesApi> pumpWithLikes(
      WidgetTester tester, {
      bool liked = false,
    }) async {
      tester.view.physicalSize = const Size(1280, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = _FakeNotesApi(shared: true)..liked = liked;
      final client = MockClient(api.handle);
      final service = NotesSharingService(client: client);
      final notesApi = NotesApi(client: client);
      addTearDown(service.dispose);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: GroupNotesTab(
              groupId: _gid,
              service: service,
              notesApi: notesApi,
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      return api;
    }

    final likeKey = find.byKey(const Key('like-$_noteId'));

    testWidgets('la tarjeta muestra el like propio que trae el listado', (
      tester,
    ) async {
      await pumpWithLikes(tester, liked: true);
      expect(likeKey, findsOneWidget);
      expect(
        find.descendant(of: likeKey, matching: find.text('3')),
        findsOneWidget,
      );
      expect(
        find.descendant(
          of: likeKey,
          matching: find.byIcon(Icons.favorite_rounded),
        ),
        findsOneWidget,
      );
      // El badge estático "N likes" ya no existe.
      expect(find.textContaining('LIKES'), findsNothing);
    });

    testWidgets('dar y quitar like desde la tarjeta usa el backend real', (
      tester,
    ) async {
      final api = await pumpWithLikes(tester);
      expect(
        find.descendant(
          of: likeKey,
          matching: find.byIcon(Icons.favorite_border_rounded),
        ),
        findsOneWidget,
      );
      await tester.tap(likeKey);
      await tester.pumpAndSettle();
      expect(api.liked, isTrue);
      expect(find.descendant(of: likeKey, matching: find.text('4')), findsOne);
      expect(
        find.descendant(
          of: likeKey,
          matching: find.byIcon(Icons.favorite_rounded),
        ),
        findsOneWidget,
      );

      await tester.tap(likeKey);
      await tester.pumpAndSettle();
      expect(api.liked, isFalse);
      expect(find.descendant(of: likeKey, matching: find.text('3')), findsOne);
      expect(tester.takeException(), isNull);
    });

    testWidgets('like en el visor se refleja en la tarjeta al cerrar', (
      tester,
    ) async {
      final api = await pumpWithLikes(tester);
      await tester.tap(find.text('ABRIR'));
      await tester.pumpAndSettle();
      // Tarjeta (detrás) + visor: el del visor es el último.
      expect(likeKey, findsNWidgets(2));
      await tester.tap(likeKey.last);
      await tester.pumpAndSettle();
      expect(api.liked, isTrue);
      await tester.tap(find.widgetWithText(NeobrutalistButton, 'CERRAR').last);
      await tester.pumpAndSettle();
      expect(likeKey, findsOneWidget);
      expect(find.descendant(of: likeKey, matching: find.text('4')), findsOne);
    });

    testWidgets('si el like falla se revierte y se avisa', (tester) async {
      final api = await pumpWithLikes(tester);
      api.failLike = true;
      await tester.tap(likeKey);
      await tester.pumpAndSettle();
      expect(api.liked, isFalse);
      expect(find.descendant(of: likeKey, matching: find.text('3')), findsOne);
      expect(find.byType(SnackBar), findsOneWidget);
    });

    testWidgets('el like sigue visible al cambiar de pestaña y volver', (
      tester,
    ) async {
      tester.view.physicalSize = const Size(1280, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = _FakeNotesApi(shared: true);
      final client = MockClient(api.handle);
      final service = NotesSharingService(client: client);
      addTearDown(service.dispose);
      final controller = TabController(length: 2, vsync: tester);
      addTearDown(controller.dispose);
      // Igual que GroupDetailScreen: TabBarView destruye la pestaña al salir.
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: TabBarView(
              controller: controller,
              children: [
                GroupNotesTab(
                  groupId: _gid,
                  service: service,
                  notesApi: NotesApi(client: client),
                ),
                const Center(child: Text('OTRA PESTAÑA')),
              ],
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(likeKey);
      await tester.pumpAndSettle();
      expect(api.liked, isTrue);

      controller.animateTo(1);
      await tester.pumpAndSettle();
      expect(find.text('OTRA PESTAÑA'), findsOneWidget);
      controller.animateTo(0);
      await tester.pumpAndSettle();

      expect(find.descendant(of: likeKey, matching: find.text('4')), findsOne);
      expect(
        find.descendant(
          of: likeKey,
          matching: find.byIcon(Icons.favorite_rounded),
        ),
        findsOneWidget,
      );
    });
  });

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    SessionManager.saveSession('jwt-test', {'id': 'u'});
  });
  tearDown(SessionManager.clear);

  testWidgets(
    'nota compartida con Markdown y LaTeX se ve renderizada, no en crudo',
    (tester) async {
      // Sesión vía setUp(), igual que el resto de pruebas del archivo.
      tester.view.physicalSize = const Size(1280, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = _FakeNotesApi(
        shared: true,
        content:
            '# Resumen de grafos\n\nTexto con **negrita** y \$x^2\$.\n\n'
            '\$\$\\frac{a}{b}\$\$\n\n- punto uno',
      );
      final service = NotesSharingService(client: MockClient(api.handle));
      addTearDown(service.dispose);

      await _pumpTab(tester, service);
      await tester.tap(find.text('ABRIR'));
      await tester.pumpAndSettle();

      // Pasa por el mismo motor que el visor personal (NoteMarkdownBody).
      expect(find.byKey(const Key('group-note-markdown')), findsOneWidget);
      // Los símbolos de sintaxis no se muestran como texto plano.
      expect(find.textContaining('# Resumen'), findsNothing);
      expect(find.textContaining('**negrita**'), findsNothing);
      expect(find.textContaining(r'$$'), findsNothing);
      expect(
        find.textContaining('Resumen de grafos', findRichText: true),
        findsOneWidget,
      );
      expect(tester.takeException(), isNull);
    },
  );

  for (final size in [
    const Size(360, 640),
    const Size(768, 1024),
    const Size(1440, 900),
  ]) {
    for (final scale in [1.0, 2.0]) {
      testWidgets(
        'notas compartidas ${size.width} escala $scale: estados y diálogo',
        (tester) async {
          tester.view.physicalSize = size;
          tester.view.devicePixelRatio = 1;
          addTearDown(tester.view.resetPhysicalSize);
          addTearDown(tester.view.resetDevicePixelRatio);
          final api = _FakeNotesApi(shared: true);
          final gate = Completer<http.Response>();
          var loading = true;
          final service = NotesSharingService(
            client: MockClient((r) async {
              if (loading) return gate.future;
              return api.handle(r);
            }),
          );
          addTearDown(service.dispose);
          final boundaryKey = GlobalKey();
          Future<void> mount() => tester.pumpWidget(
            MaterialApp(
              builder: (context, child) => MediaQuery(
                data: MediaQuery.of(
                  context,
                ).copyWith(textScaler: TextScaler.linear(scale)),
                child: child!,
              ),
              home: RepaintBoundary(
                key: boundaryKey,
                child: Scaffold(
                  body: GroupNotesTab(
                    key: UniqueKey(),
                    groupId: _gid,
                    service: service,
                  ),
                ),
              ),
            ),
          );
          await mount();
          await tester.pump();
          expect(find.byType(CircularProgressIndicator), findsOneWidget);
          expect(tester.takeException(), isNull);
          loading = false;
          gate.complete(
            http.Response(
              jsonEncode({'notes': [], 'next_cursor': ''}),
              200,
              headers: {'content-type': 'application/json'},
            ),
          );
          await tester.pumpAndSettle();
          expect(
            find.text('Aún no hay notas compartidas con este grupo.'),
            findsOneWidget,
          );
          expect(tester.takeException(), isNull);
          await mount();
          await tester.pumpAndSettle();
          expect(find.text('Nota compartida'), findsOneWidget);
          expect(tester.takeException(), isNull);
          if (const bool.fromEnvironment('SPRINT3_CAPTURE') && scale == 1) {
            final boundary =
                boundaryKey.currentContext!.findRenderObject()
                    as RenderRepaintBoundary;
            await tester.runAsync(() async {
              final image = await boundary.toImage(pixelRatio: 1);
              final bytes = await image.toByteData(
                format: ui.ImageByteFormat.png,
              );
              await File(
                '../.git/sprint3-evidence/shared-${size.width.toInt()}.png',
              ).writeAsBytes(bytes!.buffer.asUint8List());
              image.dispose();
            });
          }
          await tester.tap(find.text('ABRIR'));
          await tester.pumpAndSettle();
          expect(find.text('cuerpo markdown'), findsOneWidget);
          expect(tester.takeException(), isNull);
          await tester.tap(
            find.widgetWithText(NeobrutalistButton, 'CERRAR').last,
          );
          await tester.pumpAndSettle();
          await tester.tap(find.text('COMPARTIR UNA NOTA'));
          await tester.pumpAndSettle();
          // A 200 % el botón queda bajo la barra de acciones: se desplaza
          // el contenido como haría el usuario antes de tocarlo.
          await tester.ensureVisible(find.text('RESTRICTED'));
          await tester.pumpAndSettle();
          await tester.tap(find.text('RESTRICTED'));
          await tester.pumpAndSettle();
          expect(find.text('Mi nota'), findsOneWidget);
          expect(tester.takeException(), isNull);
          await tester.tap(find.text('CANCELAR'));
          await tester.pumpAndSettle();
          api.failList = true;
          await mount();
          await tester.pumpAndSettle();
          expect(find.text('REINTENTAR'), findsOneWidget);
          expect(tester.takeException(), isNull);
        },
      );
    }
  }

  testWidgets('lista las notas compartidas del grupo', (tester) async {
    final api = _FakeNotesApi(shared: true);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);

    expect(find.text('NOTAS COMPARTIDAS'), findsOneWidget);
    expect(find.text('Nota compartida'), findsOneWidget);
    expect(find.text('ABRIR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('estado vacío cuando el grupo no tiene notas', (tester) async {
    final api = _FakeNotesApi(shared: false);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);

    expect(
      find.text('Aún no hay notas compartidas con este grupo.'),
      findsOneWidget,
    );
    expect(find.text('COMPARTIR UNA NOTA'), findsOneWidget);
  });

  testWidgets('error de carga muestra REINTENTAR y recupera', (tester) async {
    final api = _FakeNotesApi(shared: true, failList: true);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);
    expect(find.text('REINTENTAR'), findsOneWidget);
    expect(find.text('Nota compartida'), findsNothing);

    api.failList = false;
    await tester.tap(find.text('REINTENTAR'));
    await tester.pumpAndSettle();
    expect(find.text('Nota compartida'), findsOneWidget);
  });

  testWidgets('compartir (link) refresca y permite dejar de compartir', (
    tester,
  ) async {
    final api = _FakeNotesApi(shared: false);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);
    expect(find.text('COMPARTIR UNA NOTA'), findsOneWidget);

    // Abrir diálogo de compartir (carga mis notas) y confirmar en modo LINK.
    await tester.tap(find.text('COMPARTIR UNA NOTA'));
    await tester.pumpAndSettle();
    expect(find.text('LINK'), findsOneWidget);
    expect(find.text('RESTRICTED'), findsOneWidget);

    await tester.tap(find.text('COMPARTIR'));
    await tester.pumpAndSettle();

    // Refresco: la nota ya aparece como compartida + acción de unshare.
    expect(find.text('Nota compartida'), findsOneWidget);
    expect(find.text('DEJAR DE COMPARTIR'), findsOneWidget);

    // Unshare con confirmación.
    await tester.tap(find.text('DEJAR DE COMPARTIR'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('RETIRAR'));
    await tester.pumpAndSettle();

    expect(find.text('Nota compartida'), findsNothing);
    expect(
      find.text('Aún no hay notas compartidas con este grupo.'),
      findsOneWidget,
    );
  });

  testWidgets('diálogo: nota YA COMPARTIDA queda deshabilitada', (
    tester,
  ) async {
    const otherId = '33333333-3333-4333-8333-333333333333';
    final posts = <String>[];
    http.Response json(int code, Object body) => http.Response(
      jsonEncode(body),
      code,
      headers: {'content-type': 'application/json'},
    );
    final service = NotesSharingService(
      client: MockClient((req) async {
        final p = req.url.servicePath;
        if (req.method == 'GET' && p.endsWith('/groups/$_gid/notes')) {
          return json(200, {
            'notes': [
              {'id': _noteId, 'title': 'Ya en el grupo', 'user_id': 'u'},
            ],
            'next_cursor': '',
          });
        }
        if (req.method == 'GET' && p.endsWith('/notes/me')) {
          return json(200, {
            'notes': [
              {'id': _noteId, 'title': 'Ya en el grupo'},
              {'id': otherId, 'title': 'Nota libre'},
            ],
            'next_cursor': '',
          });
        }
        if (req.method == 'POST' && p.endsWith('/share')) {
          posts.add(p);
          return json(201, {'shared_note_id': 's2'});
        }
        return http.Response('not found', 404);
      }),
    );
    addTearDown(service.dispose);
    await _pumpTab(tester, service);
    await tester.tap(find.text('COMPARTIR UNA NOTA'));
    await tester.pumpAndSettle();

    final sharedTile = find.byKey(const Key('share-note-tile-$_noteId'));
    expect(sharedTile, findsOneWidget);
    expect(
      find.descendant(of: sharedTile, matching: find.text('YA COMPARTIDA')),
      findsOneWidget,
    );
    expect(
      find.descendant(
        of: sharedTile,
        matching: find.byIcon(Icons.block_rounded),
      ),
      findsOneWidget,
    );
    // La selección inicial es la primera nota libre.
    final freeTile = find.byKey(const Key('share-note-tile-$otherId'));
    expect(
      find.descendant(
        of: freeTile,
        matching: find.byIcon(Icons.radio_button_checked_rounded),
      ),
      findsOneWidget,
    );

    // Tocar la ya compartida no la selecciona.
    await tester.tap(find.text('Ya en el grupo').last);
    await tester.pumpAndSettle();
    expect(
      find.descendant(
        of: sharedTile,
        matching: find.byIcon(Icons.radio_button_checked_rounded),
      ),
      findsNothing,
    );
    expect(
      find.descendant(
        of: freeTile,
        matching: find.byIcon(Icons.radio_button_checked_rounded),
      ),
      findsOneWidget,
    );

    await tester.tap(find.text('COMPARTIR'));
    await tester.pumpAndSettle();
    expect(posts, ['/notes/$otherId/share']);
  });
}
