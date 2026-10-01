// Ruta: lib/core/widgets/neobrutalism.dart
//
// Catálogo canónico de componentes neobrutalistas de Sigma Academy.
// Ver .skills/flutter_neobrutalism_ui.md (sección 4) para las reglas de uso:
// sombras duras con blurRadius 0, bordes de tinta, radios 0 (chips 4-6) y
// microinteracciones mecánicas de hundimiento.
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme/app_theme.dart';

// ---------------------------------------------------------------------------
// Responsividad
// ---------------------------------------------------------------------------

enum AppBreakpoint { compact, medium, expanded }

extension AppBreakpointContext on BuildContext {
  double get _width => MediaQuery.sizeOf(this).width;

  AppBreakpoint get breakpoint {
    if (_width < AppDimens.breakpointCompact) return AppBreakpoint.compact;
    if (_width <= AppDimens.breakpointMedium) return AppBreakpoint.medium;
    return AppBreakpoint.expanded;
  }

  bool get isCompact => breakpoint == AppBreakpoint.compact;
  bool get isMedium => breakpoint == AppBreakpoint.medium;
  bool get isExpanded => breakpoint == AppBreakpoint.expanded;
  bool get isDesktop => breakpoint == AppBreakpoint.expanded;
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

/// Centra y limita el ancho del contenido en pantallas expandidas para evitar
/// que tablas y textos se desparramen en monitores ultra-wide (4K/1440p).
class MaxWidthContainer extends StatelessWidget {
  const MaxWidthContainer({
    super.key,
    required this.child,
    this.maxWidth = AppDimens.contentMaxWidth,
    this.padding = const EdgeInsets.all(AppDimens.spaceXl),
    this.alignment = Alignment.topCenter,
  });

  final Widget child;
  final double maxWidth;
  final EdgeInsetsGeometry padding;
  final AlignmentGeometry alignment;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: alignment,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: Padding(padding: padding, child: child),
      ),
    );
  }
}

/// Área interactiva de escritorio con cursor de click explícito.
class ClickCursor extends StatelessWidget {
  const ClickCursor({super.key, required this.child, this.enabled = true});

  final Widget child;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      cursor: enabled ? SystemMouseCursors.click : MouseCursor.defer,
      child: child,
    );
  }
}

// ---------------------------------------------------------------------------
// Botones
// ---------------------------------------------------------------------------

enum NeobrutalistButtonVariant { primary, secondary, accent, info, danger }

class _ButtonPalette {
  const _ButtonPalette({required this.background, required this.foreground});

  final Color background;
  final Color foreground;
}

/// Botón con microinteracción mecánica: al presionar se traslada hacia su
/// sombra (`Offset(3,3) -> Offset(0,0)`) y la sombra desaparece; al soltar
/// recupera su altura. El foco por teclado invierte los colores.
class NeobrutalistButton extends StatefulWidget {
  const NeobrutalistButton({
    super.key,
    required this.label,
    this.onPressed,
    this.icon,
    this.trailing,
    this.variant = NeobrutalistButtonVariant.primary,
    this.expand = false,
    this.borderWidth = AppDimens.borderWidth,
    this.padding = const EdgeInsets.symmetric(
      horizontal: AppDimens.spaceLg,
      vertical: AppDimens.spaceMd,
    ),
    this.focusNode,
    this.semanticLabel,
  });

  final String label;
  final VoidCallback? onPressed;
  final IconData? icon;
  final Widget? trailing;
  final NeobrutalistButtonVariant variant;
  final bool expand;
  final double borderWidth;
  final EdgeInsetsGeometry padding;
  final FocusNode? focusNode;
  final String? semanticLabel;

  @override
  State<NeobrutalistButton> createState() => _NeobrutalistButtonState();
}

class _NeobrutalistButtonState extends State<NeobrutalistButton> {
  bool _pressed = false;
  bool _focused = false;

  bool get _enabled => widget.onPressed != null;

  _ButtonPalette get _palette {
    switch (widget.variant) {
      case NeobrutalistButtonVariant.primary:
        return const _ButtonPalette(
          background: AppColors.border,
          foreground: AppColors.surface,
        );
      case NeobrutalistButtonVariant.secondary:
        return const _ButtonPalette(
          background: AppColors.surface,
          foreground: AppColors.text,
        );
      case NeobrutalistButtonVariant.accent:
        return const _ButtonPalette(
          background: AppColors.accentYellow,
          foreground: AppColors.text,
        );
      case NeobrutalistButtonVariant.info:
        return const _ButtonPalette(
          background: AppColors.accentBlueDeep,
          foreground: AppColors.surface,
        );
      case NeobrutalistButtonVariant.danger:
        return const _ButtonPalette(
          background: AppColors.errorDeep,
          foreground: AppColors.surface,
        );
    }
  }

