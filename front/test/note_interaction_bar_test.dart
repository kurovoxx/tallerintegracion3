import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/note_interaction_bar.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

const _id = '88888888-8888-4888-8888-888888888888';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

/// Padre mínimo que puede empujar un estado nuevo a la barra (como hace la
/// lista del perfil al volver de la vista del apunte).
class _Host extends StatefulWidget {
  const _Host({required this.api, required this.onReady});

  final NotesApi api;
  final void Function(void Function(NoteInteractions) push) onReady;

  @override
  State<_Host> createState() => _HostState();
}

class _HostState extends State<_Host> {
  NoteInteractions _initial = const NoteInteractions(
    noteId: _id,
    likesCount: 3,
    savesCount: 1,
  );
  NoteInteractions? lastChanged;

  @override
  void initState() {
    super.initState();
    widget.onReady((s) => setState(() => _initial = s));
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    body: NoteLikeSaveBar(
      api: widget.api,
      initial: _initial,
      onChanged: (s) => lastChanged = s,
    ),
  );
}

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
  });
  tearDown(SessionManager.clear);

  testWidgets(
    'respuesta antigua + relectura fallida: combina el campo propio con el '
    'estado más reciente del padre',
    (tester) async {
      final saveGate = Completer<void>();
      var statsCalls = 0;
      final api = NotesApi(
        baseUrl: 'http://notes.test',
        client: MockClient((req) async {
          if (req.url.path.endsWith('/save')) {
            await saveGate.future;
            // Foto antigua: guardado sí, pero sin el like posterior.
            return _json(<String, dynamic>{
              'note_id': _id,
              'likes_count': 3,
              'saves_count': 2,
              'is_liked': false,
              'is_saved': true,
            }, 201);
          }
          if (req.url.path.endsWith('/stats')) {
            statsCalls++;
            return _json(<String, dynamic>{
              'error': {'code': 'internal_error', 'message': 'x'},
            }, 500);
          }
          return _json(<String, dynamic>{}, 404);
        }),
      );
      addTearDown(api.dispose);
      late void Function(NoteInteractions) push;
      await tester.pumpWidget(
        MaterialApp(
          home: _Host(api: api, onReady: (p) => push = p),
        ),
      );

      await tester.tap(find.byKey(const Key('save-$_id')));
      await tester.pump();
      // Mientras el guardado está en vuelo, otra pantalla confirma un like.
      push(
        const NoteInteractions(
          noteId: _id,
          likesCount: 4,
          savesCount: 1,
          isLiked: true,
        ),
      );
      await tester.pump();

      saveGate.complete();
      await tester.pumpAndSettle();

      expect(statsCalls, 1, reason: 'relee una sola vez, solo en la carrera');
      final host = tester.state<_HostState>(find.byType(_Host));
      final s = host.lastChanged!;
      expect(s.isLiked, isTrue, reason: 'el like confirmado no se pierde');
      expect(s.likesCount, 4);
      expect(s.isSaved, isTrue, reason: 'el guardado propio se conserva');
      expect(s.savesCount, 2);
      expect(
        find.descendant(
          of: find.byKey(const Key('like-$_id')),
          matching: find.text('4'),
        ),
        findsOneWidget,
      );
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('sin carrera no relee /stats (sin peticiones duplicadas)', (
    tester,
  ) async {
    var statsCalls = 0;
    final api = NotesApi(
      baseUrl: 'http://notes.test',
      client: MockClient((req) async {
        if (req.url.path.endsWith('/like')) {
          return _json(<String, dynamic>{
            'note_id': _id,
            'likes_count': 4,
            'saves_count': 1,
            'is_liked': true,
            'is_saved': false,
          }, 201);
        }
        if (req.url.path.endsWith('/stats')) statsCalls++;
        return _json(<String, dynamic>{}, 404);
      }),
    );
    addTearDown(api.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: _Host(api: api, onReady: (_) {}),
      ),
    );
    await tester.tap(find.byKey(const Key('like-$_id')));
    await tester.pumpAndSettle();
    expect(statsCalls, 0);
    expect(
      tester.state<_HostState>(find.byType(_Host)).lastChanged?.isLiked,
      isTrue,
    );
  });
}
