import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/features/auth/password_recovery_screen.dart';

void main() {
  test(
    'recovery sends normalized email, preserves password and reports failures',
    () async {
      final requests = <http.Request>[];
      final service = AuthService(
        client: MockClient((request) async {
          requests.add(request);
          return request.url.path.endsWith('forgot-password')
              ? http.Response('{"message":"generic"}', 200)
              : http.Response(
                  '{"error":{"code":"invalid_reset_code","message":"Código inválido"}}',
                  400,
                );
        }),
      );
      addTearDown(service.dispose);
      expect(
        (await service.forgotPassword(' USER@example.com ')).success,
        isTrue,
      );
      final result = await service.resetPassword(
        email: ' USER@example.com ',
        code: ' 12345678 ',
        password: 'Secret9!',
      );
      expect(result.success, isFalse);
      expect(result.message, 'Código inválido');
      expect(jsonDecode(requests.first.body), {'email': 'user@example.com'});
      expect(jsonDecode(requests.last.body), {
        'email': 'user@example.com',
        'code': '12345678',
        'password': 'Secret9!',
      });
    },
  );

  test('recovery handles non-JSON and connection errors', () async {
    for (final fails in [true, false]) {
      final service = AuthService(
        client: MockClient((request) async {
          if (fails) throw http.ClientException('offline');
          return http.Response('<html>bad gateway</html>', 502);
        }),
      );
      expect(
        (await service.forgotPassword('user@example.com')).success,
        isFalse,
      );
      service.dispose();
    }
  });

  testWidgets(
    'requests code, validates confirmation, resets and returns to login',
    (tester) async {
      final paths = <String>[];
      final service = AuthService(
        client: MockClient((request) async {
          paths.add(request.url.path);
          return http.Response('{"message":"ok"}', 200);
        }),
      );
      addTearDown(service.dispose);
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (context) => Scaffold(
              body: TextButton(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (_) => PasswordRecoveryScreen(
                      initialEmail: 'user@example.com',
                      authService: service,
                    ),
                  ),
                ),
                child: const Text('Login'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('Login'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('ENVIAR CÓDIGO'));
      await tester.pumpAndSettle();
      expect(paths.single, endsWith('/auth/forgot-password'));
      expect(find.text('Reenviar código en 60s'), findsOneWidget);
      final fields = find.byType(TextFormField);
      await tester.enterText(fields.at(1), '12345678');
      await tester.enterText(fields.at(2), 'NewPassword9');
      await tester.enterText(fields.at(3), 'Different9');
      await tester.ensureVisible(find.text('GUARDAR CONTRASEÑA'));
      await tester.tap(find.text('GUARDAR CONTRASEÑA'));
      await tester.pumpAndSettle();
      expect(find.text('Las contraseñas no coinciden'), findsOneWidget);
      expect(paths.length, 1);
      await tester.enterText(fields.at(3), 'NewPassword9');
      await tester.ensureVisible(find.text('GUARDAR CONTRASEÑA'));
      await tester.tap(find.text('GUARDAR CONTRASEÑA'));
      await tester.pumpAndSettle();
      expect(paths.last, endsWith('/auth/reset-password'));
      expect(find.text('Login'), findsOneWidget);
      expect(
        find.text('Contraseña actualizada. Ya puedes iniciar sesión.'),
        findsOneWidget,
      );
    },
  );

  testWidgets('failed request stays on email step with an error', (
    tester,
  ) async {
    final service = AuthService(
      client: MockClient(
        (_) async => http.Response(
          '{"error":{"code":"mail_unavailable","message":"Servicio no disponible"}}',
          503,
        ),
      ),
    );
    addTearDown(service.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: PasswordRecoveryScreen(
          initialEmail: 'user@example.com',
          authService: service,
        ),
      ),
    );
    await tester.tap(find.text('ENVIAR CÓDIGO'));
    await tester.pumpAndSettle();
    expect(find.text('Servicio no disponible'), findsOneWidget);
    expect(find.byType(TextFormField), findsOneWidget);
  });
}