  void _setPressed(bool value) {
    if (!_enabled || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final palette = _palette;
    final background = !_enabled
        ? AppColors.surfaceLow
        : (_focused ? palette.foreground : palette.background);
    final foreground = !_enabled
        ? AppColors.muted
        : (_focused ? palette.background : palette.foreground);
    final borderColor = _enabled ? AppColors.border : AppColors.muted;
    final pressed = _pressed && _enabled;

    final content = AnimatedContainer(
      duration: AppMotion.press,
      curve: AppMotion.standard,
      transform: Matrix4.translationValues(
        pressed ? AppShadows.offsetButton.dx : 0,
        pressed ? AppShadows.offsetButton.dy : 0,
        0,
      ),
      padding: widget.padding,
      decoration: BoxDecoration(
        color: background,
        border: Border.all(color: borderColor, width: widget.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: pressed || !_enabled
            ? const <BoxShadow>[]
            : AppShadows.button,
      ),
      child: Row(
        mainAxisSize: widget.expand ? MainAxisSize.max : MainAxisSize.min,
        mainAxisAlignment: MainAxisAlignment.center,
        children: <Widget>[
          if (widget.icon != null) ...<Widget>[
            Icon(widget.icon, size: 16, color: foreground),
            const SizedBox(width: AppDimens.spaceSm),
          ],
          Flexible(
            child: Text(
              widget.label.toUpperCase(),
              textAlign: TextAlign.center,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.5,
                color: foreground,
              ),
            ),
          ),
          if (widget.trailing != null) ...<Widget>[
            const SizedBox(width: AppDimens.spaceSm),
            widget.trailing!,
          ],
        ],
      ),
    );

    return Semantics(
      button: true,
      enabled: _enabled,
      label: widget.semanticLabel ?? widget.label,
      child: MouseRegion(
        cursor: _enabled
            ? SystemMouseCursors.click
            : SystemMouseCursors.forbidden,
        child: Focus(
          focusNode: widget.focusNode,
          onFocusChange: (bool value) => setState(() => _focused = value),
          onKeyEvent: (_, KeyEvent event) {
            if (!_enabled) return KeyEventResult.ignored;
            final isActivation = event is KeyDownEvent &&
                (event.logicalKey == LogicalKeyboardKey.enter ||
                    event.logicalKey == LogicalKeyboardKey.space);
            if (isActivation) {
              widget.onPressed?.call();
              return KeyEventResult.handled;
            }
            return KeyEventResult.ignored;
          },
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTapDown: _enabled ? (_) => _setPressed(true) : null,
            onTapUp: _enabled ? (_) => _setPressed(false) : null,
            onTapCancel: _enabled ? () => _setPressed(false) : null,
            onTap: widget.onPressed,
            child: widget.expand
                ? SizedBox(width: double.infinity, child: content)
                : content,
          ),
        ),
      ),
    );
  }
}

/// Botón cuadrado de ícono (acciones de toolbar, rail y FAB compacto).
class NeobrutalistIconButton extends StatefulWidget {
  const NeobrutalistIconButton({
    super.key,
    required this.icon,
    this.onPressed,
    this.tooltip,
    this.size = 44,
    this.borderWidth = AppDimens.borderWidth,
    this.variant = NeobrutalistButtonVariant.secondary,
    this.focusNode,
  });

  final IconData icon;
  final VoidCallback? onPressed;
  final String? tooltip;
  final double size;
  final double borderWidth;
  final NeobrutalistButtonVariant variant;
  final FocusNode? focusNode;

  @override
  State<NeobrutalistIconButton> createState() =>
      _NeobrutalistIconButtonState();
}

class _NeobrutalistIconButtonState extends State<NeobrutalistIconButton> {
  bool _pressed = false;

  bool get _enabled => widget.onPressed != null;

  Color get _background {
    if (!_enabled) return AppColors.surfaceLow;
    switch (widget.variant) {
      case NeobrutalistButtonVariant.primary:
        return AppColors.border;
      case NeobrutalistButtonVariant.accent:
        return AppColors.accentYellow;
      case NeobrutalistButtonVariant.info:
        return AppColors.accentBlueDeep;
      case NeobrutalistButtonVariant.danger:
        return AppColors.errorDeep;
      case NeobrutalistButtonVariant.secondary:
        return AppColors.surface;
    }
  }

