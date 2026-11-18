import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/public_profile_screen.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/public_note_preview_screen.dart';

const _publicId = 'a8kp92mqwx7r';

ProfileService _profileService({int status = 200, Map<String, dynamic>? body}) {
  return ProfileService(
    client: MockClient((request) async {
      return http.Response(
        jsonEncode(
          body ??
              <String, dynamic>{
                'public_id': _publicId,
                'display_name': 'Ana Pública',
                'photo_url': null,
                'institution': 'UCT',
                'description': 'Apuntes de cálculo',
                'email': 'ana@test.invalid',
                'phone': '+56912345678',
                'visibility': 'public',
              },
        ),
        status,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
}

NotesApi _notesApi({
  int status = 200,
  List<Map<String, dynamic>>? notes,
  String nextCursor = '',
}) {
  var liked = false;
  return NotesApi(
    baseUrl: 'http://notes.test',
    client: MockClient((request) async {
      if (request.url.path.endsWith('/like')) {
        liked = true;
        return http.Response(
          jsonEncode(<String, dynamic>{
            'note_id': 'n1',
            'likes_count': 4,
            'saves_count': 1,
            'is_liked': liked,
            'is_saved': false,
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }
      if (request.url.path.endsWith('/stats')) {
        return http.Response(
          jsonEncode(<String, dynamic>{
            'note_id': 'n1',
            'likes_count': liked ? 4 : 3,
            'saves_count': 1,
            'is_liked': liked,
            'is_saved': false,
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      if (status != 200) {
        return http.Response(
          jsonEncode(<String, dynamic>{
            'error': {'code': 'profile_service_unavailable', 'message': 'x'},
          }),
          status,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response(
        jsonEncode(<String, dynamic>{
          'notes':
              notes ??
              <Map<String, dynamic>>[
                <String, dynamic>{
                  'id': 'n1',
                  'title': 'Apunte público de Ana',
                  'subject_id': null,
                  'visibility': 'public',
                  'likes_count': 3,
                  'saves_count': 1,
                  'is_liked': liked,
                  'is_saved': false,
                  'created_at': '2026-11-01T10:00:00Z',
                  'updated_at': '2026-11-02T10:00:00Z',
                },
              ],
          'next_cursor': nextCursor,
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
}

Future<void> _pump(
  WidgetTester tester, {
  required ProfileService profiles,
  required NotesApi notes,
}) async {
  tester.view.physicalSize = const Size(1280, 1400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      home: PublicProfileScreen(
        publicId: _publicId,
        profileService: profiles,
        notesApi: notes,
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
  });

  tearDown(() async {
    await SessionManager.clear();
  });

  testWidgets('perfil público: nombre, descripción, correo, teléfono, '
      'universidad y apuntes', (tester) async {
    final profiles = _profileService();
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    expect(find.byKey(const ValueKey('public-profile-card')), findsOneWidget);
    expect(find.text('Ana Pública'), findsOneWidget);
    expect(find.text('UCT'), findsOneWidget);
    expect(find.text('Apuntes de cálculo'), findsOneWidget);
    expect(find.text('ana@test.invalid'), findsOneWidget);
    expect(find.text('+56912345678'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('public-profile-private-badge')),
      findsNothing,
    );
    expect(find.text('Apunte público de Ana'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('public-profile-note-n1')),
      findsOneWidget,
    );
    expect(find.byKey(const Key('like-n1')), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('un perfil inexistente también en Notes muestra 404', (
    tester,
  ) async {
    final profiles = _profileService(
      status: 404,
      body: <String, dynamic>{
        'error': {'code': 'profile_not_found', 'message': 'no'},
      },
    );
    final notes = _notesApi(status: 404);
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    expect(
      find.byKey(const ValueKey('public-profile-not-found')),
      findsOneWidget,
    );
    expect(find.text('PERFIL NO DISPONIBLE'), findsOneWidget);
    expect(find.text('Ana Pública'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('perfil privado: solo nombre y foto; nada más aunque llegue', (
    tester,
  ) async {
    const png =
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';
    final profiles = _profileService(
      body: <String, dynamic>{
        'public_id': _publicId,
        'display_name': 'Bea Privada',
        'photo_url': 'data:image/png;base64,$png',
        'visibility': 'private',
        // Un backend defectuoso no debe lograr que se muestren.
        'description': 'Descripción oculta',
        'email': 'bea@test.invalid',
        'phone': '+56900000000',
        'institution': 'Universidad oculta',
      },
    );
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    expect(find.text('Bea Privada'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('public-profile-private-badge')),
      findsOneWidget,
    );
    final avatar = find.byKey(const ValueKey('public-profile-avatar'));
    expect(
      find.descendant(
        of: avatar,
        matching: find.byWidgetPredicate(
          (w) => w is Image && w.image is MemoryImage,
        ),
      ),
      findsOneWidget,
    );
    for (final hidden in <String>[
      'Descripción oculta',
      'bea@test.invalid',
      '+56900000000',
      'Universidad oculta',
    ]) {
      expect(find.text(hidden), findsNothing, reason: hidden);
    }
    for (final key in <String>[
      'public-profile-description',
      'public-profile-email',
      'public-profile-phone',
      'public-profile-institution',
    ]) {
      expect(find.byKey(ValueKey(key)), findsNothing, reason: key);
    }
    // Sus apuntes públicos siguen visibles.
    expect(find.text('Apunte público de Ana'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('un Auth anterior (404 para privados) sigue mostrando apuntes', (
    tester,
  ) async {
    final profiles = _profileService(
      status: 404,
      body: <String, dynamic>{
        'error': {'code': 'profile_not_found', 'message': 'privado'},
      },
    );
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    expect(find.byKey(const ValueKey('public-profile-hidden')), findsOneWidget);
    expect(find.text('PERFIL PRIVADO'), findsOneWidget);
    expect(find.text('Apunte público de Ana'), findsOneWidget);
    expect(find.text('Ana Pública'), findsNothing);
    expect(find.text('UCT'), findsNothing);
    expect(
      find.byKey(const ValueKey('public-profile-not-found')),
      findsNothing,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('muestra la foto subida desde la app (data URL)', (tester) async {
    const png =
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';
    final profiles = _profileService(
      body: <String, dynamic>{
        'public_id': _publicId,
        'display_name': 'Ana Pública',
        'photo_url': 'data:image/png;base64,$png',
        'institution': 'UCT',
        'visibility': 'public',
      },
    );
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    final avatar = find.byKey(const ValueKey('public-profile-avatar'));
    expect(
      find.descendant(
        of: avatar,
        matching: find.byWidgetPredicate(
          (w) => w is Image && w.image is MemoryImage,
        ),
      ),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('una foto remota no se carga: muestra las iniciales', (
    tester,
  ) async {
    final profiles = _profileService(
      body: <String, dynamic>{
        'public_id': _publicId,
        'display_name': 'Ana Pública',
        'photo_url': 'https://tracker.example/foto.png',
        'institution': 'UCT',
        'visibility': 'public',
      },
    );
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    final avatar = find.byKey(const ValueKey('public-profile-avatar'));
    expect(
      find.descendant(of: avatar, matching: find.byType(Image)),
      findsNothing,
    );
    expect(
      find.descendant(of: avatar, matching: find.text('AP')),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('VER APUNTE abre la vista previa del apunte', (tester) async {
    final profiles = _profileService();
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);
    await tester.tap(find.byKey(const ValueKey('public-profile-open-note-n1')));
    await tester.pumpAndSettle();

    expect(find.byType(PublicNotePreviewScreen), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('CARGAR MÁS pide la siguiente página usando el cursor', (
    tester,
  ) async {
    final profiles = _profileService();
    final cursors = <String?>[];
    final notes = NotesApi(
      baseUrl: 'http://notes.test',
      client: MockClient((request) async {
        final cursor = request.url.queryParameters['cursor'];
        cursors.add(cursor);
        final firstPage = cursor == null || cursor.isEmpty;
        return http.Response(
          jsonEncode(<String, dynamic>{
            'notes': <Map<String, dynamic>>[
              <String, dynamic>{
                'id': firstPage ? 'n1' : 'n2',
                'title': firstPage ? 'Página uno' : 'Página dos',
                'subject_id': null,
                'visibility': 'public',
                'likes_count': 0,
                'saves_count': 0,
                'is_liked': false,
                'is_saved': false,
                'created_at': '2026-11-01T10:00:00Z',
                'updated_at': '2026-11-02T10:00:00Z',
              },
            ],
            'next_cursor': firstPage ? 'cursor-1' : '',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);
    expect(find.text('Página uno'), findsOneWidget);
    expect(find.text('CARGAR MÁS'), findsOneWidget);

    await tester.tap(find.text('CARGAR MÁS'));
    await tester.pumpAndSettle();

    expect(cursors, [null, 'cursor-1']);
    expect(find.text('Página uno'), findsOneWidget);
    expect(find.text('Página dos'), findsOneWidget);
    expect(find.text('CARGAR MÁS'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('like y guardado hechos en la vista del apunte se ven al volver, '
      'sin pedir de nuevo la lista', (tester) async {
    const nid = '44444444-4444-4444-8444-444444444444';
    final profiles = _profileService();
    var listRequests = 0;
    var liked = false;
    var saved = false;
    Map<String, dynamic> stats() => <String, dynamic>{
      'note_id': nid,
      'likes_count': liked ? 4 : 3,
      'saves_count': saved ? 2 : 1,
      'is_liked': liked,
      'is_saved': saved,
    };
    http.Response json(Object body, int status) => http.Response(
      jsonEncode(body),
      status,
      headers: {'content-type': 'application/json'},
    );
    final notes = NotesApi(
      baseUrl: 'http://notes.test',
      client: MockClient((request) async {
        final path = request.url.path;
        if (path.startsWith('/public-users/')) {
          listRequests++;
          return json(<String, dynamic>{
            'notes': <Map<String, dynamic>>[
              <String, dynamic>{
                'id': nid,
                'title': 'Apunte público de Ana',
                'visibility': 'public',
                ...stats(),
              },
            ],
            'next_cursor': '',
          }, 200);
        }
        if (path.endsWith('/like')) {
          liked = request.method == 'POST';
          return json(stats(), 201);
        }
        if (path.endsWith('/save')) {
          saved = request.method == 'POST';
          return json(stats(), 201);
        }
        if (path.endsWith('/stats')) return json(stats(), 200);
        if (path == '/notes/$nid') {
          return json(<String, dynamic>{
            'id': nid,
            'title': 'Apunte público de Ana',
            'content': '# Hola',
            'visibility': 'public',
            'is_own': false,
            'attachments': <Object>[],
            ...stats(),
          }, 200);
        }
        return json(<String, dynamic>{}, 404);
      }),
    );
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);
    expect(listRequests, 1);
    expect(find.text('3'), findsOneWidget);
    expect(find.text('GUARDAR · 1'), findsOneWidget);

    await tester.tap(find.byKey(ValueKey('public-profile-open-note-$nid')));
    await tester.pumpAndSettle();
    expect(find.byType(PublicNotePreviewScreen), findsOneWidget);

    await tester.tap(find.byKey(Key('like-$nid')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(Key('save-$nid')));
    await tester.pumpAndSettle();

    // Volver atrás (como el botón del sistema / la flecha de la vista).
    tester.state<NavigatorState>(find.byType(Navigator)).pop();
    await tester.pumpAndSettle();
    expect(find.byType(PublicNotePreviewScreen), findsNothing);

    // La tarjeta refleja el estado confirmado sin salir del perfil.
    expect(find.text('4'), findsOneWidget);
    expect(find.text('GUARDADO · 2'), findsOneWidget);
    expect(
      find.descendant(
        of: find.byKey(Key('like-$nid')),
        matching: find.byIcon(Icons.favorite_rounded),
      ),
      findsOneWidget,
    );
    // Sin solicitudes duplicadas: la lista no se volvió a pedir.
    expect(listRequests, 1);
    expect(tester.takeException(), isNull);
  });

  testWidgets('el autor que busca su propio código no ve «Guardar»', (
    tester,
  ) async {
    final profiles = _profileService();
    final notes = _notesApi(
      notes: <Map<String, dynamic>>[
        <String, dynamic>{
          'id': 'n1',
          'title': 'Mi apunte',
          'visibility': 'public',
          'likes_count': 0,
          'saves_count': 0,
          'is_liked': false,
          'is_saved': false,
          'is_own': true,
        },
      ],
    );
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    expect(find.byKey(const Key('like-n1')), findsOneWidget);
    expect(find.byKey(const Key('save-n1')), findsNothing);
    expect(tester.takeException(), isNull);
  });

  // Estado compartido entre pantallas ante una respuesta antigua: el
  // guardado iniciado en la TARJETA queda en vuelo; mientras tanto el
  // usuario abre el apunte, da like (confirmado) y vuelve. Cuando por fin
  // llega la respuesta del guardado —una foto del servidor ANTERIOR al like—
  // no debe borrar el like confirmado más reciente.
  testWidgets(
    'una respuesta antigua que llega al volver no pisa el estado confirmado',
    (tester) async {
      const nid = '55555555-5555-4555-8555-555555555555';
      final profiles = _profileService();
      var liked = false;
      var saved = false;
      var listRequests = 0;
      Map<String, dynamic> stats() => <String, dynamic>{
        'note_id': nid,
        'likes_count': liked ? 4 : 3,
        'saves_count': saved ? 2 : 1,
        'is_liked': liked,
        'is_saved': saved,
      };
      http.Response json(Object body, int status) => http.Response(
        jsonEncode(body),
        status,
        headers: {'content-type': 'application/json'},
      );
      final saveResponse = Completer<http.Response>();
      final notes = NotesApi(
        baseUrl: 'http://notes.test',
        client: MockClient((request) async {
          final path = request.url.path;
          if (path.startsWith('/public-users/')) {
            listRequests++;
            return json(<String, dynamic>{
              'notes': <Map<String, dynamic>>[
                <String, dynamic>{
                  'id': nid,
                  'title': 'Apunte público de Ana',
                  'visibility': 'public',
                  ...stats(),
                },
              ],
              'next_cursor': '',
            }, 200);
          }
          if (path.endsWith('/save') && request.method == 'POST') {
            // El servidor aplica el guardado y toma la foto AHORA (sin like),
            // pero la respuesta llega tarde.
            saved = true;
            final snapshot = json(stats(), 201);
            return saveResponse.future.then((_) => snapshot);
          }
          if (path.endsWith('/like') && request.method == 'POST') {
            liked = true;
            return json(stats(), 201);
          }
          if (path.endsWith('/stats')) return json(stats(), 200);
          if (path == '/notes/$nid') {
            return json(<String, dynamic>{
              'id': nid,
              'title': 'Apunte público de Ana',
              'content': '# Hola',
              'visibility': 'public',
              'is_own': false,
              'attachments': <Object>[],
              ...stats(),
            }, 200);
          }
          return json(<String, dynamic>{}, 404);
        }),
      );
      addTearDown(profiles.dispose);
      addTearDown(notes.dispose);

      // 1. Perfil ajeno con sus apuntes.
      await _pump(tester, profiles: profiles, notes: notes);
      expect(find.text('GUARDAR · 1'), findsOneWidget);

      // Guardar desde la tarjeta: la petición queda en vuelo.
      await tester.tap(find.byKey(const Key('save-$nid')));
      await tester.pump();

      // 2-3. Abrir el apunte y dar like (confirmado por el servidor).
      await tester.tap(
        find.byKey(const ValueKey('public-profile-open-note-$nid')),
      );
      await tester.pumpAndSettle();
      expect(find.byType(PublicNotePreviewScreen), findsOneWidget);
      await tester.tap(find.byKey(const Key('like-$nid')));
      await tester.pumpAndSettle();

      // 4. Volver a la lista: ya refleja el like.
      tester.state<NavigatorState>(find.byType(Navigator)).pop();
      await tester.pumpAndSettle();
      expect(find.byType(PublicNotePreviewScreen), findsNothing);

      // 5. Llega la respuesta antigua del guardado (sin el like).
      saveResponse.complete(http.Response('', 200));
      await tester.pumpAndSettle();

      // 6. El like confirmado sigue ahí y el guardado también.
      final likeButton = find.byKey(const Key('like-$nid'));
      expect(
        find.descendant(of: likeButton, matching: find.text('4')),
        findsOneWidget,
      );
      expect(
        find.descendant(
          of: likeButton,
          matching: find.byIcon(Icons.favorite_rounded),
        ),
        findsOneWidget,
      );
      expect(find.text('GUARDADO · 2'), findsOneWidget);
      // Sin recargar la lista.
      expect(listRequests, 1);
      expect(tester.takeException(), isNull);
    },
  );

  // Respuesta de página antigua: «CARGAR MÁS» se pide ANTES del like hecho
  // en la vista del apunte y llega DESPUÉS de volver. Debe agregar la página
  // sin tocar el estado confirmado de los apuntes ya cargados.
  testWidgets(
    'una página pedida antes del like no revierte el estado al llegar',
    (tester) async {
      const n1 = '66666666-6666-4666-8666-666666666666';
      const n2 = '77777777-7777-4777-8777-777777777777';
      final profiles = _profileService();
      var liked = false;
      Map<String, dynamic> stats(String id) => <String, dynamic>{
        'note_id': id,
        'likes_count': id == n1 && liked ? 1 : 0,
        'saves_count': 0,
        'is_liked': id == n1 && liked,
        'is_saved': false,
      };
      http.Response json(Object body, int status) => http.Response(
        jsonEncode(body),
        status,
        headers: {'content-type': 'application/json'},
      );
      final page2 = Completer<void>();
      final notes = NotesApi(
        baseUrl: 'http://notes.test',
        client: MockClient((request) async {
          final path = request.url.path;
          if (path.startsWith('/public-users/')) {
            final cursor = request.url.queryParameters['cursor'];
            if (cursor == null || cursor.isEmpty) {
              return json(<String, dynamic>{
                'notes': <Map<String, dynamic>>[
                  <String, dynamic>{'id': n1, 'title': 'Uno', ...stats(n1)},
                ],
                'next_cursor': 'c1',
              }, 200);
            }
            final body = <String, dynamic>{
              'notes': <Map<String, dynamic>>[
                <String, dynamic>{'id': n2, 'title': 'Dos', ...stats(n2)},
              ],
              'next_cursor': '',
            };
            await page2.future;
            return json(body, 200);
          }
          if (path.endsWith('/like') && request.method == 'POST') {
            liked = true;
            return json(stats(n1), 201);
          }
          if (path.endsWith('/stats')) return json(stats(n1), 200);
          if (path == '/notes/$n1') {
            return json(<String, dynamic>{
              'id': n1,
              'title': 'Uno',
              'content': '# Uno',
              'visibility': 'public',
              'is_own': false,
              'attachments': <Object>[],
              ...stats(n1),
            }, 200);
          }
          return json(<String, dynamic>{}, 404);
        }),
      );
      addTearDown(profiles.dispose);
      addTearDown(notes.dispose);

      await _pump(tester, profiles: profiles, notes: notes);
      await tester.tap(find.text('CARGAR MÁS'));
      await tester.pump();

      await tester.tap(
        find.byKey(const ValueKey('public-profile-open-note-$n1')),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('like-$n1')));
      await tester.pumpAndSettle();
      tester.state<NavigatorState>(find.byType(Navigator)).pop();
      await tester.pumpAndSettle();

      page2.complete();
      await tester.pumpAndSettle();

      expect(find.text('Dos'), findsOneWidget);
      expect(
        find.descendant(
          of: find.byKey(const Key('like-$n1')),
          matching: find.byIcon(Icons.favorite_rounded),
        ),
        findsOneWidget,
      );
      expect(tester.takeException(), isNull);
    },
  );
}
