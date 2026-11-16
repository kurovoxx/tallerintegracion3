import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/common_widgets.dart';
import '../../core/services/auth_service.dart';
import '../../core/theme/app_theme.dart';
import 'auth_form_layout.dart';

class PasswordRecoveryScreen extends StatefulWidget {
  const PasswordRecoveryScreen({
    super.key,
    this.initialEmail = '',
    this.authService,
  });

  final String initialEmail;
  final AuthService? authService;

  @override
  State<PasswordRecoveryScreen> createState() => _PasswordRecoveryScreenState();
}

class _PasswordRecoveryScreenState extends State<PasswordRecoveryScreen> {
  final _form = GlobalKey<FormState>();
  late final _email = TextEditingController(text: widget.initialEmail);
  final _code = TextEditingController();
  final _password = TextEditingController();
  final _confirmation = TextEditingController();
  late final _auth = widget.authService ?? AuthService();
  bool _codeSent = false;
  bool _busy = false;
  bool _obscure = true;
  String? _message;
  bool _error = false;
  int _seconds = 0;
  Timer? _timer;

  @override
  void dispose() {
    _timer?.cancel();
    for (final controller in [_email, _code, _password, _confirmation]) {
      controller.dispose();
    }
    if (widget.authService == null) _auth.dispose();
    super.dispose();
  }

