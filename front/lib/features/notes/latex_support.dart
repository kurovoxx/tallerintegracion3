import 'package:flutter/material.dart';
import 'package:flutter_math_fork/flutter_math.dart';

import '../../core/theme/app_theme.dart';

/// Soporte LaTeX de los apuntes: fórmulas inline `$...$` y en bloque
/// `$$...$$` dentro del Markdown.
///
/// Toda fórmula pasa por [sanitizeLatex] antes de renderizarse: se bloquean
/// macros peligrosos (definición de macros, E/S, enlaces, HTML/CSS, recursión)
/// y la sintaxis corrupta (llaves o entornos desbalanceados, fórmulas
/// gigantes). Si algo falla, [SafeMath] dibuja un fallback limpio en vez de
/// lanzar: la pantalla nunca crashea por una fórmula.

/// Longitud máxima de una fórmula (evita bloquear el hilo de UI).
const int kMaxLatexLength = 2000;

/// Profundidad máxima de llaves anidadas.
const int kMaxLatexBraceDepth = 32;

/// Comandos que nunca se renderizan: definen/redefinen macros (bucles de
/// expansión), hacen E/S, abren enlaces o inyectan HTML/CSS.
const Set<String> kBlockedLatexCommands = <String>{
  'def',
  'gdef',
  'edef',
  'xdef',
  'let',
  'futurelet',
  'newcommand',
  'renewcommand',
  'providecommand',
  'newenvironment',
  'renewenvironment',
  'DeclareMathOperator',
  'expandafter',
  'csname',
  'endcsname',
  'catcode',
  'loop',
  'repeat',
  'input',
  'include',
  'includegraphics',
  'write',
  'immediate',
  'openin',
  'openout',
  'read',
  'special',
  'href',
  'url',
  'htmlClass',
  'htmlId',
  'htmlStyle',
  'htmlData',
  'class',
  'cssId',
  'style',
  'require',
  'usepackage',
  'documentclass',
};

final RegExp _commandPattern = RegExp(r'\\([A-Za-z]+)');
final RegExp _controlChars = RegExp(r'[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]');

/// Resultado de sanitizar una fórmula: [tex] listo para renderizar, o
/// [error] con el motivo legible si no debe renderizarse.
class LatexSanitizeResult {
  const LatexSanitizeResult.ok(this.tex) : error = null;
  const LatexSanitizeResult.error(this.error) : tex = null;

  final String? tex;
  final String? error;

  bool get isOk => error == null;
}

/// Valida una fórmula antes de renderizarla. No intenta "arreglarla": si es
/// peligrosa o está corrupta devuelve un error para mostrar el fallback.
LatexSanitizeResult sanitizeLatex(String input) {
  final tex = input.replaceAll(_controlChars, '').trim();
  if (tex.isEmpty) {
    return const LatexSanitizeResult.error('Fórmula vacía');
  }
  if (tex.length > kMaxLatexLength) {
    return const LatexSanitizeResult.error(
      'Fórmula demasiado larga (máx. $kMaxLatexLength caracteres)',
    );
  }
  for (final m in _commandPattern.allMatches(tex)) {
    final name = m.group(1)!;
    if (kBlockedLatexCommands.contains(name) ||
        name.startsWith('html') ||
        name.startsWith('new') ||
        name.startsWith('renew')) {
      return LatexSanitizeResult.error('Comando no permitido: \\$name');
    }
  }
  var depth = 0;
  var maxDepth = 0;
  for (var i = 0; i < tex.length; i++) {
    final ch = tex[i];
    if (ch == r'\') {
      i++; // \{ \} \\ son literales, no agrupan.
      continue;
    }
    if (ch == '{') {
      depth++;
      if (depth > maxDepth) maxDepth = depth;
    } else if (ch == '}') {
      depth--;
      if (depth < 0) {
        return const LatexSanitizeResult.error('Llaves desbalanceadas');
      }
    }
  }
  if (depth != 0) {
    return const LatexSanitizeResult.error('Llaves desbalanceadas');
  }
  if (maxDepth > kMaxLatexBraceDepth) {
    return const LatexSanitizeResult.error('Fórmula demasiado anidada');
  }
  final begins = RegExp(r'\\begin\s*\{').allMatches(tex).length;
  final ends = RegExp(r'\\end\s*\{').allMatches(tex).length;
  if (begins != ends) {
    return const LatexSanitizeResult.error(r'\begin y \end desbalanceados');
  }
  return LatexSanitizeResult.ok(tex);
}

