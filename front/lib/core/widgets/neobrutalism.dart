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

/// Borde de un botón: en Sigma siempre la tinta; en UCT los botones rellenos
/// usan su propio color (sin contorno visible) y el secundario un trazo suave.
Color _variantBorder(NeobrutalistButtonVariant variant, Color background) {
  if (!AppThemeController.isUct) return AppColors.border;
  return variant == NeobrutalistButtonVariant.secondary
      ? AppColors.outline
      : background;
}

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
        return _ButtonPalette(
          background: AppColors.primary,
          foreground: AppColors.onPrimary,
        );
      case NeobrutalistButtonVariant.secondary:
        return _ButtonPalette(
          background: AppColors.surface,
          foreground: AppThemeController.isUct
              ? AppColors.border
              : AppColors.text,
        );
      case NeobrutalistButtonVariant.accent:
        return _ButtonPalette(
          background: AppColors.accentYellow,
          foreground: AppColors.onAccent,
        );
      case NeobrutalistButtonVariant.info:
        return _ButtonPalette(
          background: AppColors.accentBlueDeep,
          foreground: AppColors.onPrimary,
        );
      case NeobrutalistButtonVariant.danger:
        return _ButtonPalette(
          background: AppColors.errorDeep,
          foreground: AppColors.onPrimary,
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
    final borderColor = !_enabled
        ? AppColors.muted
        : _variantBorder(widget.variant, palette.background);
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
            final isActivation =
                event is KeyDownEvent &&
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
  State<NeobrutalistIconButton> createState() => _NeobrutalistIconButtonState();
}

class _NeobrutalistIconButtonState extends State<NeobrutalistIconButton> {
  bool _pressed = false;

  bool get _enabled => widget.onPressed != null;

  Color get _background {
    if (!_enabled) return AppColors.surfaceLow;
    switch (widget.variant) {
      case NeobrutalistButtonVariant.primary:
        return AppColors.primary;
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
        return AppColors.onPrimary;
      case NeobrutalistButtonVariant.secondary:
        return AppThemeController.isUct ? AppColors.border : AppColors.text;
      case NeobrutalistButtonVariant.accent:
        return AppColors.onAccent;
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
          color: _enabled
              ? _variantBorder(widget.variant, _background)
              : AppColors.muted,
          width: widget.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: pressed || !_enabled
            ? const <BoxShadow>[]
            : AppShadows.button,
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
          color: !enabled
              ? AppColors.muted
              : (AppThemeController.isUct
                    ? AppColors.accentYellow
                    : AppColors.border),
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
        color: enabled ? AppColors.onAccent : AppColors.muted,
      ),
    );