  Color get _foreground {
    if (!_enabled) return AppColors.muted;
    switch (widget.variant) {
      case NeobrutalistButtonVariant.primary:
      case NeobrutalistButtonVariant.info:
      case NeobrutalistButtonVariant.danger:
        return AppColors.surface;
      case NeobrutalistButtonVariant.secondary:
      case NeobrutalistButtonVariant.accent:
        return AppColors.text;
    }
  }

  void _setPressed(bool value) {
    if (!_enabled || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final pressed = _pressed && _enabled;
    final button = AnimatedContainer(
      duration: AppMotion.press,
      curve: AppMotion.standard,
      width: widget.size,
      height: widget.size,
      transform: Matrix4.translationValues(
        pressed ? AppShadows.offsetButton.dx : 0,
        pressed ? AppShadows.offsetButton.dy : 0,
        0,
      ),
      decoration: BoxDecoration(
        color: _background,
        border: Border.all(
          color: _enabled ? AppColors.border : AppColors.muted,
          width: widget.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow:
            pressed || !_enabled ? const <BoxShadow>[] : AppShadows.button,
      ),
      child: Icon(widget.icon, size: 20, color: _foreground),
    );

    final interactive = MouseRegion(
      cursor: _enabled
          ? SystemMouseCursors.click
          : SystemMouseCursors.forbidden,
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTapDown: _enabled ? (_) => _setPressed(true) : null,
        onTapUp: _enabled ? (_) => _setPressed(false) : null,
        onTapCancel: _enabled ? () => _setPressed(false) : null,
        onTap: widget.onPressed,
        child: button,
      ),
    );

    if (widget.tooltip == null) return interactive;
    return Tooltip(message: widget.tooltip!, child: interactive);
  }
}

/// FAB neobrutalista cuadrado (56x56) con hundimiento mecánico: al presionar
/// la pieza se acerca a su sombra (hueco 2,2 -> 0,0).
class NeobrutalistFab extends StatefulWidget {
  const NeobrutalistFab({
    super.key,
    this.onPressed,
    this.icon = Icons.add_rounded,
    this.tooltip,
  });

  final VoidCallback? onPressed;
  final IconData icon;
  final String? tooltip;

  @override
  State<NeobrutalistFab> createState() => _NeobrutalistFabState();
}

class _NeobrutalistFabState extends State<NeobrutalistFab> {
  bool _pressed = false;

  void _setPressed(bool value) {
    if (widget.onPressed == null || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final enabled = widget.onPressed != null;
    final pressed = _pressed && enabled;
    final fab = AnimatedContainer(
      duration: AppMotion.press,
      curve: AppMotion.standard,
      width: 56,
      height: 56,
      transform: Matrix4.translationValues(
        pressed ? AppShadows.offsetBadge.dx : 0,
        pressed ? AppShadows.offsetBadge.dy : 0,
        0,
      ),
      decoration: BoxDecoration(
        color: enabled ? AppColors.accentYellow : AppColors.surfaceLow,
        border: Border.all(
          color: enabled ? AppColors.border : AppColors.muted,
          width: AppDimens.borderWidthAction,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: pressed
            ? AppShadows.badge
            : (enabled ? AppShadows.card : const <BoxShadow>[]),
      ),
      child: Icon(
        widget.icon,
        size: 24,
        color: enabled ? AppColors.text : AppColors.muted,
      ),
    );

    final interactive = MouseRegion(
      cursor:
          enabled ? SystemMouseCursors.click : SystemMouseCursors.forbidden,
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTapDown: enabled ? (_) => _setPressed(true) : null,
        onTapUp: enabled ? (_) => _setPressed(false) : null,
        onTapCancel: enabled ? () => _setPressed(false) : null,
        onTap: widget.onPressed,
        child: fab,
      ),
    );

    if (widget.tooltip == null) return interactive;
    return Tooltip(message: widget.tooltip!, child: interactive);
  }
}

// ---------------------------------------------------------------------------
// Badges
// ---------------------------------------------------------------------------

enum NeobrutalistTone { neutral, accent, info, success, danger, pending }

extension NeobrutalistToneStyle on NeobrutalistTone {
  Color get background {
    switch (this) {
      case NeobrutalistTone.neutral:
        return AppColors.surfaceLow;
      case NeobrutalistTone.accent:
        return AppColors.accentYellow;
      case NeobrutalistTone.info:
        return AppColors.accentBlueDeep;
      case NeobrutalistTone.success:
        return AppColors.success;
      case NeobrutalistTone.danger:
        return AppColors.errorDeep;
      case NeobrutalistTone.pending:
        return AppColors.pendingLight;
    }
  }

