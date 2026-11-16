import 'dart:async';

import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Panel visible del editor en pantallas estrechas.
enum LiveEditorPane { edit, preview }

/// Editor Markdown/LaTeX con vista previa en tiempo real.
///
/// - Ancho >= [splitMinWidth]: split-view, editor a la izquierda y vista
///   previa renderizada a la derecha.
/// - Más estrecho (móvil): pestañas EDITAR / VISTA PREVIA. El editor se
///   mantiene montado (no pierde cursor ni foco al alternar).
///
/// La vista previa se actualiza con un debounce ligero ([debounce]) y solo
/// si el texto cambió, para que escribir siga fluido aunque el apunte tenga
/// muchas fórmulas.
///
/// Con [expand] el editor ocupa todo el alto que le dé su padre (debe ser
/// acotado, p. ej. dentro de un `Expanded`): editor y vista previa estiran su
/// altura y cada uno tiene su propio scroll. En ese modo [editor] debe ser un
/// `TextField` con `expands: true`.
class MarkdownLiveEditor extends StatefulWidget {
  const MarkdownLiveEditor({
    super.key,
    required this.controller,
    required this.editor,
    required this.previewBuilder,
    this.debounce = const Duration(milliseconds: 250),
    this.splitMinWidth = 720,
    this.previewMaxHeight = 420,
    this.expand = false,
  });

  /// Controlador del contenido (el mismo que usa [editor]).
  final TextEditingController controller;

  /// Campo de texto ya configurado por quien usa el editor.
  final Widget editor;

  /// Construye el render Markdown/LaTeX del texto.
  final Widget Function(String text) previewBuilder;

  final Duration debounce;
  final double splitMinWidth;
  final double previewMaxHeight;
  final bool expand;

  @override
  State<MarkdownLiveEditor> createState() => _MarkdownLiveEditorState();
}

class _MarkdownLiveEditorState extends State<MarkdownLiveEditor> {
  late String _previewText = widget.controller.text;
  Timer? _timer;
  LiveEditorPane _pane = LiveEditorPane.edit;

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_onTextChanged);
  }

  @override
  void didUpdateWidget(covariant MarkdownLiveEditor oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller.removeListener(_onTextChanged);
      widget.controller.addListener(_onTextChanged);
      _previewText = widget.controller.text;
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    widget.controller.removeListener(_onTextChanged);
    super.dispose();
  }

  void _onTextChanged() {
    // Los cambios de solo selección/cursor no re-renderizan la vista previa.
    if (widget.controller.text == _previewText) {
      _timer?.cancel();
      return;
    }
    _timer?.cancel();
    _timer = Timer(widget.debounce, _flush);
  }

  void _flush() {
    _timer?.cancel();
    if (!mounted) return;
    final text = widget.controller.text;
    if (text != _previewText) setState(() => _previewText = text);
  }

  void _selectPane(LiveEditorPane pane) {
    if (pane == LiveEditorPane.preview) _flush();
    setState(() => _pane = pane);
  }

  @override
  Widget build(BuildContext context) {
    final preview = _PreviewPanel(
      maxHeight: widget.previewMaxHeight,
      fill: widget.expand,
      isEmpty: _previewText.trim().isEmpty,
      child: RepaintBoundary(child: widget.previewBuilder(_previewText)),
    );
    return LayoutBuilder(
      builder: (context, constraints) {
        if (constraints.maxWidth >= widget.splitMinWidth) {
          return Row(
            key: const Key('live-editor-split'),
            crossAxisAlignment: widget.expand
                ? CrossAxisAlignment.stretch
                : CrossAxisAlignment.start,
            children: <Widget>[
              Expanded(child: widget.editor),
              const SizedBox(width: AppDimens.spaceMd),
              Expanded(child: preview),
            ],
          );
        }
        return Column(
          key: const Key('live-editor-tabs'),
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: widget.expand ? MainAxisSize.max : MainAxisSize.min,
          children: <Widget>[
            Wrap(
              spacing: AppDimens.spaceSm,
              runSpacing: AppDimens.spaceSm,
              children: <Widget>[
                _PaneButton(
                  key: const Key('live-editor-tab-edit'),
                  label: 'Editar',
                  icon: Icons.edit_rounded,
                  selected: _pane == LiveEditorPane.edit,
                  onPressed: () => _selectPane(LiveEditorPane.edit),
                ),
                _PaneButton(
                  key: const Key('live-editor-tab-preview'),
                  label: 'Vista previa',
                  icon: Icons.visibility_rounded,
                  selected: _pane == LiveEditorPane.preview,
                  onPressed: () => _selectPane(LiveEditorPane.preview),
                ),
              ],
            ),
            const SizedBox(height: AppDimens.spaceSm),
            if (widget.expand)
              Expanded(
                child: IndexedStack(
                  index: _pane == LiveEditorPane.edit ? 0 : 1,
                  sizing: StackFit.expand,
                  children: <Widget>[widget.editor, preview],
                ),
              )
            else
              IndexedStack(
                index: _pane == LiveEditorPane.edit ? 0 : 1,
                sizing: StackFit.passthrough,
                children: <Widget>[widget.editor, preview],
              ),
          ],
        );
      },
    );
  }
}

