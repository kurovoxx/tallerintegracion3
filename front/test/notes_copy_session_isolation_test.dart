// Aislamiento por sesión del coordinador de copias (NoteCopyRunner).
// Una copia de la cuenta A sigue en segundo plano tras cerrar sesión, pero
// nunca se muestra, bloquea ni notifica a la cuenta B (ni a una nueva sesión
// de la misma cuenta).
import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/note_copy_runner.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/public_note_preview_screen.dart';

const _orig = '11111111-1111-4111-8111-111111111111';
const _cloneA = '22222222-2222-4222-8222-22222222222a';
const _cloneB = '22222222-2222-4222-8222-22222222222b';
const _success = 'Copia guardada en tus apuntes y en tu Drive.';

http.Response _json(Object b, [int status = 200]) => http.Response(
  jsonEncode(b),
  status,
  headers: {'content-type': 'application/json; charset=utf-8'},
);

Map<String, Object> _created(String id) => {
  'note_id': id,
  'forked_from_note_id': _orig,
  'title': 'Apunte ajeno',
  'visibility': 'private',
  'sync_status': 'synced',
};

/// Backend falso: cada cuenta (por su token) tiene su propia respuesta de
/// copia, que el test completa cuando quiere.
class _Server {
  final Map<String, Completer<http.Response>> replies = {
    'Bearer jwt-a': Completer<http.Response>(),
    'Bearer jwt-b': Completer<http.Response>(),
  };
  final Map<String, int> posts = {'Bearer jwt-a': 0, 'Bearer jwt-b': 0};

  late final MockClient client = MockClient((r) async {
    if (r.method == 'POST' && r.url.path == '/notes/$_orig/copy') {
      final auth = r.headers['Authorization'] ?? '';
      posts[auth] = (posts[auth] ?? 0) + 1;
      return replies[auth]!.future;
    }
    if (r.url.path == '/notes/$_orig') {
      return _json({
        'id': _orig,
        'user_id': 'autor',
        'title': 'Apunte ajeno',
        'content': '# Hola',
        'visibility': 'public',
        'is_own': false,
        'attachments': [],
      });
    }
    return _json({}, 404);
  });

  NotesApi api() => NotesApi(client: client, baseUrl: 'http://notes');
}

Future<void> _login(String who) async {
  await SessionManager.saveSession('jwt-$who', {'id': 'user-$who'});
}

Future<void> _pumpApp(WidgetTester tester, _Server server) async {
  await tester.pumpWidget(
    MaterialApp(
      scaffoldMessengerKey: appScaffoldMessengerKey,
      home: Builder(
        builder: (context) => Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) =>
                      PublicNotePreviewScreen(noteId: _orig, api: server.api()),
                ),
              ),
              child: const Text('abrir'),
            ),
          ),
        ),
      ),
    ),
  );
}

Future<void> _open(WidgetTester tester) async {
  await tester.tap(find.text('abrir'));
  await tester.pumpAndSettle();
}

Future<void> _back(WidgetTester tester) async {
  tester.state<NavigatorState>(find.byType(Navigator)).pop();
  await tester.pumpAndSettle();
}

