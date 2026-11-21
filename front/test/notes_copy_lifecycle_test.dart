// Ciclo de vida de una copia de apunte en la app.
// Volver atrás durante la copia no la interrumpe: la petición sigue viva,
// el resultado real se informa con el aviso global y «Mis apuntes» se
// refresca. Nunca se anuncia éxito antes de que la copia termine.
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
const _clone = '22222222-2222-4222-8222-222222222222';

http.Response _json(Object b, [int status = 200]) => http.Response(
  jsonEncode(b),
  status,
  headers: {'content-type': 'application/json; charset=utf-8'},
);

final _detail = {
  'id': _orig,
  'user_id': 'autor',
  'title': 'Apunte ajeno',
  'content': '# Hola',
  'visibility': 'public',
  'is_own': false,
  'attachments': [],
};

final _created = {
  'note_id': _clone,
  'forked_from_note_id': _orig,
  'user_id': 'u',
  'title': 'Apunte ajeno',
  'visibility': 'private',
  'sync_status': 'synced',
  'version': 1,
};

/// Servidor cuya copia responde cuando el test completa [reply].
class _SlowCopyServer {
  final reply = Completer<http.Response>();
  int copyPosts = 0;
  bool closed = false;

  late final http.Client client = _ClosingClient(
    MockClient((r) async {
      if (r.method == 'POST' && r.url.path == '/notes/$_orig/copy') {
        copyPosts++;
        return reply.future;
      }
      if (r.url.path == '/notes/$_orig') return _json(_detail);
      return _json({}, 404);
    }),
    onClose: () => closed = true,
  );

  NotesApi api() => NotesApi(client: client, baseUrl: 'http://notes');
}

/// Registra cuándo se cierra el cliente HTTP (cerrar = abortar peticiones).
class _ClosingClient extends http.BaseClient {
  _ClosingClient(this._inner, {required this.onClose});
  final http.Client _inner;
  final VoidCallback onClose;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) =>
      _inner.send(request);

  @override
  void close() {
    onClose();
    _inner.close();
  }
}

