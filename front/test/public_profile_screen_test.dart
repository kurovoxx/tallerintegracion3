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

  testWidgets('muestra nombre, institución y apuntes públicos', (tester) async {
    final profiles = _profileService();
    final notes = _notesApi();
    addTearDown(profiles.dispose);
    addTearDown(notes.dispose);

    await _pump(tester, profiles: profiles, notes: notes);

    expect(find.byKey(const ValueKey('public-profile-card')), findsOneWidget);
    expect(find.text('Ana Pública'), findsOneWidget);
    expect(find.text('UCT'), findsOneWidget);
    // La descripción es privada aunque el backend la enviara por error.
    expect(find.text('Apuntes de cálculo'), findsNothing);
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

  testWidgets('un perfil privado oculta sus datos pero no sus apuntes', (
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
}