/// Neutraliza los comandos bloqueados dentro de las fórmulas de un Markdown
/// antes de guardarlo: `\href` pasa a `href` (texto inofensivo). El resto del
/// apunte no se toca, así que guardar nunca pierde contenido del usuario.
String sanitizeNoteMarkdown(String markdown) {
  return markdown.replaceAllMapped(
    RegExp(r'(\$\$[\s\S]+?\$\$|(?<![\\$])\$(?!\$)[^\n$]+?(?<!\\)\$)'),
    (m) => m.group(0)!.replaceAllMapped(_commandPattern, (c) {
      final name = c.group(1)!;
      final blocked =
          kBlockedLatexCommands.contains(name) ||
          name.startsWith('html') ||
          name.startsWith('new') ||
          name.startsWith('renew');
      return blocked ? name : c.group(0)!;
    }),
  );
}

/// Trozo de texto inline: texto plano o fórmula `$...$`.
class InlineMathPart {
  const InlineMathPart.text(this.value) : isMath = false;
  const InlineMathPart.math(this.value) : isMath = true;

  final String value;
  final bool isMath;
}

final RegExp _inlineMath = RegExp(
  r'(?<![\\$])\$(?!\$)([^\n$]+?)(?<!\\)\$(?!\$)',
);

/// Separa fórmulas inline `$...$` del texto. `\$` es un dólar literal y `$$`
/// no se trata como inline (es de bloque). Un `$` sin cierre queda como texto.
List<InlineMathPart> splitInlineMath(String text) {
  final parts = <InlineMathPart>[];
  var last = 0;
  for (final m in _inlineMath.allMatches(text)) {
    final inner = m.group(1)!;
    // "$5 y $10" no es una fórmula: exige que no empiece/termine con espacio.
    if (inner.trim() != inner) continue;
    if (m.start > last) {
      parts.add(InlineMathPart.text(text.substring(last, m.start)));
    }
    parts.add(InlineMathPart.math(inner));
    last = m.end;
  }
  if (last < text.length) parts.add(InlineMathPart.text(text.substring(last)));
  return parts
      .map(
        (p) =>
            p.isMath ? p : InlineMathPart.text(p.value.replaceAll(r'\$', r'$')),
      )
      .toList();
}

/// Renderiza una fórmula de forma segura: sanitiza, renderiza con
/// flutter_math y, ante cualquier error de sintaxis, muestra [LatexFallback].
class SafeMath extends StatelessWidget {
  const SafeMath({
    super.key,
    required this.tex,
    this.display = false,
    this.fontSize,
  });

  /// Tamaño por defecto de fórmulas inline (`$...$`).
  static const double inlineFontSize = 18;

  /// Tamaño por defecto de bloques (`$$...$$`): fracciones, integrales y
  /// sumatorias grandes y legibles.
  static const double displayFontSize = 22;

  final String tex;
  final bool display;

  /// null = [inlineFontSize] o [displayFontSize] según [display].
  final double? fontSize;

  static const TexParserSettings _settings = TexParserSettings(
    // Limita la expansión de macros: ninguna fórmula puede colgar la UI.
    maxExpand: 200,
    strict: Strict.ignore,
  );

  @override
  Widget build(BuildContext context) {
    final result = sanitizeLatex(tex);
    if (!result.isOk) {
      return LatexFallback(tex: tex, reason: result.error!, display: display);
    }
    final math = Math.tex(
      result.tex!,
      mathStyle: display ? MathStyle.display : MathStyle.text,
      settings: _settings,
      textStyle: TextStyle(
        fontSize: fontSize ?? (display ? displayFontSize : inlineFontSize),
        color: AppColors.text,
      ),
      onErrorFallback: (e) => LatexFallback(
        tex: tex,
        reason: 'Sintaxis LaTeX inválida',
        display: display,
      ),
    );
    if (!display) return math;
    // Bloque: centrado y con scroll horizontal para fórmulas anchas (nunca
    // desborda la pantalla en móvil).
    return Semantics(
      label: 'Fórmula: $tex',
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: AppDimens.spaceLg),
        child: SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Center(child: math),
        ),
      ),
    );
  }
}

/// Fallback visual de una fórmula que no se puede renderizar: muestra el
/// código original en monoespaciado con una etiqueta del motivo.
class LatexFallback extends StatelessWidget {
  const LatexFallback({
    super.key,
    required this.tex,
    required this.reason,
    this.display = false,
  });

  final String tex;
  final String reason;
  final bool display;