  Future<void> _sendCode({bool resend = false}) async {
    if (_busy || (resend && _seconds > 0)) return;
    if (!resend && !_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _message = null;
    });
    final result = await _auth.forgotPassword(_email.text);
    if (!mounted) return;
    setState(() {
      _busy = false;
      _error = !result.success;
      _message = result.success
          ? 'Si el correo está registrado, recibirás un código válido por 15 minutos. Revisa también spam.'
          : result.message;
      if (result.success) {
        _codeSent = true;
        _seconds = 60;
        _code.clear();
      }
    });
    if (result.success) {
      _timer?.cancel();
      _timer = Timer.periodic(const Duration(seconds: 1), (timer) {
        if (!mounted) {
          timer.cancel();
          return;
        }
        setState(() => _seconds--);
        if (_seconds == 0) timer.cancel();
      });
    }
  }

  Future<void> _resetPassword() async {
    if (_busy || !_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _message = null;
    });
    final result = await _auth.resetPassword(
      email: _email.text,
      code: _code.text,
      password: _password.text,
    );
    if (!mounted) return;
    if (result.success) {
      ScaffoldMessenger.of(context).showSnackBar(
        authSuccessSnackBar(
          'Contraseña actualizada. Ya puedes iniciar sesión.',
        ),
      );
      Navigator.of(context).pop();
      return;
    }
    setState(() {
      _busy = false;
      _error = true;
      _message = result.message;
    });
  }

  @override
  Widget build(BuildContext context) {
    return AuthFormScaffold(
      title: 'Recuperar contraseña',
      child: Form(
        key: _form,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            AuthFormHeading(
              title: _codeSent
                  ? 'Crea una nueva contraseña'
                  : 'Recupera tu cuenta',
              subtitle:
                  'Te enviaremos un código desde sigmaacademy.noreply@gmail.com.',
            ),
            const SizedBox(height: AppDimens.spaceXl),
            const AppFieldLabel('CORREO INSTITUCIONAL'),
            const SizedBox(height: 6),
            TextFormField(
              controller: _email,
              readOnly: _codeSent || _busy,
              keyboardType: TextInputType.emailAddress,
              autofillHints: const [AutofillHints.email],
              style: authFieldTextStyle,
              decoration: appInputDecoration('ejemplo@alu.uct.cl').copyWith(
                fillColor: _codeSent ? AppColors.surfaceLow : AppColors.bg,
              ),
              validator: (value) =>
                  RegExp(
                    r'^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$',
                  ).hasMatch(value?.trim() ?? '')
                  ? null
                  : 'Ingresa un correo válido',
            ),
            if (_codeSent) ...[
              const SizedBox(height: 18),
              const AppFieldLabel('CÓDIGO DE 8 DÍGITOS'),
              const SizedBox(height: 6),
              TextFormField(
                controller: _code,
                enabled: !_busy,
                keyboardType: TextInputType.number,
                autofillHints: const [AutofillHints.oneTimeCode],
                inputFormatters: [
                  FilteringTextInputFormatter.digitsOnly,
                  LengthLimitingTextInputFormatter(8),
                ],
                style: authFieldTextStyle,
                decoration: appInputDecoration('00000000'),
                validator: (value) => RegExp(r'^\d{8}$').hasMatch(value ?? '')
                    ? null
                    : 'Ingresa los 8 dígitos',
              ),
              const SizedBox(height: 18),
              const AppFieldLabel('NUEVA CONTRASEÑA'),
              const SizedBox(height: 6),
              TextFormField(
                controller: _password,
                enabled: !_busy,
                obscureText: _obscure,
                autocorrect: false,
                enableSuggestions: false,
                autofillHints: const [AutofillHints.newPassword],
                style: authFieldTextStyle,
                decoration: appInputDecoration('••••••••').copyWith(
                  helperText:
                      'Mínimo 8 caracteres, una letra y un número, sin espacios.',
                  helperMaxLines: 2,
                  helperStyle: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
                    color: AppColors.mutedStrong,
                  ),
                  suffixIcon: IconButton(
                    tooltip: _obscure
                        ? 'Mostrar contraseña'
                        : 'Ocultar contraseña',
                    icon: Icon(
                      _obscure
                          ? Icons.visibility_outlined
                          : Icons.visibility_off_outlined,
                      color: AppColors.muted,
                      size: 20,
                    ),
                    onPressed: () => setState(() => _obscure = !_obscure),
                  ),
                ),
                validator: (value) {
                  final password = value ?? '';
                  if (password.runes.length < 8 ||
                      utf8.encode(password).length > 72 ||
                      !RegExp(r'\p{L}', unicode: true).hasMatch(password) ||
                      !RegExp(r'\p{Nd}', unicode: true).hasMatch(password) ||
                      RegExp(r'\s').hasMatch(password)) {
                    return 'Usa 8 o más caracteres, letras y números, sin espacios (máx. 72 bytes).';
                  }
                  return null;
                },
              ),
              const SizedBox(height: 18),
              const AppFieldLabel('CONFIRMAR CONTRASEÑA'),
              const SizedBox(height: 6),
              TextFormField(
                controller: _confirmation,
                enabled: !_busy,
                obscureText: _obscure,
                autocorrect: false,
                enableSuggestions: false,
                style: authFieldTextStyle,
                decoration: appInputDecoration('Repite la contraseña'),
                validator: (value) => value == _password.text
                    ? null
                    : 'Las contraseñas no coinciden',
                onFieldSubmitted: (_) => _resetPassword(),
              ),
            ],
            if (_message != null) ...[
              const SizedBox(height: AppDimens.spaceLg),
              AuthStatusMessage(
                message: _message!,
                kind: _error ? AuthStatusKind.error : AuthStatusKind.success,
              ),
            ],
            const SizedBox(height: AppDimens.spaceXl),
            AuthPrimaryButton(
              label: _busy
                  ? 'PROCESANDO...'
                  : (_codeSent ? 'GUARDAR CONTRASEÑA' : 'ENVIAR CÓDIGO'),
              onPressed: _busy
                  ? null
                  : (_codeSent ? _resetPassword : () => _sendCode()),
            ),
            if (_codeSent) ...[
              const SizedBox(height: AppDimens.spaceMd),
              AuthSecondaryButton(
                label: _seconds > 0
                    ? 'Reenviar código en ${_seconds}s'
                    : 'Reenviar código',
                onPressed: _busy || _seconds > 0
                    ? null
                    : () => _sendCode(resend: true),
              ),
              const SizedBox(height: AppDimens.spaceSm),
              AuthLink(
                text: 'Usar otro correo',
                onPressed: _busy
                    ? null
                    : () {
                        _timer?.cancel();
                        setState(() {
                          _codeSent = false;
                          _message = null;
                          _seconds = 0;
                          _code.clear();
                          _password.clear();
                          _confirmation.clear();
                        });
                      },
              ),
            ],
          ],
        ),
      ),
    );
  }
}
