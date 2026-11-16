import 'dart:async';

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/services/auth_service.dart';
import '../../core/theme/app_theme.dart';

/// Mensaje genérico del reenvío: el backend responde igual exista o no la
/// cuenta, así que la app tampoco puede afirmar que el correo salió.
const emailVerificationResentMessage =
    'Si el correo está registrado y aún no fue confirmado, recibirás un nuevo '
    'código en unos minutos. Revisa también la carpeta de spam.';

/// Segundos entre reenvíos (coincide con el enfriamiento del backend).
const emailVerificationResendCooldown = 60;

/// Confirmación de cuenta con el código recibido por correo
/// (POST /auth/verify-email) y reenvío (POST /auth/send-verification-email).
///
/// Al confirmar (`verified` o `already_verified`) vuelve a la pantalla
/// anterior (el login) avisando que ya se puede iniciar sesión. No inicia
/// sesión ni toca la sesión guardada.
class EmailVerificationScreen extends StatefulWidget {
  const EmailVerificationScreen({
    super.key,
    this.initialEmail = '',
    this.authService,
    this.initialCooldownSeconds = 0,
    this.initialMessage,
  });

  final String initialEmail;
  final AuthService? authService;

  /// Cuenta atrás inicial del reenvío: tras el registro o un reenvío recién
  /// hecho, un reenvío inmediato sería descartado en silencio por el backend.
  final int initialCooldownSeconds;

  /// Aviso informativo inicial (p. ej. «cuenta creada»).
  final String? initialMessage;

  @override
  State<EmailVerificationScreen> createState() =>
      _EmailVerificationScreenState();
}

class _EmailVerificationScreenState extends State<EmailVerificationScreen> {
  late final _email = TextEditingController(text: widget.initialEmail);
  final _code = TextEditingController();
  late final _auth = widget.authService ?? AuthService();
  bool _verifying = false;
  bool _resending = false;
  late String? _message = widget.initialMessage;
  bool _error = false;
  int _seconds = 0;
  Timer? _timer;

  bool get _busy => _verifying || _resending;

  static final _emailPattern = RegExp(
    r'^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$',
  );

  @override
  void initState() {
    super.initState();
    if (widget.initialCooldownSeconds > 0) {
      _startCooldown(widget.initialCooldownSeconds);
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    _email.dispose();
    _code.dispose();
    if (widget.authService == null) _auth.dispose();
    super.dispose();
  }

  void _startCooldown(int seconds) {
    _timer?.cancel();
    _seconds = seconds;
    _timer = Timer.periodic(const Duration(seconds: 1), (timer) {
      if (!mounted) {
        timer.cancel();
        return;
      }
      setState(() => _seconds--);
      if (_seconds <= 0) timer.cancel();
    });
  }

  Future<void> _verify() async {
    if (_busy) return;
    final code = _code.text.trim();
    if (code.isEmpty) {
      setState(() {
        _error = true;
        _message = 'Pega el código que recibiste por correo.';
      });
      return;
    }
    setState(() {
      _verifying = true;
      _message = null;
    });
    final result = await _auth.verifyEmail(code);
    if (!mounted) return;
    if (result.confirmed) {
      final text = result.outcome == VerifyEmailOutcome.alreadyVerified
          ? 'Tu correo ya estaba confirmado. Ya puedes iniciar sesión.'
          : 'Correo confirmado. Ya puedes iniciar sesión.';
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(text)));
      Navigator.of(context).pop();
      return;
    }
    setState(() {
      _verifying = false;
      _error = true;
      _message = result.outcome == VerifyEmailOutcome.invalid
          ? 'El código es inválido, expiró o fue reemplazado por uno más reciente. Solicita un nuevo correo.'
          : (result.message ??
                'No se pudo confirmar el correo. Inténtalo de nuevo.');
    });
  }

  Future<void> _resend() async {
    if (_busy || _seconds > 0) return;
    if (!_emailPattern.hasMatch(_email.text.trim())) {
      setState(() {
        _error = true;
        _message = 'Ingresa un correo válido para reenviar el código.';
      });
      return;
    }
    setState(() {
      _resending = true;
      _message = null;
    });
    final result = await _auth.sendVerificationEmail(_email.text);
    if (!mounted) return;
    setState(() {
      _resending = false;
      _error = !result.success;
      // Ante un error (p. ej. 503 sin SMTP o sin conexión) nunca se afirma
      // que el correo se haya enviado.
      _message = result.success
          ? emailVerificationResentMessage
          : (result.message ??
                'No se pudo solicitar el correo. Inténtalo más tarde.');
      if (result.success) _startCooldown(emailVerificationResendCooldown);
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Confirmar correo')),
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 440),
            child: Form(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Text(
                    'CONFIRMA TU CORREO',
                    style: TextStyle(fontSize: 24, fontWeight: FontWeight.w900),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Te enviamos un código desde sigmaacademy.noreply@gmail.com. '
                    'Cópialo del correo y pégalo aquí.',
                  ),
                  const SizedBox(height: 24),
                  TextFormField(
                    key: const ValueKey('email-verification-email'),
                    controller: _email,
                    readOnly: _busy,
                    keyboardType: TextInputType.emailAddress,
                    autofillHints: const [AutofillHints.email],
                    decoration: appInputDecoration(
                      'tu@correo.com',
                    ).copyWith(labelText: 'Correo electrónico'),
                  ),
                  const SizedBox(height: 16),
                  TextFormField(
                    key: const ValueKey('email-verification-code'),
                    controller: _code,
                    enabled: !_busy,
                    autocorrect: false,
                    enableSuggestions: false,
                    autofillHints: const [AutofillHints.oneTimeCode],
                    decoration: appInputDecoration(
                      'Pega aquí el código',
                    ).copyWith(labelText: 'Código de confirmación'),
                    onFieldSubmitted: (_) => _verify(),
                  ),
                  if (_message != null) ...[
                    const SizedBox(height: 20),
                    Text(
                      _message!,
                      key: const ValueKey('email-verification-message'),
                      style: TextStyle(
                        color: _error
                            ? Theme.of(context).colorScheme.error
                            : AppColors.text,
                      ),
                    ),
                  ],
                  const SizedBox(height: 24),
                  FilledButton(
                    onPressed: _busy ? null : _verify,
                    child: Text(_verifying ? 'CONFIRMANDO...' : 'CONFIRMAR'),
                  ),
                  TextButton(
                    onPressed: _busy || _seconds > 0 ? null : _resend,
                    child: Text(
                      _resending
                          ? 'Enviando...'
                          : (_seconds > 0
                                ? 'Reenviar correo en ${_seconds}s'
                                : 'Reenviar correo'),
                    ),
                  ),
                  TextButton(
                    onPressed: _busy ? null : () => Navigator.of(context).pop(),
                    child: const Text('Volver al inicio de sesión'),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