/// App con el messenger global y un botón que abre la vista previa.
Future<void> _pumpApp(WidgetTester tester, NotesApi Function() api) async {
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
                      PublicNotePreviewScreen(noteId: _orig, api: api()),
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

Future<void> _openPreview(WidgetTester tester) async {
  await tester.tap(find.text('abrir'));
  await tester.pumpAndSettle();
}

Future<void> _goBack(WidgetTester tester) async {
  final nav = tester.state<NavigatorState>(find.byType(Navigator));
  nav.pop();
  await tester.pumpAndSettle();
}

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    NoteCopyRunner.resetForTesting();
    await SessionManager.saveSession('jwt', {'id': 'u'});
  });
  tearDown(() async => SessionManager.clear());

  testWidgets('volver atrás no interrumpe: éxito real se avisa al terminar', (
    tester,
  ) async {
    final server = _SlowCopyServer();
    final completedBefore = NoteCopyRunner.completed.value;
    await _pumpApp(tester, server.api);
    await _openPreview(tester);

    await tester.tap(find.byKey(const Key('note-preview-copy')));
    await tester.pump();
    expect(find.text('COPIANDO…'), findsOneWidget);
    expect(find.textContaining('Copia guardada'), findsNothing);

    await _goBack(tester); // vuelve atrás con la copia en curso
    expect(find.byType(PublicNotePreviewScreen), findsNothing);
    expect(server.closed, isFalse, reason: 'no se debe abortar la petición');
    expect(find.textContaining('Copia guardada'), findsNothing);
    expect(NoteCopyRunner.completed.value, completedBefore);

    // La respuesta llega con la pantalla ya cerrada.
    server.reply.complete(_json(_created, 201));
    await tester.pumpAndSettle();
    expect(
      find.text('Copia guardada en tus apuntes y en tu Drive.'),
      findsOneWidget,
    );
    expect(NoteCopyRunner.completed.value, completedBefore + 1);
    expect(server.copyPosts, 1);
    // El api lo aporta quien abrió la vista previa: la pantalla no lo cierra
    // (el cierre diferido de NotesApi se prueba aparte).
    expect(server.closed, isFalse);
    expect(tester.takeException(), isNull);
  });

  testWidgets('fallo tardío tras salir: se avisa el error, nunca éxito', (
    tester,
  ) async {
    final server = _SlowCopyServer();
    final completedBefore = NoteCopyRunner.completed.value;
    await _pumpApp(tester, server.api);
    await _openPreview(tester);
    await tester.tap(find.byKey(const Key('note-preview-copy')));
    await tester.pump();
    await _goBack(tester);

    server.reply.complete(
      _json({
        'error': {'code': 'internal_error', 'message': 'x'},
      }, 502),
    );
    await tester.pumpAndSettle();
    expect(find.textContaining('Copia guardada'), findsNothing);
    expect(
      find.text('Google Drive no respondió. Inténtalo más tarde.'),
      findsOneWidget,
    );
    expect(NoteCopyRunner.completed.value, completedBefore);
  });

  testWidgets(
    'volver a entrar mientras copia: muestra progreso, sin 2.º POST',
    (tester) async {
      final server = _SlowCopyServer();
      await _pumpApp(tester, server.api);
      await _openPreview(tester);
      await tester.tap(find.byKey(const Key('note-preview-copy')));
      await tester.pump();
      await _goBack(tester);

      await _openPreview(tester); // vuelve a abrir el mismo apunte
      expect(find.text('COPIANDO…'), findsOneWidget);
      expect(server.copyPosts, 1);

      server.reply.complete(_json(_created, 201));
      await tester.pumpAndSettle();
      expect(find.text('COPIADO EN TUS APUNTES'), findsOneWidget);
      // La pantalla presente informa; el aviso no se duplica.
      expect(
        find.text('Copia guardada en tus apuntes y en tu Drive.'),
        findsOneWidget,
      );
      expect(server.copyPosts, 1);
    },
  );

  testWidgets('copia ya en curso en el servidor (409): no se marca copiada', (
    tester,
  ) async {
    final api = NotesApi(
      baseUrl: 'http://notes',
      client: MockClient((r) async {
        if (r.method == 'POST') {
          return _json({
            'error': {
              'code': 'copy_in_progress',
              'message': 'Ya estamos copiando este apunte',
            },
          }, 409);
        }
        return _json(_detail);
      }),
    );
    await tester.pumpWidget(
      MaterialApp(
        home: PublicNotePreviewScreen(noteId: _orig, api: api),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('note-preview-copy')));
    await tester.pumpAndSettle();
    expect(find.text(kCopyInProgressMessage), findsOneWidget);
    expect(find.text('COPIADO EN TUS APUNTES'), findsNothing);
    expect(find.text('COPIAR A MIS APUNTES'), findsOneWidget);
    expect(find.textContaining('Copia guardada'), findsNothing);
  });

  test('NoteCopyRunner reutiliza la copia en curso del mismo apunte', () async {
    final server = _SlowCopyServer();
    final a = NoteCopyRunner.start(server.api(), _orig, idempotencyKey: 'k1');
    final b = NoteCopyRunner.start(server.api(), _orig, idempotencyKey: 'k2');
    expect(identical(a, b), isTrue);
    server.reply.complete(_json(_created, 201));
    expect((await a).noteId, _clone);
    expect(server.copyPosts, 1);
    expect(NoteCopyRunner.pending(_orig), isNull);
  });

  test('NotesApi.dispose espera a la copia en curso antes de cerrar', () async {
    final server = _SlowCopyServer();
    final api = server.api();
    final copy = api.copyNote(_orig, idempotencyKey: 'k');
    await Future<void>.delayed(Duration.zero);
    api.dispose();
    expect(server.closed, isFalse);
    server.reply.complete(_json(_created, 201));
    await copy;
    expect(server.closed, isTrue);
    // Sin copias en curso, dispose cierra al instante.
    final other = _SlowCopyServer();
    other.api().dispose();
    expect(other.closed, isTrue);
  });
}
