import 'package:flutter/material.dart';
import 'package:flutter_math_fork/flutter_math.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart'
    show debugBuildNoteMarkdown;
import 'package:taller_integracion_front/features/notes/note_live_editor.dart';

Future<TextEditingController> _pump(
  WidgetTester tester, {
  required Size size,
  String initial = '',
  double textScale = 1,
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  tester.platformDispatcher.textScaleFactorTestValue = textScale;
  addTearDown(() {
    tester.view.resetPhysicalSize();
    tester.view.resetDevicePixelRatio();
    tester.platformDispatcher.clearTextScaleFactorTestValue();
  });
  final controller = TextEditingController(text: initial);
  addTearDown(controller.dispose);
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: MarkdownLiveEditor(
            controller: controller,
            editor: TextField(
              key: const Key('editor-field'),
              controller: controller,
              minLines: 4,
              maxLines: 10,
            ),
            previewBuilder: debugBuildNoteMarkdown,
          ),
        ),
      ),
    ),
  );
  return controller;
}

Finder _inPreview(Finder f) => find.descendant(
  of: find.byKey(const Key('live-editor-preview')),
  matching: f,
);

void main() {
  testWidgets('escritorio: split-view y vista previa con debounce', (
    tester,
  ) async {
    await _pump(tester, size: const Size(1200, 900), initial: '# Hola');
    expect(find.byKey(const Key('live-editor-split')), findsOneWidget);
    expect(_inPreview(find.textContaining('Hola')), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('editor-field')),
      r'Energía: $E = mc^2$',
    );
    // Antes del debounce la vista previa aún no cambia (escritura fluida).
    await tester.pump(const Duration(milliseconds: 100));
    expect(_inPreview(find.byType(Math)), findsNothing);
    // Tras el debounce se renderiza el LaTeX.
    await tester.pump(const Duration(milliseconds: 300));
    expect(_inPreview(find.byType(Math)), findsOneWidget);
    expect(_inPreview(find.textContaining('Energía')), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('móvil: pestañas Editar / Vista previa conservan el texto', (
    tester,
  ) async {
    final controller = await _pump(tester, size: const Size(360, 800));
    expect(find.byKey(const Key('live-editor-tabs')), findsOneWidget);
    expect(find.byKey(const Key('editor-field')), findsOneWidget);
    expect(find.byKey(const Key('live-editor-preview')), findsNothing);

    await tester.enterText(
      find.byKey(const Key('editor-field')),
      'Bloque:\n\$\$\n\\frac{1}{2}\n\$\$',
    );
    // Cambiar de pestaña aplica el texto al instante (sin esperar debounce).
    await tester.tap(find.byKey(const Key('live-editor-tab-preview')));
    await tester.pump();
    expect(find.byKey(const Key('editor-field')), findsNothing);
    expect(_inPreview(find.byType(Math)), findsOneWidget);

    await tester.tap(find.byKey(const Key('live-editor-tab-edit')));
    await tester.pump();
    expect(find.byKey(const Key('editor-field')), findsOneWidget);
    expect(controller.text, contains(r'\frac{1}{2}'));
    await tester.pump(const Duration(milliseconds: 300));
  });

  testWidgets('LaTeX inválido en la vista previa no rompe la pantalla', (
    tester,
  ) async {
    await _pump(
      tester,
      size: const Size(1200, 900),
      initial: r'Roto: $\frac{1}{$ y peligroso: $$\def\x{\x}\x$$',
    );
    expect(tester.takeException(), isNull);
    expect(_inPreview(find.byKey(const Key('latex-fallback'))), findsWidgets);
  });

  testWidgets('sin desbordes en 360 px con texto al 200 %', (tester) async {
    await _pump(
      tester,
      size: const Size(360, 800),
      textScale: 2,
      initial: r'$$\sum_{i=1}^{n} i = \frac{n(n+1)}{2}$$',
    );
    await tester.tap(find.byKey(const Key('live-editor-tab-preview')));
    await tester.pump();
    expect(tester.takeException(), isNull);
  });
}
