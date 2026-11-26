import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';

import '../../core/services/api_config.dart';
import '../../core/theme/app_theme.dart';
import 'attachment_resources.dart';
import 'authenticated_attachment_image.dart';
import 'latex_support.dart';

/// Constructor de imagen para las referencias Markdown (`![alt](url)`).
///
/// Permite que cada pantalla decida cómo resolver el binario (bytes locales de
/// una nota sin subir, endpoint autenticado, red, etc.) sin duplicar el parser.
typedef NoteMarkdownImageBuilder =
    Widget Function({
      required String url,
      Map<String, dynamic>? resource,
      required BoxFit fit,
      required Widget Function() fallback,
      Widget Function(BuildContext, Widget, ImageChunkEvent?)? loadingBuilder,
      String debugLabel,
    });

/// Cuerpo de nota renderizado como Markdown visual neobrutalista: encabezados,
/// negrita/cursiva, listas con bullets sólidos, bloques de código, blockquotes,
/// enlaces, imágenes embebidas (adjunto autenticado, archivo local o URL) y
/// chips de PDF.
///
/// Es el mismo renderizador que usa el visor de TODAS LAS NOTAS, extraído para
/// que la vista previa de apuntes de otros perfiles («Ver apunte») muestre
/// Markdown real en lugar del texto crudo. No interpreta HTML: todo lo que no es una construcción Markdown
/// soportada se pinta como texto plano, así que `<script>` nunca se ejecuta.
class NoteMarkdownBody extends StatelessWidget {
  const NoteMarkdownBody({
    super.key,
    required this.content,
    this.resources = const [],
    this.onOpenResource,
    this.onOpenLink,
    this.imageBuilder,
    this.imageBaseUrl,
    this.imageClient,
  });

  final String content;

  /// Adjuntos reales de la nota (filas de `note_attachments`) para resolver
  /// referencias `attachment:<uuid>` y enlaces Drive heredados.
  final List<Map<String, dynamic>> resources;

  /// Se invoca al tocar una imagen o documento. Si es null, se muestra el
  /// recurso pero sin acción de ampliación.
  final ValueChanged<Map<String, dynamic>>? onOpenResource;

  /// Se invoca al tocar un enlace http(s)/mailto. Si es null se abre con
  /// url_launcher en la app externa.
  final ValueChanged<String>? onOpenLink;

  /// Resolución de imágenes inyectable. El visor principal pasa su builder
  /// para conservar archivos locales y adjuntos pendientes en memoria.
  final NoteMarkdownImageBuilder? imageBuilder;

  /// Base del microservicio de notas para el endpoint autenticado de adjuntos.
  /// Por defecto usa [notesApiBaseUrl].
  final String? imageBaseUrl;

  /// Cliente HTTP inyectable para pruebas del endpoint de contenido.
  final http.Client? imageClient;

  Map<String, dynamic>? _attachmentFor(String url) {
    // Secuencia correcta: crear nota → UUID nota → subir → UUID REAL
    // attachment → insertar `attachment:<uuid>` → PATCH → renderer resuelve
    // UUID → endpoint autenticado content → Image.memory.
    // Nunca usar ID temporal, timestamp, external_file_id, filename ni URL Drive.
    final inlineId = attachmentIdFromUrl(url);
    if (inlineId.isNotEmpty) {
      for (final r in resources) {
        if ((r['attachment_id'] as String?) == inlineId) return r;
      }
      return null;
    }
    final identity = resourceIdentity(url);
    final matches = resources.where(
      (r) =>
          resourceIdentity(r['url'] as String? ?? '') == identity ||
          urlMatchesAttachment(r, identity),
    );
    return matches.isEmpty ? null : matches.first;
  }