class _PaneButton extends StatelessWidget {
  const _PaneButton({
    super.key,
    required this.label,
    required this.icon,
    required this.selected,
    required this.onPressed,
  });

  final String label;
  final IconData icon;
  final bool selected;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return NeobrutalistButton(
      label: label,
      icon: icon,
      variant: selected
          ? NeobrutalistButtonVariant.accent
          : NeobrutalistButtonVariant.secondary,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceMd,
        vertical: AppDimens.spaceXs + 2,
      ),
      onPressed: onPressed,
    );
  }
}

class _PreviewPanel extends StatelessWidget {
  const _PreviewPanel({
    required this.child,
    required this.maxHeight,
    required this.isEmpty,
    this.fill = false,
  });

  final Widget child;
  final double maxHeight;

  /// true = ocupa todo el alto del padre (scroll interno sin tope fijo).
  final bool fill;
  final bool isEmpty;

  @override
  Widget build(BuildContext context) {
    return Container(
      key: const Key('live-editor-preview'),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.badge,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: fill ? MainAxisSize.max : MainAxisSize.min,
        children: <Widget>[
          Container(
            padding: const EdgeInsets.symmetric(
              horizontal: AppDimens.spaceMd,
              vertical: AppDimens.spaceXs + 2,
            ),
            decoration: const BoxDecoration(
              color: AppColors.surfaceLow,
              border: Border(
                bottom: BorderSide(color: AppColors.border, width: 1.5),
              ),
            ),
            child: const Row(
              children: <Widget>[
                Icon(Icons.visibility_rounded, size: 14, color: AppColors.text),
                SizedBox(width: AppDimens.spaceXs),
                Expanded(
                  child: Text(
                    'VISTA PREVIA',
                    style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      letterSpacing: 0.6,
                      color: AppColors.text,
                    ),
                  ),
                ),
                NeobrutalistBadge(
                  label: 'En vivo',
                  tone: NeobrutalistTone.success,
                  compact: true,
                ),
              ],
            ),
          ),
          if (fill)
            Expanded(child: _scrollBody())
          else
            ConstrainedBox(
              constraints: BoxConstraints(maxHeight: maxHeight),
              child: _scrollBody(),
            ),
        ],
      ),
    );
  }

  Widget _scrollBody() {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      child: isEmpty
          ? const Text(
              r'Escribe Markdown o LaTeX ($x^2$, $$...$$) y verás el resultado aquí.',
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w700,
                color: AppColors.muted,
              ),
            )
          : child,
    );
  }
}
