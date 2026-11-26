import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Piezas visuales compartidas por la recuperación de contraseña y la
/// confirmación de correo. Solo componen componentes y tokens existentes
/// (`NeobrutalistCard`, `NeobrutalistButton`, `NeobrutalistBadge`, `AppColors`,
/// `AppDimens`) con el mismo lenguaje del Login; no definen un tema propio.

/// Ancho máximo de la tarjeta del formulario.
const double authFormMaxWidth = 480;

/// Texto de los campos: el mismo que usan Login y Registro.
TextStyle authFieldTextStyle = TextStyle(
  fontWeight: FontWeight.w700,
  fontSize: 14,
  color: AppColors.text,
);

/// Pantalla de formulario: barra superior con «volver» y título, lienzo
/// `bg` con scroll y una tarjeta centrada de borde y sombra dura.
class AuthFormScaffold extends StatelessWidget {
  const AuthFormScaffold({super.key, required this.title, required this.child});

  /// Título de la barra superior (se muestra en mayúsculas).
  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Container(
              padding: const EdgeInsets.symmetric(
                horizontal: AppDimens.spaceLg,
                vertical: AppDimens.spaceMd,
              ),
              decoration: BoxDecoration(
                color: AppColors.surface,
                border: Border(
                  bottom: BorderSide(
                    color: AppColors.outline,
                    width: AppDimens.borderWidth,
                  ),
                ),
              ),
              child: Row(
                children: [
                  NeobrutalistIconButton(
                    icon: Icons.arrow_back_rounded,
                    tooltip: 'Volver',
                    onPressed: () => Navigator.of(context).maybePop(),
                  ),
                  const SizedBox(width: AppDimens.spaceMd),
                  Expanded(
                    child: Text(
                      title.toUpperCase(),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w900,
                        letterSpacing: 0.5,
                        color: AppColors.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            Expanded(
              child: SingleChildScrollView(
                physics: const BouncingScrollPhysics(),
                padding: const EdgeInsets.all(AppDimens.spaceXl),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(
                      maxWidth: authFormMaxWidth,
                    ),
                    child: NeobrutalistCard(
                      padding: const EdgeInsets.all(AppDimens.spaceXl),
                      child: child,
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Encabezado de la tarjeta: título en mayúsculas y subtítulo.
class AuthFormHeading extends StatelessWidget {
  const AuthFormHeading({super.key, required this.title, this.subtitle});

  final String title;
  final String? subtitle;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title.toUpperCase(),
          style: TextStyle(
            fontSize: 24,
            fontWeight: FontWeight.w900,
            letterSpacing: -0.5,
            height: 1.1,
            color: AppColors.text,
          ),
        ),
        if (subtitle != null) ...[
          const SizedBox(height: AppDimens.spaceMd),
          Text(
            subtitle!,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w700,
              height: 1.35,
              color: AppColors.mutedStrong,
            ),
          ),
        ],
      ],
    );
  }
}

enum AuthStatusKind { info, success, error }

/// Bloque de estado: etiqueta (`AVISO`, `ENVIADO`, `ERROR`) y mensaje, para
/// que el estado no dependa solo del color.
class AuthStatusMessage extends StatelessWidget {
  const AuthStatusMessage({
    super.key,
    required this.message,
    required this.kind,
    this.messageKey,
  });

  final String message;
  final AuthStatusKind kind;

  /// Clave del `Text` del mensaje (las pruebas leen su contenido).
  final Key? messageKey;

  @override
  Widget build(BuildContext context) {
    final (label, tone, icon) = switch (kind) {
      AuthStatusKind.info => (
        'Aviso',
        NeobrutalistTone.info,
        Icons.info_outline_rounded,
      ),
      AuthStatusKind.success => (
        'Enviado',
        NeobrutalistTone.success,
        Icons.check_rounded,
      ),
      AuthStatusKind.error => (
        'Error',
        NeobrutalistTone.danger,
        Icons.error_outline_rounded,
      ),
    };
    return Container(
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      decoration: BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border.all(
          color: kind == AuthStatusKind.error
              ? AppColors.error
              : AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          NeobrutalistBadge(label: label, tone: tone, icon: icon),
          const SizedBox(height: AppDimens.spaceSm),
          Text(
            message,
            key: messageKey,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w700,
              height: 1.35,
              color: AppColors.text,
            ),
          ),
        ],
      ),
    );
  }
}

/// Acción principal: botón negro a lo ancho con flecha, como el del Login,
/// pero con estado deshabilitado (`onPressed == null`).
class AuthPrimaryButton extends StatelessWidget {
  const AuthPrimaryButton({
    super.key,
    required this.label,
    required this.onPressed,
  });

  final String label;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return NeobrutalistButton(
      label: label,
      onPressed: onPressed,
      expand: true,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceLg,
        vertical: 14,
      ),
      trailing: onPressed == null
          ? null
          : Icon(
              Icons.arrow_forward_rounded,
              size: 16,
              color: AppColors.surface,
            ),
    );
  }
}

/// Acción secundaria a lo ancho (p. ej. reenviar con cuenta atrás).
class AuthSecondaryButton extends StatelessWidget {
  const AuthSecondaryButton({
    super.key,
    required this.label,
    required this.onPressed,
  });

  final String label;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return NeobrutalistButton(
      label: label,
      onPressed: onPressed,
      variant: NeobrutalistButtonVariant.secondary,
      expand: true,
    );
  }
}

/// Enlace con el estilo de «¿Olvidaste tu clave?» del Login.
class AuthLink extends StatelessWidget {
  const AuthLink({super.key, required this.text, required this.onPressed});

  final String text;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null;
    return Semantics(
      button: true,
      enabled: enabled,
      child: MouseRegion(
        cursor: enabled ? SystemMouseCursors.click : SystemMouseCursors.basic,
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: onPressed,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: AppDimens.spaceSm),
            child: Text(
              text,
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w800,
                color: enabled ? AppColors.accentBlue : AppColors.muted,
                decoration: TextDecoration.underline,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// SnackBar de éxito con el estilo del Login.
SnackBar authSuccessSnackBar(String text) {
  return SnackBar(
    backgroundColor: AppColors.border,
    content: Text(
      text,
      style: TextStyle(
        fontWeight: FontWeight.w800,
        color: AppColors.surface,
      ),
    ),
  );
}

/// SnackBar de error con el estilo del Login.
SnackBar authErrorSnackBar(String text) {
  return SnackBar(
    backgroundColor: AppColors.error,
    content: Text(
      text,
      style: TextStyle(
        fontWeight: FontWeight.w700,
        color: AppColors.surface,
      ),
    ),
  );
}
