import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/public_note_preview_screen.dart';

const _id = '11111111-1111-4111-8111-111111111111';
const _imageId = '22222222-2222-4222-8222-222222222222';
const _pdfId = '33333333-3333-4333-8333-333333333333';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

final _png = base64Decode(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
);

Map<String, Object?> _note({
  String content =
      '# Cinemática\n\n**Velocidad** media\n\n![Diagrama](attachment:$_imageId)',
  bool isOwn = false,
}) => {
  'id': _id,
  'user_id': 'autor',
  'title': 'Física I',
  'content': content,
  'visibility': 'public',
  'is_own': isOwn,
  'attachments': [
    {
      'id': _imageId,
      'note_id': _id,
      'file_type': 'image/png',
      'file_name': 'diagrama.png',
      'external_file_id': 'drive-img',
    },
    {
      'id': _pdfId,
      'note_id': _id,
      'file_type': 'application/pdf',
      'file_name': 'guia.pdf',
      'external_file_id': 'drive-pdf',
    },
  ],
};

Future<void> _pump(
  WidgetTester tester,
  MockClient client, {
  http.Client? imageClient,
  NoteResourceOpener? openResource,
  VoidCallback? onNoteCopied,
}) async {
  await SessionManager.saveSession('jwt-test', {'id': 'u'});
  addTearDown(SessionManager.clear);
  await tester.pumpWidget(
    MaterialApp(
      home: PublicNotePreviewScreen(
        noteId: _id,
        initialTitle: 'Física I',
        api: NotesApi(client: client, baseUrl: 'http://notes'),
        imageClient: imageClient,
        openResource: openResource,
        onNoteCopied: onNoteCopied,
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  testWidgets('renderiza Markdown e imágenes por el endpoint autenticado', (
    tester,
  ) async {
    final contentPaths = <String>[];
    final imageClient = MockClient((request) async {
      contentPaths.add(request.url.path);
      expect(request.headers['Authorization'], 'Bearer jwt-test');
      return http.Response.bytes(
        _png,
        200,
        headers: {'content-type': 'image/png'},
      );
    });
    await _pump(
      tester,
      MockClient((req) async {
        expect(req.url.path, '/notes/$_id');
        return _json(_note(), 200);
      }),
      imageClient: imageClient,
    );

    expect(find.byKey(const Key('note-preview-title')), findsOneWidget);
    expect(find.textContaining('Cinemática'), findsOneWidget);
    // Markdown real: sin asteriscos ni almohadillas crudas.
    expect(find.textContaining('**'), findsNothing);
    expect(find.textContaining('# Cinemática'), findsNothing);
    expect(contentPaths, <String>['/notes/$_id/attachments/$_imageId/content']);
    expect(
      find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),
      findsOneWidget,
    );
  });

  testWidgets('el PDF adjunto se abre en el visor con su attachment_id', (
    tester,
  ) async {
    final opened = <Map<String, dynamic>>[];
    await _pump(
      tester,
      MockClient((_) async => _json(_note(content: 'Sin imágenes'), 200)),
      openResource: (_, resource) async => opened.add(resource),
    );

    // Solo el PDF aparece como adjunto para ver; la imagen va en el cuerpo.
    expect(
      find.byKey(const Key('note-preview-attachment-$_imageId')),
      findsNothing,
    );
    await tester.tap(find.byKey(const Key('note-preview-attachment-$_pdfId')));
    await tester.pumpAndSettle();

    expect(opened, hasLength(1));
    expect(opened.single['type'], 'pdf');
    expect(opened.single['note_id'], _id);
    expect(opened.single['attachment_id'], _pdfId);
  });

  testWidgets('copiar usa una clave estable y la segunda vez informa', (
    tester,
  ) async {
    final keys = <String?>[];
    var copied = 0;
    var calls = 0;
    await _pump(
      tester,
      MockClient((req) async {
        if (req.url.path == '/notes/$_id/copy') {
          calls++;
          keys.add(req.headers['X-Idempotency-Key']);
          return _json({
            'note_id': '44444444-4444-4444-8444-444444444444',
            'forked_from_note_id': _id,
            'title': 'Física I',
            'version': 1,
          }, 201);
        }
        return _json(_note(content: 'Contenido'), 200);
      }),
      onNoteCopied: () => copied++,
    );

    await tester.tap(find.byKey(const Key('note-preview-copy')));
    await tester.pump();
    expect(find.textContaining('Copia guardada'), findsOneWidget);
    expect(copied, 1);
    expect(calls, 1);
    expect(keys.single, startsWith('copy-$_id-'));

    // Ya copiado: el botón queda deshabilitado y no se repite la petición.
    await tester.pumpAndSettle();
    expect(find.text('COPIADO EN TUS APUNTES'), findsOneWidget);
    await tester.tap(find.byKey(const Key('note-preview-copy')));
    await tester.pumpAndSettle();
    expect(calls, 1);
  });

  testWidgets('already_copied deja el apunte como copiado sin error', (
    tester,
  ) async {
    await _pump(
      tester,
      MockClient((req) async {
        if (req.url.path == '/notes/$_id/copy') {
          return _json({
            'error': {'code': 'already_copied', 'message': 'x'},
          }, 409);
        }
        return _json(_note(content: 'Contenido'), 200);
      }),
    );
    await tester.tap(find.byKey(const Key('note-preview-copy')));
    await tester.pumpAndSettle();
    expect(find.text('Ya copiaste este apunte a tus apuntes.'), findsOneWidget);
    expect(find.text('COPIADO EN TUS APUNTES'), findsOneWidget);
  });

  testWidgets('un apunte que dejó de ser accesible muestra 404 claro', (
    tester,
  ) async {
    await _pump(
      tester,
      MockClient(
        (_) async => _json({
          'error': {'code': 'not_found', 'message': 'x'},
        }, 404),
      ),
    );
    expect(find.byKey(const Key('note-preview-error')), findsOneWidget);
    expect(find.text('Apunte no disponible'), findsOneWidget);
    expect(find.byKey(const Key('note-preview-copy')), findsNothing);
  });
}