  static TextStyle get _paragraphStyle => TextStyle(
    fontSize: 14,
    height: 1.5,
    color: AppColors.text,
  );

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: _buildBlocks(content),
    );
  }

  List<Widget> _buildBlocks(String content) {
    final lines = content.split('\n');
    final widgets = <Widget>[];
    final paragraph = <String>[];

    void flushParagraph() {
      if (paragraph.isNotEmpty) {
        // Se conservan los saltos de línea del apunte: un renglón por línea
        // del Markdown, sin colapsarlos en un único párrafo.
        widgets.add(_paragraph(paragraph.join('\n')));
        paragraph.clear();
      }
    }

    var i = 0;
    while (i < lines.length) {
      final raw = lines[i];
      final line = raw.trim();

      // Bloque de código delimitado por triple backtick.
      if (line.startsWith('```')) {
        flushParagraph();
        final buf = <String>[];
        i++;
        while (i < lines.length && !lines[i].trimLeft().startsWith('```')) {
          buf.add(lines[i]);
          i++;
        }
        i++;
        widgets.add(_codeBlock(buf.join('\n')));
        continue;
      }

      // Bloque LaTeX $$ ... $$ (una o varias líneas). Sin cierre: aviso y
      // el resto se sigue mostrando como Markdown normal.
      if (line.startsWith(r'$$')) {
        flushParagraph();
        if (line.length > 4 && line.endsWith(r'$$')) {
          widgets.add(
            SafeMath(tex: line.substring(2, line.length - 2), display: true),
          );
          i++;
          continue;
        }
        final start = i;
        final buf = <String>[line.substring(2)];
        var closed = false;
        i++;
        while (i < lines.length) {
          final t = lines[i].trimRight();
          if (t.endsWith(r'$$')) {
            buf.add(t.substring(0, t.length - 2));
            closed = true;
            i++;
            break;
          }
          buf.add(lines[i]);
          i++;
        }
        if (closed) {
          widgets.add(SafeMath(tex: buf.join('\n'), display: true));
        } else {
          i = start + 1;
          widgets.add(
            const LatexFallback(
              tex: r'$$',
              reason: r'Bloque $$ sin cerrar',
              display: true,
            ),
          );
        }
        continue;
      }

      // Encabezados # / ## / ### / #### (H1-H6; H4+ se muestran como
      // subtítulo técnico compacto sin las almohadillas).
      final heading = RegExp(r'^(#{1,6})\s+(.+)$').firstMatch(line);
      if (heading != null) {
        flushParagraph();
        widgets.add(_heading(heading.group(1)!.length, heading.group(2)!));
        i++;
        continue;
      }

      // Cita / blockquote '> '.
      if (line.startsWith('> ')) {
        flushParagraph();
        widgets.add(_blockquote(line.substring(2)));
        i++;
        continue;
      }

      // Imagen standalone ![alt](url).
      final standaloneImg = RegExp(r'^!\[(.*?)\]\((.*?)\)$').firstMatch(line);
      if (standaloneImg != null) {
        flushParagraph();
        widgets.add(
          _blockImage(standaloneImg.group(1) ?? '', standaloneImg.group(2)!),
        );
        i++;
        continue;
      }

      // Documento standalone [label](url.pdf).
      final standaloneDoc = RegExp(
        r'^\[(.*?)\]\((.*?)\)$',
        caseSensitive: false,
      ).firstMatch(line);
      if (standaloneDoc != null && resourcesFromMarkdown(line).isNotEmpty) {
        flushParagraph();
        widgets.add(
          _blockDoc(standaloneDoc.group(1) ?? '', standaloneDoc.group(2)!),
        );
        i++;
        continue;
      }

      // Item de lista '- ', '* ' o '1. '.
      final listItem = RegExp(r'^([-*]|\d+\.)\s+(.+)$').firstMatch(line);
      if (listItem != null) {
        flushParagraph();
        final isOrdered = RegExp(r'^\d+\.$').hasMatch(listItem.group(1)!);
        widgets.add(_listItem(listItem.group(2)!, isOrdered));
        i++;
        continue;
      }

      if (line.isEmpty) {
        flushParagraph();
        i++;
        continue;
      }

      paragraph.add(line);
      i++;
    }
    flushParagraph();
    return widgets;
  }

  Widget _paragraph(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Text.rich(
        TextSpan(style: _paragraphStyle, children: _parseInlineSpans(text)),
      ),
    );
  }

  Widget _heading(int level, String text) {
    // H4-H6: subtítulo técnico compacto con barra de acento y tipografía
    // monoespaciada neobrutalista (sin mostrar '####').
    if (level >= 4) {
      return Padding(
        padding: const EdgeInsets.only(top: 8, bottom: 6),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              width: 4,
              height: 14,
              margin: const EdgeInsets.only(top: 2, right: 8),
              color: level == 4 ? AppColors.accentBlueDeep : AppColors.muted,
            ),
            Expanded(
              child: Text.rich(
                TextSpan(
                  style: TextStyle(
                    fontSize: 12.5,
                    height: 1.3,
                    fontWeight: FontWeight.w900,
                    fontFamily: 'monospace',
                    letterSpacing: 0.3,
                    color: AppColors.text,
                  ),
                  children: _parseInlineSpans(text),
                ),
              ),
            ),
          ],
        ),
      );
    }
    final fontSize = level == 1
        ? 20.0
        : level == 2
        ? 17.0
        : 15.0;
    final weight = level <= 2 ? FontWeight.w900 : FontWeight.w800;
    return Padding(
      padding: const EdgeInsets.only(top: 6, bottom: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text.rich(
            TextSpan(
              style: TextStyle(
                fontSize: fontSize,
                fontWeight: weight,
                color: AppColors.text,
              ),
              children: _parseInlineSpans(text),
            ),
          ),
          if (level == 1) ...[
            const SizedBox(height: 4),
            Container(height: 3, width: 48, color: AppColors.border),
          ],
        ],
      ),
    );
  }

  Widget _listItem(String text, bool ordered) {
    return Padding(
      padding: const EdgeInsets.only(left: 10, bottom: 5),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 9,
            height: 9,
            margin: const EdgeInsets.only(top: 6, right: 9),
            decoration: BoxDecoration(
              color: ordered ? AppColors.border : AppColors.text,
              borderRadius: BorderRadius.circular(ordered ? 5 : 2),
            ),
          ),
          Expanded(
            child: Text.rich(
              TextSpan(
                style: _paragraphStyle,
                children: _parseInlineSpans(text),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _blockquote(String text) {
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: AppColors.bg,
        border: Border(
          left: BorderSide(color: AppColors.outline, width: 3),
          top: BorderSide(color: AppColors.outline, width: 1),
          right: BorderSide(color: AppColors.outline, width: 1),
          bottom: BorderSide(color: AppColors.outline, width: 1),
        ),
      ),
      child: Text.rich(
        TextSpan(
          style: _paragraphStyle.copyWith(fontStyle: FontStyle.italic),
          children: _parseInlineSpans(text),
        ),
      ),
    );
  }

  Widget _blockImage(String alt, String url) {
    final resolved = _attachmentFor(url);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Align(
        alignment: Alignment.center,
        child: ConstrainedBox(
          constraints: const BoxConstraints(
            minHeight: 250,
            maxHeight: 520,
            maxWidth: 850,
          ),
          child: MouseRegion(
            cursor: onOpenResource == null
                ? MouseCursor.defer
                : SystemMouseCursors.click,
            child: GestureDetector(
              onTap: onOpenResource == null
                  ? null
                  : () => onOpenResource!({
                      'type': 'image',
                      'name': alt.isEmpty ? url : alt,
                      'url': url,
                      'size': 'Adjunto',
                    }),
              child: Container(
                clipBehavior: Clip.hardEdge,
                decoration: BoxDecoration(
                  color: AppColors.surfaceLow,
                  border: Border.all(color: AppColors.outline, width: 2),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: AppShadows.badge,
                ),
                child: Stack(
                  children: [
                    _buildImage(
                      url: url,
                      resource: resolved,
                      fit: BoxFit.contain,
                      debugLabel: 'INLINE',
                      fallback: () => Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 8,
                        ),
                        color: AppColors.surfaceLow,
                        alignment: Alignment.center,
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.broken_image_rounded,
                              size: 16,
                              color: AppColors.muted,
                            ),
                            SizedBox(width: 6),
                            Text(
                              'IMAGEN NO DISPONIBLE',
                              style: TextStyle(
                                fontSize: 9,
                                fontWeight: FontWeight.w900,
                                color: AppColors.muted,
                              ),
                            ),
                          ],
                        ),
                      ),
                      loadingBuilder: (context, child, progress) {
                        if (progress == null) return child;
                        return Container(
                          width: 200,
                          height: 64,
                          color: AppColors.surfaceLow,
                          alignment: Alignment.center,
                          child: SizedBox(
                            width: 22,
                            height: 22,
                            child: CircularProgressIndicator(
                              strokeWidth: 2.5,
                              color: AppColors.border,
                            ),
                          ),
                        );
                      },
                    ),
                    if (onOpenResource != null)
                      Positioned(
                        bottom: 6,
                        right: 6,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 7,
                            vertical: 4,
                          ),
                          decoration: BoxDecoration(
                            color: AppColors.accentYellow,
                            border: Border.all(
                              color: AppColors.outline,
                              width: 1.5,
                            ),
                            borderRadius: BorderRadius.circular(
                              AppDimens.radiusChip,
                            ),
                            boxShadow: AppShadows.badge,
                          ),
                          child: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              Icon(
                                Icons.zoom_in_rounded,
                                size: 13,
                                color: AppColors.text,
                              ),
                              SizedBox(width: 4),
                              Text(
                                'AMPLIAR',
                                style: TextStyle(
                                  fontSize: 9,
                                  fontWeight: FontWeight.w900,
                                  color: AppColors.text,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    if (alt.isNotEmpty)
                      Positioned(
                        top: 6,
                        left: 6,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 7,
                            vertical: 3,
                          ),
                          decoration: BoxDecoration(
                            color: AppColors.surface,
                            border: Border.all(
                              color: AppColors.outline,
                              width: 1.5,
                            ),
                            borderRadius: BorderRadius.circular(
                              AppDimens.radiusChip,
                            ),
                          ),
                          child: Text(
                            alt,
                            style: TextStyle(
                              fontSize: 9,
                              fontWeight: FontWeight.w900,
                              color: AppColors.text,
                            ),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _blockDoc(String label, String url) {
    final resource = resourcesFromMarkdown('[$label]($url)').single;
    final name = resource['name'] as String;
    final isPdf = resource['type'] == 'pdf';
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: MouseRegion(
        cursor: onOpenResource == null
            ? MouseCursor.defer
            : SystemMouseCursors.click,
        child: GestureDetector(
          onTap: onOpenResource == null
              ? null
              : () => onOpenResource!(resource),
          child: Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: AppColors.surface,
              border: Border.all(color: AppColors.outline, width: 2),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: AppShadows.badge,
            ),
            child: Row(
              children: [
                Container(
                  width: 36,
                  height: 36,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: AppColors.errorDeep,
                    border: Border.all(color: AppColors.outline, width: 1.5),
                    borderRadius: BorderRadius.circular(AppDimens.radius),
                  ),
                  child: Icon(
                    isPdf
                        ? Icons.picture_as_pdf_rounded
                        : Icons.description_rounded,
                    size: 18,
                    color: AppColors.surface,
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        name,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w900,
                          color: AppColors.text,
                        ),
                      ),
                      Text(
                        'Documento adjunto · Toca para abrir',
                        style: TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w700,
                          color: AppColors.muted,
                        ),
                      ),
                    ],
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 5,
                  ),
                  decoration: BoxDecoration(
                    color: AppColors.accentYellow,
                    border: Border.all(color: AppColors.outline, width: 1.5),
                    borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                    boxShadow: AppShadows.badge,
                  ),
                  child: Text(
                    'VER',
                    style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w900,
                      color: AppColors.text,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _codeBlock(String code) {
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.only(bottom: 8),
      decoration: BoxDecoration(
        color: AppColors.codeBg,
        border: Border.all(color: AppColors.outline, width: 2),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.badge,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            decoration: BoxDecoration(
              color: AppColors.surfaceLow,
              border: Border(
                bottom: BorderSide(color: AppColors.outline, width: 1.5),
              ),
            ),
            child: Row(
              children: [
                Icon(
                  Icons.terminal_rounded,
                  size: 14,
                  color: AppColors.text,
                ),
                const SizedBox(width: 6),
                Text(
                  'CÓDIGO',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                    letterSpacing: 0.5,
                  ),
                ),
                const Spacer(),
                Builder(
                  builder: (ctx) => InkWell(
                    onTap: () {
                      Clipboard.setData(ClipboardData(text: code));
                      ScaffoldMessenger.of(ctx).showSnackBar(
                        SnackBar(
                          backgroundColor: AppColors.border,
                          duration: Duration(seconds: 1),
                          content: Text(
                            'Código copiado al portapapeles',
                            style: TextStyle(
                              color: AppColors.surface,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        ),
                      );
                    },
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          Icons.copy_rounded,
                          size: 12,
                          color: AppColors.text,
                        ),
                        SizedBox(width: 4),
                        Text(
                          'COPIAR',
                          style: TextStyle(
                            fontSize: 9,
                            fontWeight: FontWeight.w900,
                            color: AppColors.text,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(10),
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: SelectableText(
                code,
                style: TextStyle(
                  fontFamily: 'monospace',
                  fontSize: 12,
                  height: 1.4,
                  color: AppColors.text,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Divide una línea en spans con estilos inline: **negrita**, *cursiva*,
  /// `código`, imágenes `![alt](ruta)`, enlaces externos y chips de documento
  /// `[label](archivo.pdf)`. Inline: separa primero las fórmulas `$...$`
  /// (renderizadas con [SafeMath]) y aplica el Markdown inline al resto.
  List<InlineSpan> _parseInlineSpans(String text) {
    if (!text.contains(r'$')) return _parseInlineMarkdown(text);
    final spans = <InlineSpan>[];
    for (final part in splitInlineMath(text)) {
      if (part.isMath) {
        spans.add(
          WidgetSpan(
            alignment: PlaceholderAlignment.middle,
            child: SafeMath(tex: part.value),
          ),
        );
      } else {
        spans.addAll(_parseInlineMarkdown(part.value));
      }
    }
    return spans;
  }

  List<InlineSpan> _parseInlineMarkdown(String text) {
    final spans = <InlineSpan>[];
    final re = RegExp(
      r'(\*\*.+?\*\*|\*[^*\n]+?\*|!\[[^\]]*\]\([^)\s]+\)|\[[^\]]+\]\([^)\s]+\)|`[^`\n]+`)',
      caseSensitive: false,
    );
    var last = 0;
    for (final m in re.allMatches(text)) {
      if (m.start > last) {
        spans.add(TextSpan(text: text.substring(last, m.start)));
      }
      final token = m.group(0)!;
      if (token.startsWith('**')) {
        spans.add(
          TextSpan(
            text: token.substring(2, token.length - 2),
            style: TextStyle(
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        );
      } else if (token.startsWith('*')) {
        spans.add(
          TextSpan(
            text: token.substring(1, token.length - 1),
            style: TextStyle(
              fontStyle: FontStyle.italic,
              color: AppColors.text,
            ),
          ),
        );
      } else if (token.startsWith('`')) {
        spans.add(
          TextSpan(
            text: token.substring(1, token.length - 1),
            style: TextStyle(
              fontFamily: 'monospace',
              fontSize: 12,
              backgroundColor: AppColors.codeBg,
              color: AppColors.text,
            ),
          ),
        );
      } else if (token.startsWith('![')) {
        final img = RegExp(r'!\[([^\]]*)\]\(([^)\s]+)\)').firstMatch(token);
        if (img != null) {
          spans.add(
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: _inlineImage(img.group(1) ?? '', img.group(2)!),
            ),
          );
        }
      } else if (token.startsWith('[')) {
        final link = RegExp(r'\[([^\]]+)\]\(([^)\s]+)\)').firstMatch(token);
        if (link == null) {
          spans.add(TextSpan(text: token));
        } else if (resourcesFromMarkdown(token).isNotEmpty) {
          spans.add(
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: _pdfChip(link.group(1)!, link.group(2)!),
            ),
          );
        } else if (_isSafeExternalLink(link.group(2)!)) {
          spans.add(
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: _inlineLink(link.group(1)!, link.group(2)!),
            ),
          );
        } else {
          // Esquemas no permitidos (javascript:, data:, file:...) se muestran
          // como texto: el Markdown nunca se convierte en HTML ejecutable.
          spans.add(TextSpan(text: link.group(1)!));
        }
      }
      last = m.end;
    }
    if (last < text.length) {
      spans.add(TextSpan(text: text.substring(last)));
    }
    return spans;
  }

  bool _isSafeExternalLink(String url) {
    final uri = Uri.tryParse(url.trim());
    if (uri == null) return false;
    switch (uri.scheme.toLowerCase()) {
      case 'http':
      case 'https':
      case 'mailto':
        return true;
      default:
        return false;
    }
  }

  Widget _inlineLink(String label, String url) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: () => _openLink(url),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 14,
            height: 1.5,
            fontWeight: FontWeight.w800,
            color: AppColors.accentBlueDeep,
            decoration: TextDecoration.underline,
            decorationColor: AppColors.accentBlueDeep,
          ),
        ),
      ),
    );
  }

  void _openLink(String url) {
    final handler = onOpenLink;
    if (handler != null) {
      handler(url);
      return;
    }
    _launchExternal(url);
  }

  Widget _inlineImage(String alt, String url) {
    return MouseRegion(
      cursor: onOpenResource == null
          ? MouseCursor.defer
          : SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onOpenResource == null
            ? null
            : () => onOpenResource!({
                'type': 'image',
                'name': alt.isEmpty ? url : alt,
                'url': url,
                'size': 'Adjunto',
              }),
        child: Container(
          width: 200,
          height: 132,
          margin: const EdgeInsets.symmetric(vertical: 4),
          clipBehavior: Clip.hardEdge,
          decoration: BoxDecoration(
            color: AppColors.surfaceLow,
            border: Border.all(
              color: AppColors.outline,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.badge,
          ),
          child: _buildImage(
            url: url,
            resource: _attachmentFor(url),
            fit: BoxFit.contain,
            debugLabel: 'INLINE',
            fallback: () => Center(
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    Icons.broken_image_rounded,
                    size: 16,
                    color: AppColors.muted,
                  ),
                  SizedBox(width: 4),
                  Text(
                    'IMAGEN NO DISPONIBLE',
                    style: TextStyle(
                      fontSize: 8,
                      fontWeight: FontWeight.w900,
                      color: AppColors.muted,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _pdfChip(String label, String url) {
    final resource = resourcesFromMarkdown('[$label]($url)').single;
    final name = resource['name'] as String;
    return MouseRegion(
      cursor: onOpenResource == null
          ? MouseCursor.defer
          : SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onOpenResource == null ? null : () => onOpenResource!(resource),
        child: Container(
          margin: const EdgeInsets.symmetric(horizontal: 2, vertical: 2),
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
          decoration: BoxDecoration(
            color: AppColors.accentYellow,
            border: Border.all(color: AppColors.outline, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                resource['type'] == 'pdf'
                    ? Icons.picture_as_pdf_rounded
                    : Icons.description_rounded,
                size: 14,
                color: AppColors.text,
              ),
              const SizedBox(width: 5),
              Flexible(
                child: Text(
                  name,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                  ),
                ),
              ),
              const SizedBox(width: 4),
              Icon(
                Icons.open_in_new_rounded,
                size: 12,
                color: AppColors.text,
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildImage({
    required String url,
    Map<String, dynamic>? resource,
    required BoxFit fit,
    required Widget Function() fallback,
    Widget Function(BuildContext, Widget, ImageChunkEvent?)? loadingBuilder,
    String debugLabel = 'ATTACHMENT',
  }) {
    final custom = imageBuilder;
    if (custom != null) {
      return custom(
        url: url,
        resource: resource,
        fit: fit,
        fallback: fallback,
        loadingBuilder: loadingBuilder,
        debugLabel: debugLabel,
      );
    }
    return _defaultImage(
      url: url,
      resource: resource,
      fit: fit,
      fallback: fallback,
      loadingBuilder: loadingBuilder,
      debugLabel: debugLabel,
    );
  }

  /// Resolución por defecto sin `dart:io`: adjunto autenticado si hay UUIDs,
  /// red para http(s) y fallback neobrutalista para el resto.
  Widget _defaultImage({
    required String url,
    Map<String, dynamic>? resource,
    required BoxFit fit,
    required Widget Function() fallback,
    Widget Function(BuildContext, Widget, ImageChunkEvent?)? loadingBuilder,
    String debugLabel = 'ATTACHMENT',
  }) {
    final noteId = resource?['note_id'];
    final attachmentId = resource?['attachment_id'];
    if (noteId is String && attachmentId is String) {
      final base = (imageBaseUrl ?? notesApiBaseUrl).replaceAll(
        RegExp(r'/+$'),
        '',
      );
      return AuthenticatedAttachmentImage(
        contentUri: Uri.parse(
          '$base/notes/${Uri.encodeComponent(noteId)}/attachments/'
          '${Uri.encodeComponent(attachmentId)}/content',
        ),
        fit: fit,
        client: imageClient,
        debugLabel: debugLabel,
        externalFileId: resource?['external_file_id'] as String?,
        onOpenDrive: () => _launchExternal(url),
      );
    }
    // Referencia inline `attachment:<uuid>` sin adjunto real resuelto:
    // no intentar Drive ni red, mostrar fallback compacto directamente.
    if (url.trim().startsWith('attachment:')) {
      debugPrint(
        '[Notes] $debugLabel attachment=${url.trim()} status=unresolved mime=-',
      );
      return fallback();
    }
    if (isDriveViewerUrl(url)) {
      return Center(
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text(
                'Vista previa no disponible aquí.',
                textAlign: TextAlign.center,
              ),
              TextButton(
                onPressed: () => _launchExternal(url),
                child: const Text('ABRIR EN DRIVE'),
              ),
            ],
          ),
        ),
      );
    }
    final lower = url.toLowerCase();
    if (lower.startsWith('http://') || lower.startsWith('https://')) {
      return Image.network(
        url,
        fit: fit,
        gaplessPlayback: true,
        errorBuilder: (_, _, _) => fallback(),
        loadingBuilder: loadingBuilder,
      );
    }
    return fallback();
  }

  Future<void> _launchExternal(String url) async {
    final uri = Uri.tryParse(url.trim());
    if (uri == null) return;
    try {
      await launchUrl(uri, mode: LaunchMode.platformDefault);
    } catch (_) {
      // Sin plugin/handler disponible (tests, escritorio sin navegador): el
      // enlace se muestra igual, simplemente no se abre.
    }
  }
}
