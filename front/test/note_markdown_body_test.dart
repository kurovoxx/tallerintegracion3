import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/note_markdown_body.dart';

const _noteId = '11111111-1111-4111-8111-111111111111';
const _attachmentId = '22222222-2222-4222-8222-222222222222';

final _png = base64Decode(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
);

Future<void> _pump(
  WidgetTester tester,
  Widget body, {
  Size size = const Size(800, 900),
}) async {
  await tester.binding.setSurfaceSize(size);
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: body,
        ),
      ),
    ),
  );
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  testWidgets('renderiza títulos, estilos, listas, código y enlaces', (
    tester,
  ) async {
    final opened = <String>[];
    await _pump(
      tester,
      NoteMarkdownBody(
        content:
            '# Título principal\n'
            '## Subtítulo\n'
            '**negrita** y *cursiva* y `código`\n'
            '- primer punto\n'
            '- segundo punto\n'
            '```\nprint(1)\n```\n'
            '[Documentación](https://example.com/guia)\n'
            '\n'
            'línea uno\n'
            'línea dos',
        onOpenLink: opened.add,
      ),
    );

    expect(find.text('Título principal'), findsOneWidget);
    expect(find.text('Subtítulo'), findsOneWidget);
    expect(find.text('# Título principal'), findsNothing);
    expect(find.text('## Subtítulo'), findsNothing);
    expect(find.textContaining('negrita'), findsOneWidget);
    expect(find.textContaining('cursiva'), findsOneWidget);
    expect(find.textContaining('**'), findsNothing);
    expect(find.textContaining('primer punto'), findsOneWidget);
    expect(find.textContaining('segundo punto'), findsOneWidget);
    expect(find.text('print(1)'), findsOneWidget);
    expect(find.text('Documentación'), findsOneWidget);
    expect(find.text('línea uno\nlínea dos'), findsOneWidget);

    await tester.tap(find.text('Documentación'));
    expect(opened, <String>['https://example.com/guia']);
  });

  testWidgets('no ejecuta HTML ni enlaces con esquemas peligrosos', (
    tester,
  ) async {
    final opened = <String>[];
    await _pump(
      tester,
      NoteMarkdownBody(
        content:
            '<script>alert("xss")</script>\n'
            '[malo](javascript:alert(1))\n'
            '[archivo](file:///etc/passwd)',
        onOpenLink: opened.add,
      ),
    );

    expect(
      find.textContaining('<script>alert("xss")</script>'),
      findsOneWidget,
    );
    expect(find.textContaining('malo'), findsOneWidget);
    expect(find.textContaining('archivo'), findsOneWidget);
    expect(opened, isEmpty);
    expect(tester.takeException(), isNull);
  });

  testWidgets('imagen attachment:<uuid> usa el endpoint autenticado', (
    tester,
  ) async {
    await SessionManager.saveSession('jwt-markdown', {'id': 'u1'});
    addTearDown(SessionManager.clear);
    final paths = <String>[];
    final client = MockClient((request) async {
      paths.add(request.url.path);
      expect(request.headers['Authorization'], 'Bearer jwt-markdown');
      return http.Response.bytes(
        _png,
        200,
        headers: {'content-type': 'image/png'},
      );
    });

    await _pump(
      tester,
      NoteMarkdownBody(
        content: '![Diagrama](attachment:$_attachmentId)',
        resources: <Map<String, dynamic>>[
          <String, dynamic>{
            'id': _attachmentId,
            'attachment_id': _attachmentId,
            'note_id': _noteId,
            'file_type': 'image/png',
            'file_name': 'diagrama.png',
            'external_file_id': 'drive-file',
          },
        ],
        imageBaseUrl: 'http://notes',
        imageClient: client,
      ),
    );
    await tester.pumpAndSettle();

    expect(paths, <String>[
      '/notes/$_noteId/attachments/$_attachmentId/content',
    ]);
    expect(
      find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),
      findsOneWidget,
    );
    expect(find.text('IMAGEN NO DISPONIBLE'), findsNothing);
  });

  testWidgets('referencia attachment sin fila real muestra fallback', (
    tester,
  ) async {
    await _pump(
      tester,
      const NoteMarkdownBody(
        content: '![Perdida](attachment:$_attachmentId)',
        imageBaseUrl: 'http://notes',
      ),
    );
    expect(find.text('IMAGEN NO DISPONIBLE'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
