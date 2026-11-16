import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/auth/email_verification_screen.dart';
import 'package:taller_integracion_front/features/auth/login_screen.dart';

import 'support/service_uri.dart';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json; charset=utf-8'},
);

http.Response _error(String code, String message, int status) => _json({
  'error': {'code': code, 'message': message},
}, status);

/// Backend simulado: registra cada petición y responde según la ruta.
class _Backend {
  _Backend(this.routes);

  final Map<String, FutureOr<http.Response> Function(http.Request)> routes;
  final requests = <http.Request>[];

  late final service = AuthService(
    client: MockClient((request) async {
      requests.add(request);
      final handler = routes[request.url.servicePath];
      if (handler == null) {
        throw StateError('ruta inesperada ${request.url.servicePath}');
      }
      return await handler(request);
    }),
  );

  List<String> get paths => [for (final r in requests) r.url.servicePath];
}

Future<void> _pumpLogin(WidgetTester tester, _Backend backend) async {
  SharedPreferences.setMockInitialValues({});
  await tester.binding.setSurfaceSize(const Size(1280, 1600));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  addTearDown(backend.service.dispose);
  await tester.pumpWidget(
    MaterialApp(home: LoginScreen(authService: backend.service)),
  );
  await tester.pumpAndSettle();
}

Future<void> _login(WidgetTester tester, String email) async {
  final fields = find.byType(TextFormField);
  await tester.enterText(fields.at(0), email);
  await tester.enterText(fields.at(1), 'Pass1234');
  await tester.tap(find.text('INGRESAR A LA PAGINA'));
  await tester.pumpAndSettle();
}

String? _prefilledEmail(WidgetTester tester) => tester
    .widget<TextFormField>(
      find.byKey(const ValueKey('email-verification-email')),
    )
    .controller
    ?.text;

Future<void> _dispose(WidgetTester tester) async {
  await tester.pumpWidget(const SizedBox());
}

final _notVerified = _error(
  'email_not_verified',
  'Confirma tu correo para iniciar sesión.',
  403,
);