  Color get foreground {
    switch (this) {
      case NeobrutalistTone.info:
      case NeobrutalistTone.danger:
        return AppColors.surface;
      case NeobrutalistTone.neutral:
      case NeobrutalistTone.accent:
      case NeobrutalistTone.success:
      case NeobrutalistTone.pending:
        return AppColors.text;
    }
  }
}

/// Badge tipo calcomanía: borde 1.5 px, radio 4 px, sombra dura (2,2) y
/// tipografía pesada en mayúsculas.
class NeobrutalistBadge extends StatelessWidget {
  const NeobrutalistBadge({
    super.key,
    required this.label,
    this.tone = NeobrutalistTone.neutral,
    this.icon,
    this.compact = false,
  });

  final String label;
  final NeobrutalistTone tone;
  final IconData? icon;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final foreground = tone.foreground;
    return Container(
      padding: EdgeInsets.symmetric(
        horizontal: compact ? 6 : AppDimens.spaceSm,
        vertical: AppDimens.spaceXs,
      ),
      decoration: BoxDecoration(
        color: tone.background,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          if (icon != null) ...<Widget>[
            Icon(icon, size: 12, color: foreground),
            const SizedBox(width: AppDimens.spaceXs),
          ],
          Text(
            label.toUpperCase(),
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
              color: foreground,
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Diálogos (portado de d0ef02c de Héctor sobre tokens actuales)
// ---------------------------------------------------------------------------

/// Diálogo neobrutalista de silueta rectangular dura: borde de tinta de 3 px,
/// radio según token, sombra rígida sin blur, header con separador y acciones.
class NeobrutalistDialog extends StatelessWidget {
  const NeobrutalistDialog({
    super.key,
    required this.title,
    required this.content,
    this.confirmLabel,
    this.cancelLabel,
    this.onConfirm,
    this.confirmVariant = NeobrutalistButtonVariant.accent,
    this.closeOnConfirm = true,
    this.maxWidth = 480,
  });

  final String title;
  final Widget content;
  final String? confirmLabel;
  final String? cancelLabel;
  final VoidCallback? onConfirm;
  final NeobrutalistButtonVariant confirmVariant;

  /// false = onConfirm decide cuándo cerrar (validación, async o pop con dato).
  final bool closeOnConfirm;

  /// Ancho máximo del marco (480 formularios; visor de imagen usa ~960).
  final double maxWidth;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: BoxConstraints(maxWidth: maxWidth),
      child: Container(
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidthThick,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: AppShadows.dialog,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 18, 20, 14),
              child: Text(
                title.toUpperCase(),
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w900,
                  letterSpacing: 0.6,
                  color: AppColors.text,
                ),
              ),
            ),
            const Divider(
              color: AppColors.border,
              thickness: AppDimens.borderWidth,
              height: 1,
            ),
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(20),
                child: content,
              ),
            ),
            if (confirmLabel != null || cancelLabel != null) ...<Widget>[
              const Divider(
                color: AppColors.border,
                thickness: AppDimens.borderWidth,
                height: 1,
              ),
              Padding(
                padding: const EdgeInsets.all(AppDimens.spaceLg),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: <Widget>[
                    if (cancelLabel != null) ...<Widget>[
                      NeobrutalistButton(
                        label: cancelLabel!,
                        variant: NeobrutalistButtonVariant.secondary,
                        onPressed: () => Navigator.of(context).pop(),
                      ),
                      const SizedBox(width: AppDimens.spaceMd),
                    ],
                    if (confirmLabel != null)
                      NeobrutalistButton(
                        label: confirmLabel!,
                        variant: confirmVariant,
                        onPressed: onConfirm == null
                            ? null
                            : () {
                                onConfirm!.call();
                                if (closeOnConfirm) {
                                  Navigator.of(context).pop();
                                }
                              },
                      ),
                  ],
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// Muestra un [NeobrutalistDialog] con `barrierColor: AppColors.scrim`.
Future<T?> showNeobrutalistDialog<T>({
  required BuildContext context,
  required Widget dialog,
  bool barrierDismissible = true,
}) {
  return showDialog<T>(
    context: context,
    barrierDismissible: barrierDismissible,
    barrierColor: AppColors.scrim,
    builder: (_) => Dialog(
      backgroundColor: Colors.transparent,
      elevation: 0,
      insetPadding: const EdgeInsets.all(AppDimens.spaceXl),
      child: dialog,
    ),
  );
}
