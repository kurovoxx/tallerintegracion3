import 'package:flutter/material.dart';

import '../../core/services/auth_service.dart';
import 'email_verification_screen.dart';

/// Resultado del diálogo: el reenvío ya fue aceptado por el backend o la
/// persona tiene un código. `null` = cancelado.
enum EmailNotVerifiedAction { resent, haveCode }

/// Diálogo del login cuando el backend responde 403 `email_not_verified`.
///
/// «Reenviar correo» ejecuta [onResend] con el diálogo abierto: muestra un
/// indicador, deshabilita todos los botones (sin solicitudes duplicadas) y
/// solo se cierra si el backend acepta (200). Ante un error (503, red) lo
/// muestra dentro del diálogo, sin afirmar que el correo salió, y permite
/// reintentar.
Future<EmailNotVerifiedAction?> showEmailNotVerifiedDialog(
  BuildContext context, {
  required Future<LoginResult> Function() onResend,
}) {
  return showDialog<EmailNotVerifiedAction>(
    context: context,
    barrierDismissible: false,
    builder: (_) => _EmailNotVerifiedDialog(onResend: onResend),
  );
}

class _EmailNotVerifiedDialog extends StatefulWidget {
  const _EmailNotVerifiedDialog({required this.onResend});

  final Future<LoginResult> Function() onResend;

  @override
  State<_EmailNotVerifiedDialog> createState() =>
      _EmailNotVerifiedDialogState();
}

class _EmailNotVerifiedDialogState extends State<_EmailNotVerifiedDialog> {
  bool _sending = false;
  bool _closed = false;
  String? _error;

  /// Cierra una sola vez aunque lleguen dos toques en el mismo frame.
  void _close([EmailNotVerifiedAction? action]) {
    if (_closed || _sending) return;
    _closed = true;
    Navigator.of(context).pop(action);
  }

  Future<void> _resend() async {
    if (_closed || _sending) return;
    setState(() {
      _sending = true;
      _error = null;
    });
    final result = await widget.onResend();
    if (!mounted) return;
    setState(() => _sending = false);
    if (result.success) {
      _close(EmailNotVerifiedAction.resent);
      return;
    }
    setState(
      () => _error =
          result.message ??
          'No se pudo solicitar el correo. Inténtalo más tarde.',
    );
  }

  @override
  Widget build(BuildContext context) {
    return PopScope(
      canPop: !_sending,
      child: AlertDialog(
        title: const Text('Confirma tu correo'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Tu cuenta todavía no está confirmada. Revisa tu correo y usa el '
              'código que te enviamos, o pide uno nuevo.',
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(
                _error!,
                key: const ValueKey('email-not-verified-error'),
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ],
          ],
        ),
        actions: [
          TextButton(
            onPressed: _sending ? null : () => _close(),
            child: const Text('Cancelar'),
          ),
          TextButton(
            key: const ValueKey('email-not-verified-resend'),
            onPressed: _sending ? null : _resend,
            child: _sending
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Text('Reenviar correo'),
          ),
          FilledButton(
            onPressed: _sending
                ? null
                : () => _close(EmailNotVerifiedAction.haveCode),
            child: const Text('Tengo un código'),
          ),
        ],
      ),
    );
  }
}

/// Abre la pantalla de confirmación con el correo precargado.
Future<void> openEmailVerification(
  BuildContext context, {
  required String email,
  AuthService? authService,
  int initialCooldownSeconds = 0,
  String? initialMessage,
}) {
  return Navigator.of(context).push<void>(
    MaterialPageRoute<void>(
      builder: (_) => EmailVerificationScreen(
        initialEmail: email,
        authService: authService,
        initialCooldownSeconds: initialCooldownSeconds,
        initialMessage: initialMessage,
      ),
    ),
  );
}
