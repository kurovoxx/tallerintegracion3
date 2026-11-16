import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/common_widgets.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/auth/auth_form_layout.dart';
import 'package:taller_integracion_front/features/auth/email_not_verified_dialog.dart';
import 'package:taller_integracion_front/features/auth/email_verification_screen.dart';
import 'package:taller_integracion_front/features/auth/password_recovery_screen.dart';

// Capa visual de recuperación de contraseña y confirmación de correo:
// componentes del catálogo (sin Material por defecto) y sin desbordes en las
// resoluciones de la guía (`.skills/flutter_neobrutalism_ui.md` §6) con el
// texto al 100 % y al 200 %.

const _sizes = <Size>[Size(360, 640), Size(768, 1024), Size(1440, 900)];
const _scales = <double>[1, 2];

AuthService _service(int status, Object body) {
  return AuthService(
    client: MockClient((_) async => http.Response(jsonEncode(body), status)),
  );
}

Future<void> _pump(
  WidgetTester tester,
  Widget home,
  Size size,
  double scale,
) async {
  await tester.binding.setSurfaceSize(size);
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    MaterialApp(
      home: MediaQuery(
        data: MediaQueryData(size: size, textScaler: TextScaler.linear(scale)),
        child: home,
      ),
    ),
  );
  await tester.pump();
}

void _expectCatalogOnly() {
  expect(find.byType(AppBar), findsNothing);
  expect(find.byType(FilledButton), findsNothing);
  expect(find.byType(TextButton), findsNothing);
  expect(find.byType(AlertDialog), findsNothing);
}

Future<void> _tapVisible(WidgetTester tester, Finder finder) async {
  // Sin foco: el campo enfocado no vuelve a desplazar la vista hacia sí
  // mismo después de `ensureVisible` (como al tocar fuera del campo).
  FocusManager.instance.primaryFocus?.unfocus();
  await tester.pumpAndSettle();
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
}

/// Etiqueta de estado presente (aunque quede bajo el pliegue) y alcanzable.
Future<void> _expectStatus(WidgetTester tester, String label) async {
  final any = find.text(label, skipOffstage: false);
  expect(any, findsOneWidget);
  await tester.ensureVisible(any);
  await tester.pumpAndSettle();
  expect(find.text(label), findsOneWidget);
}

Future<void> _dispose(WidgetTester tester) async {
  await tester.pumpWidget(const SizedBox());
}

void main() {
  for (final size in _sizes) {
    for (final scale in _scales) {
      final tag = '${size.width.toInt()}x${size.height.toInt()} @${scale}x';

      testWidgets('recuperación (pasos 1 y 2 con error) sin desbordes $tag', (
        tester,
      ) async {
        var calls = 0;
        final service = AuthService(
          client: MockClient((_) async {
            calls++;
            return calls == 1
                ? http.Response('{"message":"ok"}', 200)
                : http.Response(
                    '{"error":{"code":"invalid_reset_code","message":"El código es inválido, expiró o agotó sus intentos."}}',
                    400,
                  );
          }),
        );
        addTearDown(service.dispose);
        await _pump(
          tester,
          PasswordRecoveryScreen(
            initialEmail: 'usuario@alu.uct.cl',
            authService: service,
          ),
          size,
          scale,
        );
        expect(find.byType(NeobrutalistCard), findsOneWidget);
        expect(find.byType(AppFieldLabel), findsOneWidget);
        _expectCatalogOnly();

        await _tapVisible(tester, find.text('ENVIAR CÓDIGO'));
        await tester.pumpAndSettle();
        expect(calls, 1);
        await _expectStatus(tester, 'ENVIADO');
        expect(find.byType(AppFieldLabel), findsNWidgets(4));
        final fields = find.byType(TextFormField);
        await tester.enterText(fields.at(1), '12345678');
        await tester.enterText(fields.at(2), 'NewPassword9');
        await tester.enterText(fields.at(3), 'NewPassword9');
        await _tapVisible(tester, find.text('GUARDAR CONTRASEÑA'));
        await tester.pumpAndSettle();
        expect(calls, 2);
        await _expectStatus(tester, 'ERROR');
        expect(
          find.textContaining('REENVIAR CÓDIGO EN', skipOffstage: false),
          findsOneWidget,
        );
        _expectCatalogOnly();
        expect(tester.takeException(), isNull);
        await _dispose(tester);
      });

      testWidgets('confirmación (aviso inicial y error) sin desbordes $tag', (
        tester,
      ) async {
        final service = _service(400, {
          'error': {
            'code': 'invalid_verification_token',
            'message': 'inválido',
          },
        });
        addTearDown(service.dispose);
        await _pump(
          tester,
          EmailVerificationScreen(
            initialEmail: 'usuario@alu.uct.cl',
            authService: service,
            initialCooldownSeconds: 60,
            initialMessage:
                'Tu cuenta fue creada. Te enviamos un código para confirmar tu correo.',
          ),
          size,
          scale,
        );
        expect(find.byType(NeobrutalistCard), findsOneWidget);
        expect(find.byType(AppFieldLabel), findsNWidgets(2));
        await _expectStatus(tester, 'AVISO');
        expect(
          find.text('REENVIAR CORREO EN 60S', skipOffstage: false),
          findsOneWidget,
        );
        _expectCatalogOnly();

        await tester.enterText(
          find.byKey(const ValueKey('email-verification-code')),
          'codigo-equivocado',
        );
        await _tapVisible(tester, find.text('CONFIRMAR'));
        await tester.pump();
        await tester.pump();
        await _expectStatus(tester, 'ERROR');
        _expectCatalogOnly();
        expect(tester.takeException(), isNull);
        await _dispose(tester);
      });

      testWidgets('diálogo email_not_verified (con error) sin desbordes $tag', (
        tester,
      ) async {
        await _pump(
          tester,
          Builder(
            builder: (context) => Center(
              child: GestureDetector(
                onTap: () => showEmailNotVerifiedDialog(
                  context,
                  onResend: () async => LoginResult.failure(
                    'El envío de correos no está disponible temporalmente. Inténtalo más tarde.',
                  ),
                ),
                child: const Text('abrir'),
              ),
            ),
          ),
          size,
          scale,
        );
        await tester.tap(find.text('abrir'));
        await tester.pumpAndSettle();
        expect(find.byType(NeobrutalistDialog), findsOneWidget);
        expect(
          find.descendant(
            of: find.byType(NeobrutalistDialog),
            matching: find.byType(NeobrutalistButton),
          ),
          findsNWidgets(3),
        );
        _expectCatalogOnly();

        await tester.tap(
          find.byKey(const ValueKey('email-not-verified-resend')),
        );
        await tester.pumpAndSettle();
        expect(find.text('ERROR'), findsOneWidget);
        expect(tester.takeException(), isNull);
      });
    }
  }

  testWidgets('AuthStatusMessage etiqueta cada estado con texto', (
    tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Column(
          children: [
            AuthStatusMessage(message: 'a', kind: AuthStatusKind.info),
            AuthStatusMessage(message: 'b', kind: AuthStatusKind.success),
            AuthStatusMessage(message: 'c', kind: AuthStatusKind.error),
          ],
        ),
      ),
    );
    for (final label in ['AVISO', 'ENVIADO', 'ERROR']) {
      expect(find.text(label), findsOneWidget);
    }
  });
}