void main() {
  testWidgets('login 403 email_not_verified: diálogo y «Tengo un código»', (
    tester,
  ) async {
    final backend = _Backend({'/auth/login': (_) => _notVerified});
    await _pumpLogin(tester, backend);
    await _login(tester, 'nuevo@alu.uct.cl');

    expect(find.byType(NeobrutalistDialog), findsOneWidget);
    expect(find.text('REENVIAR CORREO'), findsOneWidget);
    expect(find.text('TENGO UN CÓDIGO'), findsOneWidget);
    expect(find.text('Credenciales inválidas.'), findsNothing);

    await tester.tap(find.text('TENGO UN CÓDIGO'));
    await tester.pumpAndSettle();

    expect(find.byType(EmailVerificationScreen), findsOneWidget);
    expect(_prefilledEmail(tester), 'nuevo@alu.uct.cl');
    // Sin reenvío automático: solo se llamó al login.
    expect(backend.paths, ['/auth/login']);

    await tester.tap(find.text('Volver al inicio de sesión'));
    await tester.pumpAndSettle();
    // El botón de ingreso no quedó bloqueado en «INGRESANDO...».
    expect(find.text('INGRESAR A LA PAGINA'), findsOneWidget);
  });

  testWidgets(
    'login 403: «Reenviar correo» llama al endpoint y abre la pantalla',
    (tester) async {
      final backend = _Backend({
        '/auth/login': (_) => _notVerified,
        '/auth/send-verification-email': (_) =>
            _json({'message': 'genérico'}, 200),
      });
      await _pumpLogin(tester, backend);
      await _login(tester, 'Nuevo@alu.uct.cl');

      await tester.tap(find.text('REENVIAR CORREO'));
      await tester.pumpAndSettle();

      expect(backend.paths, ['/auth/login', '/auth/send-verification-email']);
      expect(jsonDecode(backend.requests.last.body), {
        'email': 'nuevo@alu.uct.cl',
      });
      expect(find.byType(EmailVerificationScreen), findsOneWidget);
      expect(find.text(emailVerificationResentMessage), findsOneWidget);
      expect(find.textContaining('REENVIAR CORREO EN'), findsOneWidget);
      await _dispose(tester);
    },
  );

  testWidgets('login 403: reenvío fallido avisa y no abre la pantalla', (
    tester,
  ) async {
    final backend = _Backend({
      '/auth/login': (_) => _notVerified,
      '/auth/send-verification-email': (_) => _error(
        'mail_unavailable',
        'El envío de correos no está disponible temporalmente. Inténtalo más tarde.',
        503,
      ),
    });
    await _pumpLogin(tester, backend);
    await _login(tester, 'nuevo@alu.uct.cl');
    await tester.tap(find.text('REENVIAR CORREO'));
    await tester.pumpAndSettle();

    expect(find.byType(EmailVerificationScreen), findsNothing);
    expect(find.textContaining('no está disponible'), findsOneWidget);
    expect(find.text(emailVerificationResentMessage), findsNothing);
    expect(find.byType(NeobrutalistDialog), findsOneWidget); // sigue abierto
  });

  testWidgets('login 403: «Cancelar» cierra el diálogo sin más', (
    tester,
  ) async {
    final backend = _Backend({'/auth/login': (_) => _notVerified});
    await _pumpLogin(tester, backend);
    await _login(tester, 'nuevo@alu.uct.cl');
    await tester.tap(find.text('CANCELAR'));
    await tester.pumpAndSettle();

    expect(find.byType(NeobrutalistDialog), findsNothing);
    expect(find.byType(EmailVerificationScreen), findsNothing);
    expect(backend.paths, ['/auth/login']);
  });

  testWidgets(
    'login 401 credenciales incorrectas: aviso de siempre, sin diálogo',
    (tester) async {
      final backend = _Backend({
        '/auth/login': (_) =>
            _error('invalid_credentials', 'Credenciales inválidas', 401),
      });
      await _pumpLogin(tester, backend);
      await _login(tester, 'alguien@alu.uct.cl');

      expect(find.text('Credenciales inválidas.'), findsOneWidget);
      expect(find.byType(NeobrutalistDialog), findsNothing);
      expect(find.byType(EmailVerificationScreen), findsNothing);
    },
  );

  testWidgets('login sin conexión: aviso de siempre, sin diálogo', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    final service = AuthService(
      client: MockClient((_) async => throw http.ClientException('offline')),
    );
    addTearDown(service.dispose);
    await tester.binding.setSurfaceSize(const Size(1280, 1600));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    await tester.pumpWidget(
      MaterialApp(home: LoginScreen(authService: service)),
    );
    await tester.pumpAndSettle();
    await _login(tester, 'alguien@alu.uct.cl');

    expect(find.textContaining('No se pudo conectar'), findsOneWidget);
    expect(find.byType(NeobrutalistDialog), findsNothing);
  });

  testWidgets('registro exitoso abre la confirmación con el correo precargado', (
    tester,
  ) async {
    final backend = _Backend({
      '/auth/register': (_) => _json({
        'id': 'u-1',
        'email': 'registro@alu.uct.cl',
        'created_at': '2026-11-16T00:00:00Z',
      }, 201),
    });
    await _pumpLogin(tester, backend);
    await tester.tap(find.text('CREAR CUENTA'));
    await tester.pumpAndSettle();
    final fields = find.byType(TextFormField);
    await tester.enterText(fields.at(0), 'Ana Pérez');
    await tester.enterText(fields.at(1), 'registro@alu.uct.cl');
    await tester.enterText(fields.at(2), 'Universidad Católica de Temuco');
    await tester.enterText(fields.at(3), 'Pass1234');
    await tester.tap(find.text('CREAR MI CUENTA'));
    await tester.pumpAndSettle();

    expect(backend.paths, ['/auth/register']); // no inicia sesión
    expect(find.text('CUENTA CREADA - CONFIRMA TU CORREO'), findsOneWidget);
    expect(find.byType(EmailVerificationScreen), findsOneWidget);
    expect(_prefilledEmail(tester), 'registro@alu.uct.cl');
    expect(
      find.text(
        'Tu cuenta fue creada. Te enviamos un código para confirmar tu correo.',
      ),
      findsOneWidget,
    );
    expect(find.text('REENVIAR CORREO EN 60S'), findsOneWidget);

    // Se puede volver al login, con el correo listo para iniciar sesión.
    await tester.tap(find.text('Volver al inicio de sesión'));
    await tester.pumpAndSettle();
    expect(find.text('INGRESAR A LA PAGINA'), findsOneWidget);
    expect(
      tester
          .widget<TextFormField>(find.byType(TextFormField).at(0))
          .controller
          ?.text,
      'registro@alu.uct.cl',
    );
    expect(find.text('CREANDO...'), findsNothing);
  });

  testWidgets('enlace fijo: confirmar el código vuelve al login con aviso', (
    tester,
  ) async {
    final backend = _Backend({
      '/auth/verify-email': (_) => _json({'status': 'verified'}, 200),
    });
    await _pumpLogin(tester, backend);
    await tester.enterText(find.byType(TextFormField).at(0), 'yo@alu.uct.cl');
    await tester.tap(find.text('¿Tienes un código de confirmación?'));
    await tester.pumpAndSettle();

    expect(find.byType(EmailVerificationScreen), findsOneWidget);
    expect(_prefilledEmail(tester), 'yo@alu.uct.cl');
    await tester.enterText(
      find.byKey(const ValueKey('email-verification-code')),
      ' q3Jm0bS7m2bVx3Zl8dFq1W9kYd2cN5pT0aRrU4sE6hI ',
    );
    await tester.tap(find.text('CONFIRMAR'));
    await tester.pumpAndSettle();

    expect(find.byType(EmailVerificationScreen), findsNothing);
    expect(find.text('INGRESAR A LA PAGINA'), findsOneWidget);
    expect(
      find.text('Correo confirmado. Ya puedes iniciar sesión.'),
      findsOneWidget,
    );
    // No inicia sesión ni renueva nada: solo se canjeó el código.
    expect(backend.paths, ['/auth/verify-email']);
  });

  group('P-F1: reenvío con el diálogo abierto', () {
    Finder dialogButton(String text) => find.descendant(
      of: find.byType(NeobrutalistDialog),
      matching: find.widgetWithText(NeobrutalistButton, text),
    );
    final resendButton = find.byKey(
      const ValueKey('email-not-verified-resend'),
    );
    bool enabled(WidgetTester tester, Finder finder) =>
        tester.widget<NeobrutalistButton>(finder).onPressed != null;
    int sends(_Backend b) =>
        b.paths.where((p) => p == '/auth/send-verification-email').length;

    testWidgets(
      'espera: diálogo abierto, carga, botones deshabilitados, sin duplicados; 200 → una pantalla',
      (tester) async {
        final pending = Completer<http.Response>();
        final backend = _Backend({
          '/auth/login': (_) => _notVerified,
          '/auth/send-verification-email': (_) => pending.future,
        });
        await _pumpLogin(tester, backend);
        await _login(tester, 'nuevo@alu.uct.cl');

        await tester.tap(resendButton);
        await tester.pump();

        // 1. El diálogo sigue abierto mientras se espera.
        expect(find.byType(NeobrutalistDialog), findsOneWidget);
        expect(find.byType(EmailVerificationScreen), findsNothing);
        // Indicador de carga en «Reenviar correo».
        expect(
          find.descendant(of: resendButton, matching: find.text('ENVIANDO...')),
          findsOneWidget,
        );
        // 2. Todos los botones del diálogo quedan deshabilitados.
        expect(enabled(tester, resendButton), isFalse);
        expect(enabled(tester, dialogButton('CANCELAR')), isFalse);
        expect(enabled(tester, dialogButton('TENGO UN CÓDIGO')), isFalse);
        // 3. Un segundo toque no genera otra petición; tampoco otro login.
        await tester.tap(resendButton, warnIfMissed: false);
        await tester.tap(find.text('TENGO UN CÓDIGO'), warnIfMissed: false);
        await tester.pump();
        expect(sends(backend), 1);
        expect(backend.paths.where((p) => p == '/auth/login').length, 1);

        // 4. 200 → se cierra el diálogo y se abre exactamente una pantalla.
        pending.complete(_json({'message': 'genérico'}, 200));
        await tester.pumpAndSettle();
        expect(find.byType(NeobrutalistDialog), findsNothing);
        expect(find.byType(EmailVerificationScreen), findsOneWidget);
        expect(find.text(emailVerificationResentMessage), findsOneWidget);
        expect(find.text('REENVIAR CORREO EN 60S'), findsOneWidget);
        expect(sends(backend), 1);

        await tester.tap(find.text('Volver al inicio de sesión'));
        await tester.pumpAndSettle();
        expect(find.byType(EmailVerificationScreen), findsNothing);
        expect(find.text('INGRESAR A LA PAGINA'), findsOneWidget);
      },
    );

    testWidgets('503 y luego reintento con 200 (9: se puede volver a intentar)', (
      tester,
    ) async {
      var calls = 0;
      final backend = _Backend({
        '/auth/login': (_) => _notVerified,
        '/auth/send-verification-email': (_) => ++calls == 1
            ? _error(
                'mail_unavailable',
                'El envío de correos no está disponible temporalmente. Inténtalo más tarde.',
                503,
              )
            : _json({'message': 'genérico'}, 200),
      });
      await _pumpLogin(tester, backend);
      await _login(tester, 'nuevo@alu.uct.cl');

      await tester.tap(resendButton);
      await tester.pumpAndSettle();
      // 5. 503: sin pantalla y sin éxito; error dentro del diálogo.
      expect(find.byType(EmailVerificationScreen), findsNothing);
      expect(find.text(emailVerificationResentMessage), findsNothing);
      expect(find.byType(NeobrutalistDialog), findsOneWidget);
      expect(
        find.byKey(const ValueKey('email-not-verified-error')),
        findsOneWidget,
      );
      expect(enabled(tester, resendButton), isTrue);

      await tester.tap(resendButton);
      await tester.pumpAndSettle();
      expect(calls, 2);
      expect(find.byType(EmailVerificationScreen), findsOneWidget);
      await _dispose(tester);
    });

    testWidgets(
      '6. error de conexión: sin pantalla, error visible y reintento',
      (tester) async {
        var offline = true;
        final backend = _Backend({
          '/auth/login': (_) => _notVerified,
          '/auth/send-verification-email': (_) {
            if (offline) throw http.ClientException('offline');
            return _json({'message': 'genérico'}, 200);
          },
        });
        await _pumpLogin(tester, backend);
        await _login(tester, 'nuevo@alu.uct.cl');

        await tester.tap(resendButton);
        await tester.pumpAndSettle();
        expect(find.byType(EmailVerificationScreen), findsNothing);
        expect(find.textContaining('Revisa tu conexión'), findsOneWidget);
        expect(find.text(emailVerificationResentMessage), findsNothing);

        offline = false;
        await tester.tap(resendButton);
        await tester.pumpAndSettle();
        expect(find.byType(EmailVerificationScreen), findsOneWidget);
        await _dispose(tester);
      },
    );

    testWidgets('8. doble toque en «Tengo un código» abre una sola pantalla', (
      tester,
    ) async {
      final backend = _Backend({'/auth/login': (_) => _notVerified});
      await _pumpLogin(tester, backend);
      await _login(tester, 'nuevo@alu.uct.cl');

      await tester.tap(find.text('TENGO UN CÓDIGO'));
      await tester.tap(find.text('TENGO UN CÓDIGO'), warnIfMissed: false);
      await tester.pumpAndSettle();

      expect(find.byType(EmailVerificationScreen), findsOneWidget);
      await tester.tap(find.text('Volver al inicio de sesión'));
      await tester.pumpAndSettle();
      // El login sigue en la pila (el segundo toque no hizo un pop extra).
      expect(find.text('INGRESAR A LA PAGINA'), findsOneWidget);
      expect(find.byType(EmailVerificationScreen), findsNothing);
    });
  });

  group('P-F2: sin aperturas duplicadas', () {
    testWidgets('7. doble toque en el enlace fijo abre una sola pantalla', (
      tester,
    ) async {
      final backend = _Backend({});
      await _pumpLogin(tester, backend);
      final link = find.text('¿Tienes un código de confirmación?');
      await tester.tap(link);
      await tester.tap(link);
      await tester.pumpAndSettle();

      expect(find.byType(EmailVerificationScreen), findsOneWidget);
      await tester.tap(find.text('Volver al inicio de sesión'));
      await tester.pumpAndSettle();
      expect(find.byType(EmailVerificationScreen), findsNothing);

      // La protección se libera al volver: se puede abrir de nuevo.
      await tester.tap(link);
      await tester.pumpAndSettle();
      expect(find.byType(EmailVerificationScreen), findsOneWidget);
    });

    testWidgets('el diálogo se libera al cancelar y puede volver a mostrarse', (
      tester,
    ) async {
      final backend = _Backend({'/auth/login': (_) => _notVerified});
      await _pumpLogin(tester, backend);
      await _login(tester, 'nuevo@alu.uct.cl');
      await tester.tap(find.text('CANCELAR'));
      await tester.pumpAndSettle();
      expect(find.byType(NeobrutalistDialog), findsNothing);

      await tester.tap(find.text('INGRESAR A LA PAGINA'));
      await tester.pumpAndSettle();
      expect(find.byType(NeobrutalistDialog), findsOneWidget);
    });
  });
}
