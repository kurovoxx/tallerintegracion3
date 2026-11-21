// El mismo apunte Markdown debe verse igual (renderizado, no texto crudo) en
// las cuatro rutas que abren un apunte de la API:
//   A. Buscador de perfiles → «Ver apunte»  → PublicNotePreviewScreen
//   B. Apuntes públicos     → «Ver apunte»  → NoteIdSearchScreen
//   C. Apuntes guardados    → «Ver»         → NoteIdSearchScreen
//   D. Buscar por ID                        → NoteIdSearchScreen
// Todas deben usar NoteContentView → NoteMarkdownBody y cargar la imagen
// `attachment:<uuid>` por el endpoint autenticado de adjuntos.
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/note_content_view.dart';
import 'package:taller_integracion_front/features/notes/note_id_search_screen.dart';
import 'package:taller_integracion_front/features/notes/note_markdown_body.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/public_note_preview_screen.dart';
import 'package:taller_integracion_front/features/notes/public_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/saved_notes_screen.dart';

const _id = '11111111-1111-4111-8111-111111111111';
const _imageId = '22222222-2222-4222-8222-222222222222';

const _markdown =
    '# Encabezado de prueba\n'
    '\n'
    'Un párrafo con **negrita** y *cursiva*.\n'
    '\n'
    '- Primer punto\n'
    '- Segundo punto\n'
    '\n'
    'Visita [Sigma Academy](https://ti3-brojas.dev.censei.cl).\n'
    '\n'
    '![Diagrama](attachment:$_imageId)\n'
    '\n'
    '```\n'
    'print("hola")\n'
    '```\n';

final _png = base64Decode(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
);

http.Response _json(Object body, [int status = 200]) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json; charset=utf-8'},
);

Map<String, Object?> _note() => {
  'id': _id,
  'user_id': 'autor',
  'title': 'Apunte Markdown',
  'content': _markdown,
  'visibility': 'public',
  'is_own': false,
  'likes_count': 0,
  'saves_count': 1,
  'is_liked': false,
  'is_saved': true,
  'attachments': [
    {
      'id': _imageId,
      'note_id': _id,
      'file_type': 'image/png',
      'file_name': 'diagrama.png',
      'external_file_id': 'drive-img',
    },
  ],
};

Map<String, Object> _stats() => {
  'note_id': _id,
  'likes_count': 0,
  'saves_count': 1,
  'is_liked': false,
  'is_saved': true,
};

/// API de notas falsa: detalle, lista pública y guardados.
NotesApi _api() => NotesApi(
  baseUrl: 'http://notes',
  client: MockClient((req) async {
    switch (req.url.path) {
      case '/notes/$_id':
        return _json(_note());
      case '/notes/public':
        return _json({
          'items': [
            {
              'id': _id,
              'user_id': 'autor',
              'title': 'Apunte Markdown',
              ..._stats(),
            },
          ],
          'next_cursor': '',
        });
      case '/notes/saved':
        return _json({
          'items': [
            {
              'saved_id': 's1',
              'note_id': _id,
              'title': 'Apunte Markdown',
              'is_available': true,
              'user_id': 'autor',
              ..._stats(),
            },
          ],
          'next_cursor': '',
        });
    }
    return _json({}, 404);
  }),
);

/// Cliente de imágenes: registra las rutas pedidas al endpoint autenticado.
MockClient _imageClient(List<String> paths) => MockClient((req) async {
  paths.add(req.url.path);
  return http.Response.bytes(_png, 200, headers: {'content-type': 'image/png'});
});

