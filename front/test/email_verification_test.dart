import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/features/auth/email_verification_screen.dart';

import 'support/service_uri.dart';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json; charset=utf-8'},
);

http.Response _error(String code, String message, int status) => _json({
  'error': {'code': code, 'message': message},
}, status);

const _token = 'q3Jm0bS7m2bVx3Zl8dFq1W9kYd2cN5pT0aRrU4sE6hI';
const _connectionMessage =
    'No se pudo completar la solicitud. Revisa tu conexión e inténtalo de nuevo.';

/// Pantalla anterior (el login) con un botón que abre la confirmación.
class _Launcher extends StatefulWidget {
  const _Launcher({required this.screen});

  final EmailVerificationScreen screen;

  @override
  State<_Launcher> createState() => _LauncherState();
}

class _LauncherState extends State<_Launcher> {
  Object? popResult = 'sin-volver';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          const Text('PANTALLA DE LOGIN'),
          Text('resultado:$popResult'),
          TextButton(
            onPressed: () async {
              final result = await Navigator.of(
                context,
              ).push<Object?>(MaterialPageRoute(builder: (_) => widget.screen));
              setState(() => popResult = result);
            },
            child: const Text('abrir'),
          ),
        ],
      ),
    );
  }
}

Future<void> _open(
  WidgetTester tester,
  AuthService service, {
  String email = 'user@example.com',
  int cooldown = 0,
  String? message,
}) async {
  await tester.binding.setSurfaceSize(const Size(1000, 1000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    MaterialApp(
      home: _Launcher(
        screen: EmailVerificationScreen(
          initialEmail: email,
          authService: service,
          initialCooldownSeconds: cooldown,
          initialMessage: message,
        ),
      ),
    ),
  );
  await tester.tap(find.text('abrir'));
  await tester.pumpAndSettle();
}

Finder get _codeField => find.byKey(const ValueKey('email-verification-code'));
Finder get _emailField =>
    find.byKey(const ValueKey('email-verification-email'));
Finder get _message => find.byKey(const ValueKey('email-verification-message'));

String _messageText(WidgetTester tester) =>
    tester.widget<Text>(_message).data ?? '';

TextButton _resendButton(WidgetTester tester) => tester.widget<TextButton>(
  find.ancestor(
    of: find.textContaining('Reenviar correo'),
    matching: find.byType(TextButton),
  ),
);

Future<void> _dispose(WidgetTester tester) async {
  // Desmonta la pantalla para cancelar la cuenta atrás.
  await tester.pumpWidget(const SizedBox());
}

void main() {
  group('AuthService.sendVerificationEmail', () {
    test('200 genérico: éxito con correo normalizado', () async {
      late http.Request sent;
      final service = AuthService(
        client: MockClient((request) async {
          sent = request;
          return _json({'message': 'genérico'}, 200);
        }),
      );
      addTearDown(service.dispose);

      final result = await service.sendVerificationEmail(' USER@Example.com ');

      expect(result.success, isTrue);
      expect(sent.method, 'POST');
      expect(sent.url.servicePath, '/auth/send-verification-email');
      expect(jsonDecode(sent.body), {'email': 'user@example.com'});
    });

    test('503 mail_unavailable, 400 y errores de red fallan con mensaje', () async {
      final cases = <String, Future<http.Response> Function()>{
        'El envío de correos no está disponible temporalmente. Inténtalo más tarde.':
            () async => _error(
              'mail_unavailable',
              'El envío de correos no está disponible temporalmente. Inténtalo más tarde.',
              503,
            ),
        'Formato de correo inválido.': () async =>
            _error('invalid_email', 'Formato de email inválido', 400),
        _connectionMessage: () async =>
            http.Response('<html>bad gateway</html>', 502),
        'offline': () async => throw http.ClientException('offline'),
      };
      for (final entry in cases.entries) {
        final service = AuthService(client: MockClient((_) => entry.value()));
        final result = await service.sendVerificationEmail('user@example.com');
        service.dispose();
        expect(result.success, isFalse, reason: entry.key);
        expect(
          result.message,
          entry.key == 'offline' ? _connectionMessage : entry.key,
        );
      }
    });
  });

  group('AuthService.verifyEmail', () {
    test('verified, already_verified e inválido según el contrato', () async {
      final responses = [
        _json({'status': 'verified', 'message': 'ok'}, 200),
        _json({'status': 'already_verified', 'message': 'ok'}, 200),
        _error('invalid_verification_token', 'inválido', 400),
        _error('bad_request', 'Falta el código de confirmación.', 400),
      ];
      final expected = [
        VerifyEmailOutcome.verified,
        VerifyEmailOutcome.alreadyVerified,
        VerifyEmailOutcome.invalid,
        VerifyEmailOutcome.invalid,
      ];
      final bodies = <Object?>[];
      for (var i = 0; i < responses.length; i++) {
        final service = AuthService(
          client: MockClient((request) async {
            expect(request.url.servicePath, '/auth/verify-email');
            bodies.add(jsonDecode(request.body));
            return responses[i];
          }),
        );
        final result = await service.verifyEmail('  $_token\n');
        service.dispose();
        expect(result.outcome, expected[i]);
        expect(result.confirmed, i < 2);
      }
      expect(bodies.first, {'token': _token});
    });

    test('500, respuesta no JSON y red son failed con mensaje', () async {
      final cases = <Future<http.Response> Function(), String>{
        () async => _error(
          'internal_error',
          'No se pudo confirmar el correo. Inténtalo más tarde.',
          500,
        ): 'No se pudo confirmar el correo. Inténtalo más tarde.',
        () async => http.Response('<html>bad gateway</html>', 502):
            _connectionMessage,
        () async => throw http.ClientException('offline'): _connectionMessage,
      };
      for (final entry in cases.entries) {
        final service = AuthService(client: MockClient((_) => entry.key()));
        final result = await service.verifyEmail(_token);
        service.dispose();
        expect(result.outcome, VerifyEmailOutcome.failed);
        expect(result.confirmed, isFalse);
        expect(result.message, entry.value);
      }
    });
  });

  group('EmailVerificationScreen', () {
    testWidgets('correo precargado; verified vuelve al login con aviso', (
      tester,
    ) async {
      final bodies = <Object?>[];
      final service = AuthService(
        client: MockClient((request) async {
          bodies.add(jsonDecode(request.body));
          return _json({'status': 'verified'}, 200);
        }),
      );
      addTearDown(service.dispose);
      await _open(tester, service, email: 'precargado@example.com');

      expect(
        tester.widget<TextFormField>(_emailField).controller?.text,
        'precargado@example.com',
      );
      await tester.enterText(_codeField, '   $_token  \n');
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pumpAndSettle();

      expect(bodies, [
        {'token': _token},
      ]);
      expect(find.text('PANTALLA DE LOGIN'), findsOneWidget);
      // La ruta no devuelve resultado (pop sin valor); el aviso informa.
      expect(find.text('resultado:null'), findsOneWidget);
      expect(
        find.text('Correo confirmado. Ya puedes iniciar sesión.'),
        findsOneWidget,
      );
      expect(find.byType(EmailVerificationScreen), findsNothing);
    });

    testWidgets('already_verified también vuelve al login', (tester) async {
      final service = AuthService(
        client: MockClient(
          (_) async => _json({'status': 'already_verified'}, 200),
        ),
      );
      addTearDown(service.dispose);
      await _open(tester, service);
      await tester.enterText(_codeField, _token);
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pumpAndSettle();

      expect(find.text('PANTALLA DE LOGIN'), findsOneWidget);
      expect(
        find.text('Tu correo ya estaba confirmado. Ya puedes iniciar sesión.'),
        findsOneWidget,
      );
    });

    testWidgets('código inválido o expirado: error y sigue en la pantalla', (
      tester,
    ) async {
      final service = AuthService(
        client: MockClient(
          (_) async => _error('invalid_verification_token', 'x', 400),
        ),
      );
      addTearDown(service.dispose);
      await _open(tester, service);
      await tester.enterText(_codeField, 'codigo-equivocado');
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pumpAndSettle();

      expect(find.byType(EmailVerificationScreen), findsOneWidget);
      expect(_messageText(tester), contains('inválido, expiró'));
      expect(find.text('CONFIRMAR'), findsOneWidget); // vuelve a habilitarse
      await _dispose(tester);
    });

    testWidgets('errores de servidor y de red se muestran sin salir', (
      tester,
    ) async {
      var fail = true;
      final service = AuthService(
        client: MockClient((_) async {
          if (fail) throw http.ClientException('offline');
          return _error('internal_error', 'Error del servidor X', 500);
        }),
      );
      addTearDown(service.dispose);
      await _open(tester, service);
      await tester.enterText(_codeField, _token);
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pumpAndSettle();
      expect(_messageText(tester), _connectionMessage);

      fail = false;
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pumpAndSettle();
      expect(_messageText(tester), 'Error del servidor X');
      expect(find.byType(EmailVerificationScreen), findsOneWidget);
      await _dispose(tester);
    });

    testWidgets('código vacío no llama al backend', (tester) async {
      var calls = 0;
      final service = AuthService(
        client: MockClient((_) async {
          calls++;
          return _json({'status': 'verified'}, 200);
        }),
      );
      addTearDown(service.dispose);
      await _open(tester, service);
      await tester.enterText(_codeField, '   ');
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pumpAndSettle();

      expect(calls, 0);
      expect(_messageText(tester), 'Pega el código que recibiste por correo.');
      await _dispose(tester);
    });

    testWidgets('estado de carga deshabilita los botones', (tester) async {
      final pending = Completer<http.Response>();
      final service = AuthService(client: MockClient((_) => pending.future));
      addTearDown(service.dispose);
      await _open(tester, service);
      await tester.enterText(_codeField, _token);
      await tester.tap(find.text('CONFIRMAR'));
      await tester.pump();

      expect(find.text('CONFIRMANDO...'), findsOneWidget);
      final confirm = tester.widget<FilledButton>(find.byType(FilledButton));
      expect(confirm.onPressed, isNull);
      expect(_resendButton(tester).onPressed, isNull);
      expect(tester.widget<TextFormField>(_codeField).enabled, isFalse);

      pending.complete(_error('invalid_verification_token', 'x', 400));
      await tester.pumpAndSettle();
      expect(find.text('CONFIRMAR'), findsOneWidget);
      await _dispose(tester);
    });

    testWidgets('reenvío: mensaje genérico y cuenta atrás de 60 s', (
      tester,
    ) async {
      final bodies = <Object?>[];
      final service = AuthService(
        client: MockClient((request) async {
          expect(request.url.servicePath, '/auth/send-verification-email');
          bodies.add(jsonDecode(request.body));
          return _json({'message': 'genérico del backend'}, 200);
        }),
      );
      addTearDown(service.dispose);
      await _open(tester, service, email: ' User@Example.com ');

      await tester.tap(find.text('Reenviar correo'));
      await tester.pump();
      await tester.pump();

      expect(bodies, [
        {'email': 'user@example.com'},
      ]);
      expect(_messageText(tester), emailVerificationResentMessage);
      expect(_messageText(tester), contains('spam'));
      expect(find.text('Reenviar correo en 60s'), findsOneWidget);
      expect(_resendButton(tester).onPressed, isNull);

      await tester.pump(const Duration(seconds: 1));
      expect(find.text('Reenviar correo en 59s'), findsOneWidget);
      await tester.pump(const Duration(seconds: 59));
      expect(find.text('Reenviar correo'), findsOneWidget);
      expect(_resendButton(tester).onPressed, isNotNull);
      await _dispose(tester);
    });

    testWidgets('reenvío fallido (503 o red): sin afirmar que se envió', (
      tester,
    ) async {
      var offline = false;
      final service = AuthService(
        client: MockClient((_) async {
          if (offline) throw http.ClientException('offline');
          return _error(
            'mail_unavailable',
            'El envío de correos no está disponible temporalmente. Inténtalo más tarde.',
            503,
          );
        }),
      );
      addTearDown(service.dispose);
      await _open(tester, service);

      await tester.tap(find.text('Reenviar correo'));
      await tester.pumpAndSettle();
      expect(_messageText(tester), contains('no está disponible'));
      expect(find.text('Reenviar correo'), findsOneWidget); // sin cuenta atrás

      offline = true;
      await tester.tap(find.text('Reenviar correo'));
      await tester.pumpAndSettle();
      expect(_messageText(tester), _connectionMessage);
      expect(find.textContaining('recibirás'), findsNothing);
      await _dispose(tester);
    });

    testWidgets('correo inválido no llama al reenvío', (tester) async {
      var calls = 0;
      final service = AuthService(
        client: MockClient((_) async {
          calls++;
          return _json({}, 200);
        }),
      );
      addTearDown(service.dispose);
      await _open(tester, service, email: 'no-es-correo');
      await tester.tap(find.text('Reenviar correo'));
      await tester.pumpAndSettle();

      expect(calls, 0);
      expect(_messageText(tester), contains('correo válido'));
      await _dispose(tester);
    });

    testWidgets('cuenta atrás y aviso iniciales (tras registro o reenvío)', (
      tester,
    ) async {
      final service = AuthService(
        client: MockClient((_) async => _json({}, 200)),
      );
      addTearDown(service.dispose);
      await _open(
        tester,
        service,
        cooldown: 60,
        message: 'Tu cuenta fue creada.',
      );

      expect(_messageText(tester), 'Tu cuenta fue creada.');
      expect(find.text('Reenviar correo en 60s'), findsOneWidget);
      expect(_resendButton(tester).onPressed, isNull);
      await _dispose(tester);
    });

    testWidgets('«Volver al inicio de sesión» regresa sin confirmar', (
      tester,
    ) async {
      final service = AuthService(
        client: MockClient((_) async => _json({}, 200)),
      );
      addTearDown(service.dispose);
      await _open(tester, service);
      await tester.tap(find.text('Volver al inicio de sesión'));
      await tester.pumpAndSettle();

      expect(find.text('PANTALLA DE LOGIN'), findsOneWidget);
      expect(find.text('resultado:null'), findsOneWidget);
    });
  });
}