Future<void> _tapCopy(WidgetTester tester) async {
  await tester.tap(find.byKey(const Key('note-preview-copy')));
  await tester.pump();
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    NoteCopyRunner.resetForTesting();
  });
  tearDown(() async => SessionManager.clear());

  testWidgets('A copia, cierra sesión y B usa su propia copia sin mezclar', (
    tester,
  ) async {
    final server = _Server();
    final completedAtStart = NoteCopyRunner.completed.value;
    await _login('a');
    await _pumpApp(tester, server);

    // 1. A inicia la copia y vuelve atrás (sigue en segundo plano).
    await _open(tester);
    await _tapCopy(tester);
    expect(find.text('COPIANDO…'), findsOneWidget);
    await _back(tester);

    // A cierra sesión; entra B.
    await SessionManager.clear();
    await _login('b');

    // 2-3. B abre el mismo apunte: no ve COPIANDO por la copia de A.
    await _open(tester);
    expect(find.text('COPIANDO…'), findsNothing);
    expect(find.text('COPIAR A MIS APUNTES'), findsOneWidget);
    expect(NoteCopyRunner.pending(_orig), isNull);

    // 4. B puede iniciar su propia copia (petición con el token de B).
    await _tapCopy(tester);
    expect(find.text('COPIANDO…'), findsOneWidget);
    expect(server.posts['Bearer jwt-b'], 1);

    // 7. Cerrar y volver a abrir la pantalla no duplica la copia de B.
    await _back(tester);
    await _open(tester);
    expect(find.text('COPIANDO…'), findsOneWidget);
    expect(server.posts['Bearer jwt-b'], 1);

    // 5. La respuesta de A llega: nada se notifica a B.
    server.replies['Bearer jwt-a']!.complete(_json(_created(_cloneA), 201));
    await tester.pumpAndSettle();
    expect(find.text(_success), findsNothing);
    expect(find.text('COPIANDO…'), findsOneWidget, reason: 'sigue la de B');
    expect(NoteCopyRunner.completed.value, completedAtStart);

    // 6. La de B termina: B recibe solo su resultado, una vez.
    server.replies['Bearer jwt-b']!.complete(_json(_created(_cloneB), 201));
    await tester.pumpAndSettle();
    expect(find.text(_success), findsOneWidget);
    expect(find.text('COPIADO EN TUS APUNTES'), findsOneWidget);
    expect(NoteCopyRunner.completed.value, completedAtStart + 1);
    expect(server.posts, {'Bearer jwt-a': 1, 'Bearer jwt-b': 1});
    expect(tester.takeException(), isNull);
  });

  testWidgets('un error tardío de A tampoco se notifica a B', (tester) async {
    final server = _Server();
    await _login('a');
    await _pumpApp(tester, server);
    await _open(tester);
    await _tapCopy(tester);
    await _back(tester);
    await SessionManager.clear();
    await _login('b');

    server.replies['Bearer jwt-a']!.complete(
      _json({
        'error': {'code': 'internal_error', 'message': 'x'},
      }, 502),
    );
    await tester.pumpAndSettle();
    expect(find.byType(SnackBar), findsNothing);
  });

  testWidgets('8. cerrar sesión con la pantalla abierta limpia el estado', (
    tester,
  ) async {
    final server = _Server();
    await _login('a');
    await _pumpApp(tester, server);
    await _open(tester);
    await _tapCopy(tester);
    expect(find.text('COPIANDO…'), findsOneWidget);

    // Cierra sesión y vuelve a entrar con la MISMA cuenta: sesión nueva.
    await SessionManager.clear();
    await tester.pump();
    expect(find.text('COPIANDO…'), findsNothing);
    await _login('a');
    await tester.pump();
    expect(NoteCopyRunner.pending(_orig), isNull);
    expect(find.text('COPIAR A MIS APUNTES'), findsOneWidget);

    // La respuesta de la sesión anterior no marca nada ni avisa.
    server.replies['Bearer jwt-a']!.complete(_json(_created(_cloneA), 201));
    await tester.pumpAndSettle();
    expect(find.text(_success), findsNothing);
    expect(find.text('COPIADO EN TUS APUNTES'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  test('la copia de otra sesión no se reutiliza ni se cancela', () async {
    final server = _Server();
    await _login('a');
    final sessionA = NoteCopyRunner.sessionKey();
    final copyA = NoteCopyRunner.start(
      server.api(),
      _orig,
      idempotencyKey: 'a',
    );
    expect(identical(NoteCopyRunner.pending(_orig), copyA), isTrue);

    await SessionManager.clear();
    await _login('b');
    expect(NoteCopyRunner.isCurrentSession(sessionA), isFalse);
    expect(NoteCopyRunner.pending(_orig), isNull);
    final copyB = NoteCopyRunner.start(
      server.api(),
      _orig,
      idempotencyKey: 'b',
    );
    expect(identical(copyA, copyB), isFalse);

    // La de A no se canceló: termina normalmente en segundo plano.
    server.replies['Bearer jwt-a']!.complete(_json(_created(_cloneA), 201));
    server.replies['Bearer jwt-b']!.complete(_json(_created(_cloneB), 201));
    expect((await copyA).noteId, _cloneA);
    expect((await copyB).noteId, _cloneB);
    expect(NoteCopyRunner.pending(_orig), isNull);
  });
}