Future<void> _pump(WidgetTester tester, Widget home) async {
  await SessionManager.saveSession('jwt-test', {'id': 'u'});
  addTearDown(SessionManager.clear);
  tester.view.physicalSize = const Size(1200, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(MaterialApp(home: home));
  await tester.pumpAndSettle();
}

/// Comprueba que el contenido se renderizó como Markdown con el componente
/// compartido y que la imagen se pidió por el endpoint autenticado.
void _expectRendered(WidgetTester tester, List<String> imagePaths) {
  // Implementación correcta: el componente compartido con NoteMarkdownBody.
  expect(find.byType(NoteContentView), findsOneWidget);
  expect(
    find.descendant(
      of: find.byType(NoteContentView),
      matching: find.byType(NoteMarkdownBody),
    ),
    findsOneWidget,
  );
  // Ninguna marca Markdown cruda visible.
  for (final raw in <String>[
    '# Encabezado',
    '**',
    '*cursiva*',
    '- Primer',
    '](',
    '![',
    'attachment:',
    '```',
  ]) {
    expect(find.textContaining(raw), findsNothing, reason: 'texto crudo $raw');
  }
  // Contenido presente y formateado.
  expect(find.textContaining('Encabezado de prueba'), findsOneWidget);
  expect(find.textContaining('negrita', findRichText: true), findsOneWidget);
  expect(find.textContaining('Primer punto'), findsOneWidget);
  expect(find.textContaining('Segundo punto'), findsOneWidget);
  expect(
    find.textContaining('Sigma Academy', findRichText: true),
    findsWidgets,
  );
  expect(find.textContaining('print("hola")'), findsOneWidget);
  // Negrita y cursiva como estilos reales, no como asteriscos.
  final spans = <TextSpan>[];
  for (final rt in tester.widgetList<RichText>(find.byType(RichText))) {
    rt.text.visitChildren((s) {
      if (s is TextSpan) spans.add(s);
      return true;
    });
  }
  expect(
    spans.any(
      (s) =>
          s.text == 'negrita' &&
          (s.style?.fontWeight ?? FontWeight.normal).value >=
              FontWeight.w600.value,
    ),
    isTrue,
    reason: 'negrita renderizada',
  );
  expect(
    spans.any(
      (s) => s.text == 'cursiva' && s.style?.fontStyle == FontStyle.italic,
    ),
    isTrue,
    reason: 'cursiva renderizada',
  );
  // La imagen se muestra como imagen, desde el endpoint autenticado.
  expect(imagePaths, contains('/notes/$_id/attachments/$_imageId/content'));
  expect(
    find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),
    findsOneWidget,
  );
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  testWidgets('A · buscador de perfiles: vista previa del apunte', (
    tester,
  ) async {
    final paths = <String>[];
    await _pump(
      tester,
      PublicNotePreviewScreen(
        noteId: _id,
        api: _api(),
        imageClient: _imageClient(paths),
      ),
    );
    _expectRendered(tester, paths);
    expect(tester.takeException(), isNull);
  });

  testWidgets('B · apuntes públicos → «Ver apunte»', (tester) async {
    final paths = <String>[];
    await _pump(
      tester,
      PublicNotesScreen(api: _api(), imageClient: _imageClient(paths)),
    );
    expect(find.byType(NoteContentView), findsNothing);
    await tester.tap(find.text('VER APUNTE'));
    await tester.pumpAndSettle();
    expect(find.byType(NoteIdSearchScreen), findsOneWidget);
    _expectRendered(tester, paths);
    expect(tester.takeException(), isNull);
  });

  testWidgets('C · apuntes guardados → «Ver»', (tester) async {
    final paths = <String>[];
    await _pump(
      tester,
      SavedNotesScreen(api: _api(), imageClient: _imageClient(paths)),
    );
    expect(find.byType(NoteContentView), findsNothing);
    await tester.tap(find.text('VER'));
    await tester.pumpAndSettle();
    expect(find.byType(NoteIdSearchScreen), findsOneWidget);
    _expectRendered(tester, paths);
    expect(tester.takeException(), isNull);
  });

  testWidgets('D · búsqueda por ID', (tester) async {
    final paths = <String>[];
    await _pump(
      tester,
      NoteIdSearchScreen(
        api: _api(),
        initialQuery: _id,
        imageClient: _imageClient(paths),
      ),
    );
    _expectRendered(tester, paths);
    expect(tester.takeException(), isNull);
  });

  testWidgets('D · tocar la imagen abre el visor con el adjunto real', (
    tester,
  ) async {
    final opened = <Map<String, dynamic>>[];
    await _pump(
      tester,
      NoteIdSearchScreen(
        api: _api(),
        initialQuery: _id,
        imageClient: _imageClient(<String>[]),
        openResource: (_, r) async => opened.add(r),
      ),
    );
    await tester.tap(
      find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),
    );
    await tester.pumpAndSettle();
    expect(opened, hasLength(1));
    expect(opened.single['note_id'], _id);
    expect(opened.single['attachment_id'], _imageId);
  });

  testWidgets('el contenido largo ya no se corta (antes 1200 caracteres)', (
    tester,
  ) async {
    final long = '${'Texto largo. ' * 150}\n\nFINAL DEL APUNTE';
    await _pump(
      tester,
      NoteIdSearchScreen(
        initialQuery: _id,
        api: NotesApi(
          baseUrl: 'http://notes',
          client: MockClient(
            (_) async =>
                _json({..._note(), 'content': long, 'attachments': []}),
          ),
        ),
      ),
    );
    expect(find.textContaining('FINAL DEL APUNTE'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
