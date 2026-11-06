import 'package:flutter/material.dart';
import 'package:flutter_math_fork/flutter_math.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart'
    show debugBuildNoteMarkdown;
import 'package:taller_integracion_front/features/notes/latex_support.dart';

Widget _host(Widget child, {double width = 800}) => MaterialApp(
  home: Scaffold(
    body: Center(
      child: SizedBox(width: width, child: child),
    ),
  ),
);

void main() {
  group('sanitizeLatex', () {
    test('acepta fórmulas comunes', () {
      for (final tex in [
        r'\frac{a}{b}',
        r'\int_{0}^{\infty} e^{-x^2}\,dx = \frac{\sqrt{\pi}}{2}',
        r'\begin{pmatrix} 1 & 0 \\ 0 & 1 \end{pmatrix}',
        r'\{x \mid x > 0\}',
      ]) {
        expect(sanitizeLatex(tex).isOk, isTrue, reason: tex);
      }
    });

    test('bloquea macros peligrosos', () {
      for (final tex in [
        r'\href{javascript:alert(1)}{x}',
        r'\def\x{\x\x}\x',
        r'\newcommand{\a}{b}',
        r'\renewcommand{\frac}{x}',
        r'\htmlStyle{color:red}{x}',
        r'\url{http://evil}',
        r'\input{/etc/passwd}',
        r'\csname foo\endcsname',
      ]) {
        final r = sanitizeLatex(tex);
        expect(r.isOk, isFalse, reason: tex);
        expect(r.error, startsWith('Comando no permitido'), reason: tex);
      }
    });

    test('rechaza sintaxis corrupta y fórmulas gigantes', () {
      expect(sanitizeLatex(r'\frac{a}{b').error, 'Llaves desbalanceadas');
      expect(sanitizeLatex(r'x}').error, 'Llaves desbalanceadas');
      expect(
        sanitizeLatex(r'\begin{matrix} 1').error,
        r'\begin y \end desbalanceados',
      );
      expect(sanitizeLatex('{' * 40 + '}' * 40).error, contains('anidada'));
      expect(sanitizeLatex('x' * 2001).error, contains('demasiado larga'));
      expect(sanitizeLatex('   ').error, 'Fórmula vacía');
    });

    test('quita caracteres de control', () {
      expect(sanitizeLatex('x\u0000+y').tex, 'x+y');
    });
  });

  test('sanitizeNoteMarkdown neutraliza solo dentro de fórmulas', () {
    const md =
        'Texto \\href{a}{b} fuera.\n'
        r'Inline $\href{http://x}{y} + 1$ y bloque:'
        '\n'
        r'$$\def\loop{\loop}\frac{1}{2}$$';
    final out = sanitizeNoteMarkdown(md);
    expect(out, contains(r'Texto \href{a}{b} fuera.'));
    expect(out, contains(r'$href{http://x}{y} + 1$'));
    // \def y \loop pierden la barra; \frac (permitido) se conserva.
    expect(out, contains(r'$$defloop{loop}\frac{1}{2}$$'));
  });

  group('splitInlineMath', () {
    test('separa fórmulas y respeta \\\$ literal', () {
      final parts = splitInlineMath(r'Área $\pi r^2$ y precio \$5');
      expect(parts.map((p) => p.isMath).toList(), [false, true, false]);
      expect(parts[1].value, r'\pi r^2');
      expect(parts[2].value, r' y precio $5');
    });

    test('no confunde montos con fórmulas', () {
      final parts = splitInlineMath(r'Cuesta $5 y $10 en total');
      expect(parts.every((p) => !p.isMath), isTrue);
    });

    test('ignora \$\$ (es de bloque)', () {
      expect(splitInlineMath(r'$$x$$').every((p) => !p.isMath), isTrue);
    });
  });

  group('SafeMath', () {
    testWidgets('renderiza una fórmula válida', (tester) async {
      await tester.pumpWidget(_host(const SafeMath(tex: r'\frac{a}{b}')));
      expect(find.byType(Math), findsOneWidget);
      expect(find.byKey(const Key('latex-fallback')), findsNothing);
    });

    testWidgets('macro bloqueado: fallback, sin excepción', (tester) async {
      await tester.pumpWidget(_host(const SafeMath(tex: r'\def\x{\x}\x')));
      expect(tester.takeException(), isNull);
      expect(find.byKey(const Key('latex-fallback')), findsOneWidget);
      expect(find.byType(Math), findsNothing);
    });

    testWidgets('sintaxis inválida para el parser: fallback limpio', (
      tester,
    ) async {
      for (final tex in [r'\frac{a}', r'x^^^2', r'\notacommand{x}', r'a & b']) {
        await tester.pumpWidget(_host(SafeMath(tex: tex, display: true)));
        expect(tester.takeException(), isNull, reason: tex);
      }
    });

    testWidgets('bloque ancho no desborda en 360 px', (tester) async {
      final wide = List.filled(60, r'\alpha + ').join() + r'\omega';
      await tester.pumpWidget(
        _host(SafeMath(tex: wide, display: true), width: 360),
      );
      expect(tester.takeException(), isNull);
    });
  });

  group('barra LaTeX', () {
    test('envuelve la selección con \$...\$', () {
      final c = TextEditingController(text: 'area pi r');
      c.selection = const TextSelection(baseOffset: 5, extentOffset: 9);
      insertLatexSnippet(c, kLatexSnippets.first);
      expect(c.text, r'area $pi r$');
      expect(c.selection.baseOffset, c.text.length);
    });

    testWidgets('insertar fracción deja el cursor en el numerador', (
      tester,
    ) async {
      final c = TextEditingController(text: 'x = ');
      c.selection = const TextSelection.collapsed(offset: 4);
      var changed = 0;
      await tester.pumpWidget(
        _host(LatexToolbar(controller: c, onChanged: () => changed++)),
      );
      await tester.tap(find.byKey(const Key('latex-snippet-a/b')));
      expect(c.text, r'x = \frac{a}{b}');
      expect(c.selection.baseOffset, 4 + 6);
      expect(changed, 1);
    });
  });

  testWidgets('visor de notas: Markdown + LaTeX inline y en bloque', (
    tester,
  ) async {
    const content =
        '# Cinemática\n'
        r'La posición es $x(t) = x_0 + v t$ y **la velocidad** es constante.'
        '\n\n'
        r'$$'
        '\n'
        r'\int_0^T v\,dt = x(T) - x_0'
        '\n'
        r'$$'
        '\n'
        r'Fórmula rota: $\frac{a}{$ y bloqueada: $\href{x}{y}$'
        '\n'
        r'$$ sin cerrar';
    await tester.pumpWidget(
      _host(SingleChildScrollView(child: debugBuildNoteMarkdown(content))),
    );
    expect(tester.takeException(), isNull);
    // 1 inline + 1 bloque renderizados con flutter_math.
    expect(find.byType(Math), findsNWidgets(2));
    // Rota, bloqueada y bloque sin cerrar: fallback visual, sin crash.
    expect(find.byKey(const Key('latex-fallback')), findsNWidgets(3));
    expect(find.textContaining('Cinemática'), findsOneWidget);
  });
}