  @override
  Widget build(BuildContext context) {
    final shown = tex.length > 120 ? '${tex.substring(0, 120)}…' : tex;
    return Tooltip(
      message: reason,
      child: Container(
        key: const Key('latex-fallback'),
        margin: display
            ? const EdgeInsets.symmetric(vertical: AppDimens.spaceSm)
            : EdgeInsets.zero,
        padding: const EdgeInsets.symmetric(
          horizontal: AppDimens.spaceSm,
          vertical: 2,
        ),
        decoration: BoxDecoration(
          color: AppColors.surfaceLow,
          border: Border.all(color: AppColors.error, width: 1.5),
          borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        ),
        child: Text.rich(
          TextSpan(
            children: <InlineSpan>[
              const TextSpan(
                text: '⚠ ',
                style: TextStyle(
                  color: AppColors.errorDeep,
                  fontWeight: FontWeight.w900,
                ),
              ),
              TextSpan(
                text: display ? '$reason\n' : '',
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w900,
                  color: AppColors.errorDeep,
                ),
              ),
              TextSpan(
                text: shown,
                style: const TextStyle(
                  fontFamily: 'monospace',
                  fontSize: 12,
                  color: AppColors.text,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Fragmentos LaTeX de la barra de ayuda del editor: (etiqueta, snippet,
/// desplazamiento del cursor desde el inicio del snippet tras insertarlo).
class LatexSnippet {
  const LatexSnippet(this.label, this.snippet, this.cursorOffset, this.tooltip);

  final String label;
  final String snippet;
  final int cursorOffset;
  final String tooltip;
}

const List<LatexSnippet> kLatexSnippets = <LatexSnippet>[
  LatexSnippet(r'$x$', r'$x$', 1, 'Fórmula inline'),
  LatexSnippet(r'$$', '\n\$\$\n\n\$\$\n', 4, 'Bloque de fórmula'),
  LatexSnippet('a/b', r'\frac{a}{b}', 6, 'Fracción'),
  LatexSnippet('√', r'\sqrt{x}', 6, 'Raíz'),
  LatexSnippet('x²', r'x^{2}', 3, 'Exponente'),
  LatexSnippet('∑', r'\sum_{i=1}^{n}', 6, 'Sumatoria'),
  LatexSnippet('∫', r'\int_{a}^{b} f(x)\,dx', 6, 'Integral'),
];

/// Inserta [snippet] en la posición del cursor (reemplazando la selección) y
/// deja el cursor dentro del fragmento. Con texto seleccionado y el snippet
/// inline `$x$`, envuelve la selección: `$<selección>$`.
void insertLatexSnippet(
  TextEditingController controller,
  LatexSnippet snippet,
) {
  final text = controller.text;
  var start = controller.selection.start;
  var end = controller.selection.end;
  if (start < 0 || end < 0 || start > text.length || end > text.length) {
    start = end = text.length;
  }
  final selected = text.substring(start, end);
  final String insertion;
  final int cursor;
  if (snippet.snippet == r'$x$' && selected.isNotEmpty) {
    insertion = '\$$selected\$';
    cursor = start + insertion.length;
  } else {
    insertion = snippet.snippet;
    cursor = start + snippet.cursorOffset;
  }
  controller.value = TextEditingValue(
    text: text.replaceRange(start, end, insertion),
    selection: TextSelection.collapsed(offset: cursor),
  );
}

/// Barra de ayuda rápida LaTeX del editor de apuntes.
class LatexToolbar extends StatelessWidget {
  const LatexToolbar({
    super.key,
    required this.controller,
    required this.onChanged,
  });

  final TextEditingController controller;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      key: const Key('latex-toolbar'),
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          const Padding(
            padding: EdgeInsets.only(top: 4),
            child: Icon(
              Icons.functions_rounded,
              size: 16,
              color: AppColors.text,
            ),
          ),
          const SizedBox(width: 6),
          const Padding(
            padding: EdgeInsets.only(top: 5),
            child: Text(
              'LATEX:',
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
                letterSpacing: 0.5,
              ),
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Wrap(
              spacing: 6,
              runSpacing: 4,
              children: <Widget>[
                for (final snip in kLatexSnippets)
                  Tooltip(
                    message: snip.tooltip,
                    child: MouseRegion(
                      cursor: SystemMouseCursors.click,
                      child: GestureDetector(
                        key: Key('latex-snippet-${snip.label}'),
                        onTap: () {
                          insertLatexSnippet(controller, snip);
                          onChanged();
                        },
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 8,
                            vertical: 4,
                          ),
                          decoration: BoxDecoration(
                            color: AppColors.surface,
                            border: Border.all(
                              color: AppColors.border,
                              width: 1.5,
                            ),
                            borderRadius: BorderRadius.circular(
                              AppDimens.radiusChip,
                            ),
                            boxShadow: AppShadows.badge,
                          ),
                          child: Text(
                            snip.label,
                            style: const TextStyle(
                              fontSize: 11,
                              fontWeight: FontWeight.w900,
                              color: AppColors.text,
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