    final interactive = MouseRegion(
      cursor: enabled ? SystemMouseCursors.click : SystemMouseCursors.forbidden,
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
        border: Border.all(color: AppColors.outline, width: 1.5),
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
          // Flexible: en anchos angostos o con texto al 200 % la etiqueta
          // baja de línea en vez de desbordar la calcomanía.
          Flexible(
            child: Text(
              label.toUpperCase(),
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.6,
                color: foreground,
              ),
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
    this.actions,
    this.expandContent = false,
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

  /// Acciones propias del pie (reemplazan a cancelar/confirmar). Sirve cuando
  /// el diálogo cambia de estado y debe bloquear o variar sus botones, p. ej.
  /// mientras procesa una acción irreversible.
  final List<Widget>? actions;

  /// true = el marco ocupa todo el alto disponible y [content] recibe ese
  /// espacio acotado (sin scroll envolvente), para que pueda usar
  /// `Expanded`/`Flexible` y gestionar su propio scroll interno.
  final bool expandContent;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: BoxConstraints(maxWidth: maxWidth),
      child: Container(
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.outline,
            width: AppDimens.borderWidthThick,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: AppShadows.dialog,
        ),
        child: Column(
          mainAxisSize: expandContent ? MainAxisSize.max : MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 18, 20, 14),
              child: Text(
                title.toUpperCase(),
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w900,
                  letterSpacing: 0.6,
                  color: AppColors.text,
                ),
              ),
            ),
            Divider(
              color: AppColors.border,
              thickness: AppDimens.borderWidth,
              height: 1,
            ),
            if (expandContent)
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.all(20),
                  child: content,
                ),
              )
            else
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.all(20),
                  child: content,
                ),
              ),
            if (actions != null && actions!.isNotEmpty) ...<Widget>[
              Divider(
                color: AppColors.border,
                thickness: AppDimens.borderWidth,
                height: 1,
              ),
              Padding(
                padding: const EdgeInsets.all(AppDimens.spaceLg),
                child: Wrap(
                  alignment: WrapAlignment.end,
                  spacing: AppDimens.spaceMd,
                  runSpacing: AppDimens.spaceSm,
                  children: actions!,
                ),
              ),
            ] else if (confirmLabel != null || cancelLabel != null) ...<Widget>[
              Divider(
                color: AppColors.border,
                thickness: AppDimens.borderWidth,
                height: 1,
              ),
              Padding(
                padding: const EdgeInsets.all(AppDimens.spaceLg),
                // Wrap alineado a la derecha: igual que el Row cuando caben,
                // pero en pantallas angostas o con texto ampliado las acciones
                // bajan de línea en vez de desbordar.
                child: Wrap(
                  alignment: WrapAlignment.end,
                  spacing: AppDimens.spaceMd,
                  runSpacing: AppDimens.spaceSm,
                  children: <Widget>[
                    if (cancelLabel != null)
                      NeobrutalistButton(
                        label: cancelLabel!,
                        variant: NeobrutalistButtonVariant.secondary,
                        onPressed: () => Navigator.of(context).pop(),
                      ),
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
  EdgeInsets insetPadding = const EdgeInsets.all(AppDimens.spaceXl),
}) {
  return showDialog<T>(
    context: context,
    barrierDismissible: barrierDismissible,
    barrierColor: AppColors.scrim,
    builder: (_) => Dialog(
      backgroundColor: Colors.transparent,
      elevation: 0,
      insetPadding: insetPadding,
      child: dialog,
    ),
  );
}

/// Tarjeta rígida neobrutalista: borde de tinta de 2 px, radio 0, sombra dura
/// `Offset(4,4)` sin desenfoque, con header/footer opcionales separados por
/// `Divider` de tinta. Interactiva (onTap) añade cursor y semántica.
class NeobrutalistCard extends StatelessWidget {
  const NeobrutalistCard({
    super.key,
    required this.child,
    this.title,
    this.trailing,
    this.footer,
    this.backgroundColor,
    this.padding = const EdgeInsets.all(AppDimens.spaceLg),
    this.shadow,
    this.borderWidth = AppDimens.borderWidth,
    this.onTap,
    this.semanticLabel,
  });

  final Widget child;
  final String? title;
  final Widget? trailing;
  final Widget? footer;
  final Color? backgroundColor;
  final EdgeInsetsGeometry padding;
  final BoxShadow? shadow;
  final double borderWidth;
  final VoidCallback? onTap;
  final String? semanticLabel;

  @override
  Widget build(BuildContext context) {
    final card = Container(
      decoration: BoxDecoration(
        color: (backgroundColor ?? AppColors.surface),
        border: Border.all(color: AppColors.outline, width: borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: <BoxShadow>[(shadow ?? AppShadows.hard4)],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: <Widget>[
          if (title != null) ...<Widget>[
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 14, 16, 12),
              child: Row(
                children: <Widget>[
                  Expanded(
                    child: Text(
                      title!.toUpperCase(),
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w900,
                        letterSpacing: 0.6,
                        color: AppColors.text,
                      ),
                    ),
                  ),
                  ?trailing,
                ],
              ),
            ),
            Divider(
              color: AppColors.border,
              thickness: AppDimens.borderWidth,
              height: 1,
            ),
          ],
          Padding(padding: padding, child: child),
          if (footer != null) ...<Widget>[
            Divider(
              color: AppColors.border,
              thickness: AppDimens.borderWidth,
              height: 1,
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
              child: footer!,
            ),
          ],
        ],
      ),
    );

    if (onTap == null) return card;
    return Semantics(
      button: true,
      label: semanticLabel,
      child: ClickCursor(
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: onTap,
          child: card,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Campos seleccionables (reemplazo de DropdownButton / TextButton de fecha)
// ---------------------------------------------------------------------------

/// Caja de campo pulsable con la misma silueta que los inputs
/// (`appInputDecoration`): fondo pergamino, borde de tinta de 2 px, radio 0 y
/// sombra dura `Offset(2,2)` que se hunde al presionar. Sirve para selectores
/// y fechas: muestra el valor actual y abre un diálogo propio al tocarla.
class NeobrutalistFieldBox extends StatefulWidget {
  const NeobrutalistFieldBox({
    super.key,
    required this.text,
    this.onTap,
    this.icon,
    this.trailingIcon = Icons.expand_more_rounded,
    this.isPlaceholder = false,
    this.semanticLabel,
  });

  /// Valor visible (o el texto de ayuda si [isPlaceholder]).
  final String text;
  final VoidCallback? onTap;
  final IconData? icon;
  final IconData trailingIcon;

  /// true = se pinta como hint (`mutedStrong`) en lugar de valor (`text`).
  final bool isPlaceholder;
  final String? semanticLabel;

  @override
  State<NeobrutalistFieldBox> createState() => _NeobrutalistFieldBoxState();
}

class _NeobrutalistFieldBoxState extends State<NeobrutalistFieldBox> {
  bool _pressed = false;
  bool _focused = false;

  bool get _enabled => widget.onTap != null;

  void _setPressed(bool value) {
    if (!_enabled || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final pressed = _pressed && _enabled;
    final foreground = widget.isPlaceholder
        ? AppColors.mutedStrong
        : AppColors.text;
    final box = AnimatedContainer(
      duration: AppMotion.press,
      curve: AppMotion.standard,
      transform: Matrix4.translationValues(
        pressed ? AppShadows.offsetBadge.dx : 0,
        pressed ? AppShadows.offsetBadge.dy : 0,
        0,
      ),
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
      decoration: BoxDecoration(
        color: _enabled ? AppColors.bg : AppColors.surfaceLow,
        border: Border.all(
          color: _enabled ? AppColors.border : AppColors.muted,
          // Foco por teclado: borde doble en vez del halo Material.
          width: _focused ? AppDimens.borderWidthThick : AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: pressed || !_enabled
            ? const <BoxShadow>[]
            : AppShadows.badge,
      ),
      child: Row(
        children: <Widget>[
          if (widget.icon != null) ...<Widget>[
            Icon(widget.icon, size: 18, color: AppColors.text),
            const SizedBox(width: AppDimens.spaceSm),
          ],
          Expanded(
            child: Text(
              widget.text,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 14,
                fontWeight: widget.isPlaceholder
                    ? FontWeight.w600
                    : FontWeight.w800,
                color: foreground,
              ),
            ),
          ),
          const SizedBox(width: AppDimens.spaceSm),
          Icon(widget.trailingIcon, size: 20, color: AppColors.text),
        ],
      ),
    );

    return Semantics(
      button: true,
      enabled: _enabled,
      label: widget.semanticLabel ?? widget.text,
      child: MouseRegion(
        cursor: _enabled
            ? SystemMouseCursors.click
            : SystemMouseCursors.forbidden,
        child: Focus(
          onFocusChange: (bool value) => setState(() => _focused = value),
          onKeyEvent: (_, KeyEvent event) {
            if (!_enabled) return KeyEventResult.ignored;
            final isActivation =
                event is KeyDownEvent &&
                (event.logicalKey == LogicalKeyboardKey.enter ||
                    event.logicalKey == LogicalKeyboardKey.space);
            if (isActivation) {
              widget.onTap?.call();
              return KeyEventResult.handled;
            }
            return KeyEventResult.ignored;
          },
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTapDown: _enabled ? (_) => _setPressed(true) : null,
            onTapUp: _enabled ? (_) => _setPressed(false) : null,
            onTapCancel: _enabled ? () => _setPressed(false) : null,
            onTap: widget.onTap,
            child: box,
          ),
        ),
      ),
    );
  }
}

/// Opción de [NeobrutalistSelectField]. `value` puede ser null (p. ej.
/// "Sin asignar") porque el diálogo devuelve la opción completa, no el valor.
class NeobrutalistSelectOption<T> {
  const NeobrutalistSelectOption({
    required this.value,
    required this.label,
    this.icon,
  });

  final T value;
  final String label;
  final IconData? icon;
}

/// Selector neobrutalista: una [NeobrutalistFieldBox] que abre un
/// [NeobrutalistDialog] con las opciones como filas de tinta (la elegida en
/// amarillo resaltador). Sustituye al menú flotante de `DropdownButton`.
class NeobrutalistSelectField<T> extends StatelessWidget {
  const NeobrutalistSelectField({
    super.key,
    required this.value,
    required this.options,
    required this.onChanged,
    this.hint = 'Seleccionar',
    this.dialogTitle,
    this.icon,
  });

  final T value;
  final List<NeobrutalistSelectOption<T>> options;

  /// null = deshabilitado.
  final ValueChanged<T>? onChanged;

  /// Texto cuando [value] no coincide con ninguna opción.
  final String hint;

  /// Título del diálogo; por defecto, [hint].
  final String? dialogTitle;
  final IconData? icon;

  NeobrutalistSelectOption<T>? get _current {
    for (final option in options) {
      if (option.value == value) return option;
    }
    return null;
  }

  Future<void> _open(BuildContext context) async {
    final picked = await showNeobrutalistDialog<NeobrutalistSelectOption<T>>(
      context: context,
      dialog: NeobrutalistDialog(
        title: dialogTitle ?? hint,
        cancelLabel: 'Cancelar',
        maxWidth: 420,
        content: Builder(
          // Builder: el pop debe usar la ruta del diálogo de opciones, no la
          // del formulario que lo abrió.
          builder: (dialogContext) => Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[
              for (var i = 0; i < options.length; i++)
                Padding(
                  padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
                  child: _SelectOptionTile(
                    key: Key('select-option-$i'),
                    label: options[i].label,
                    icon: options[i].icon,
                    selected: options[i].value == value,
                    onTap: () => Navigator.of(dialogContext).pop(options[i]),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
    if (picked != null) onChanged?.call(picked.value);
  }

  @override
  Widget build(BuildContext context) {
    final current = _current;
    return NeobrutalistFieldBox(
      text: current?.label ?? hint,
      isPlaceholder: current == null,
      icon: current?.icon ?? icon,
      onTap: onChanged == null ? null : () => _open(context),
    );
  }
}

/// Fila de opción: sticker rectangular con sombra dura; la seleccionada va
/// en amarillo resaltador con check (estado no solo por color).
class _SelectOptionTile extends StatefulWidget {
  const _SelectOptionTile({
    super.key,
    required this.label,
    required this.selected,
    required this.onTap,
    this.icon,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;
  final IconData? icon;

  @override
  State<_SelectOptionTile> createState() => _SelectOptionTileState();
}

class _SelectOptionTileState extends State<_SelectOptionTile> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final sunk = _pressed || widget.selected;
    return Semantics(
      button: true,
      selected: widget.selected,
      label: widget.label,
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: (_) => setState(() => _pressed = true),
          onTapUp: (_) => setState(() => _pressed = false),
          onTapCancel: () => setState(() => _pressed = false),
          onTap: widget.onTap,
          child: AnimatedContainer(
            duration: AppMotion.press,
            curve: AppMotion.standard,
            transform: Matrix4.translationValues(
              sunk ? AppShadows.offsetBadge.dx : 0,
              sunk ? AppShadows.offsetBadge.dy : 0,
              0,
            ),
            padding: const EdgeInsets.all(AppDimens.spaceMd),
            decoration: BoxDecoration(
              color: widget.selected
                  ? AppColors.accentYellow
                  : AppColors.surface,
              border: Border.all(
                color: AppColors.outline,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: sunk ? const <BoxShadow>[] : AppShadows.badge,
            ),
            child: Row(
              children: <Widget>[
                Icon(
                  widget.icon ??
                      (widget.selected
                          ? Icons.radio_button_checked_rounded
                          : Icons.radio_button_unchecked_rounded),
                  size: 18,
                  color: AppColors.text,
                ),
                const SizedBox(width: AppDimens.spaceSm),
                Expanded(
                  child: Text(
                    widget.label,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w800,
                      color: AppColors.text,
                    ),
                  ),
                ),
                if (widget.selected)
                  Icon(
                    Icons.check_rounded,
                    size: 18,
                    color: AppColors.text,
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// `showDatePicker` con la paleta oficial (tinta/amarillo, radio 0) en vez
/// del esquema Material morado por defecto (el tema global no fija
/// `colorScheme`, así que Material cae en su semilla violeta).
Future<DateTime?> showNeobrutalistDatePicker({
  required BuildContext context,
  required DateTime initialDate,
  required DateTime firstDate,
  required DateTime lastDate,
}) {
  return showDatePicker(
    context: context,
    initialDate: initialDate,
    firstDate: firstDate,
    lastDate: lastDate,
    barrierColor: AppColors.scrim,
    builder: (context, child) {
      final base = Theme.of(context);
      return Theme(
        data: base.copyWith(
          colorScheme: base.colorScheme.copyWith(
            primary: AppColors.border,
            onPrimary: AppColors.surface,
            secondary: AppColors.accentYellow,
            onSecondary: AppColors.text,
            surface: AppColors.surface,
            onSurface: AppColors.text,
            surfaceTint: Colors.transparent,
          ),
          datePickerTheme: DatePickerThemeData(
            backgroundColor: AppColors.surface,
            surfaceTintColor: Colors.transparent,
            elevation: 0,
            shape: RoundedRectangleBorder(
              side: BorderSide(
                color: AppColors.outline,
                width: AppDimens.borderWidthThick,
              ),
            ),
            headerBackgroundColor: AppColors.accentYellow,
            headerForegroundColor: AppColors.text,
            dayShape: WidgetStatePropertyAll<OutlinedBorder>(
              RoundedRectangleBorder(),
            ),
            todayBorder: BorderSide(
              color: AppColors.outline,
              width: AppDimens.borderWidth,
            ),
          ),
          textButtonTheme: TextButtonThemeData(
            style: TextButton.styleFrom(
              foregroundColor: AppColors.text,
              textStyle: const TextStyle(
                fontWeight: FontWeight.w900,
                letterSpacing: 0.5,
              ),
              shape: const RoundedRectangleBorder(),
            ),
          ),
        ),
        child: child!,
      );
    },
  );
}
