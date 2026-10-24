import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/account_deletion_service.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/theme/app_theme.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/academic/profile_screen.dart';
import 'package:taller_integracion_front/features/auth/account_deletion_dialog.dart';
import 'package:taller_integracion_front/features/auth/login_screen.dart';

import 'support/service_uri.dart';

// Flujo 3_2_16 desde Mi Perfil contra el contrato real de Identity
// (DELETE /auth/account, GET /auth/account-deletion).

String _jwt(String userId) {
  String enc(Map<String, dynamic> m) =>
      base64Url.encode(utf8.encode(jsonEncode(m))).replaceAll('=', '');
  final exp =
      DateTime.now().add(const Duration(hours: 1)).millisecondsSinceEpoch ~/
      1000;
  return '${enc({'alg': 'HS256'})}.${enc({'user_id': userId, 'exp': exp})}.firma';
}

http.Response _json(int status, Object body) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

const _deleteButton = ValueKey('profile-delete-account');
const _confirmField = ValueKey('account-deletion-confirm-field');
const _submit = ValueKey('account-deletion-submit');
const _retry = ValueKey('account-deletion-retry');

/// Identity falso: cola de respuestas por ruta, registro de llamadas.
class _FakeIdentity {
  final deleteResponses = <FutureOr<http.Response> Function()>[];
  final statusResponses = <FutureOr<http.Response> Function()>[];
  final calls = <String>[];

  int get deletes => calls.where((c) => c == 'DELETE').length;
  int get statusChecks => calls.where((c) => c == 'GET').length;

  late final MockClient client = MockClient((request) async {
    final path = request.url.servicePath;
    if (request.method == 'DELETE' && path == '/auth/account') {
      calls.add('DELETE');
      expect(request.headers['Authorization'], startsWith('Bearer '));
      expect(request.body, isEmpty);
      return deleteResponses.removeAt(0)();
    }
    if (request.method == 'GET' && path == '/auth/account-deletion') {
      calls.add('GET');
      return statusResponses.removeAt(0)();
    }
    throw StateError('ruta inesperada ${request.method} $path');
  });
}

class _Harness {
  _Harness() {
    deletion = AccountDeletionService(
      client: identity.client,
      purgeLocalData: (userId) async => purged.add(userId),
    );
  }

  final identity = _FakeIdentity();
  final purged = <String>[];
  late final AccountDeletionService deletion;
}

ProfileService _profileService() => ProfileService(
  client: MockClient((request) async {
    if (request.url.servicePath.endsWith('/google-drive/status')) {
      return _json(200, {'connected': false});
    }
    return _json(200, {
      'display_name': 'Ana Sintética',
      'email': 'ana@example.test',
      'visibility': 'public',
    });
  }),
);

Future<void> _pumpProfile(
  WidgetTester tester,
  _Harness h, {
  Size size = const Size(1440, 900),
  double textScale = 1,
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  final profile = _profileService();
  addTearDown(profile.dispose);
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light,
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(
          context,
        ).copyWith(textScaler: TextScaler.linear(textScale)),
        child: child!,
      ),
      home: ProfileScreen(service: profile, accountDeletionService: h.deletion),
    ),
  );
  await tester.pumpAndSettle();
}

Future<void> _openDialog(WidgetTester tester) async {
  final button = find.byKey(_deleteButton);
  expect(button, findsOneWidget);
  await tester.ensureVisible(button);
  await tester.pumpAndSettle();
  await tester.tap(button);
  await tester.pumpAndSettle();
  expect(find.byKey(_confirmField), findsOneWidget);
}

Future<void> _confirm(WidgetTester tester) async {
  await tester.enterText(find.byKey(_confirmField), 'eliminar');
  await tester.pump();
  await tester.ensureVisible(find.byKey(_submit));
  await tester.tap(find.byKey(_submit));
}

bool _enabled(WidgetTester tester, Finder target) {
  final button = find.ancestor(
    of: target,
    matching: find.byType(NeobrutalistButton),
    matchRoot: true,
  );
  return tester.widget<NeobrutalistButton>(button.first).onPressed != null;
}

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues({
      SessionManager.rememberedEmailKey: 'ana@example.test',
    });
    AccountDeletionService.resetInFlightForTesting();
    AuthedHttp.refreshOverride = null;
    await SessionManager.saveSession(
      _jwt('user-a'),
      {'id': 'user-a', 'email': 'ana@example.test'},
      rememberMe: true,
      refreshToken: 'refresh-a',
    );
  });

  tearDown(() async {
    AuthedHttp.refreshOverride = null;
    await SessionManager.clear();
  });

  testWidgets('la opción vive al final de Mi Perfil con advertencia', (
    tester,
  ) async {
    final h = _Harness();
    await _pumpProfile(tester, h);
    expect(find.text('CUENTA'), findsOneWidget);
    expect(find.text('ZONA DE PELIGRO'), findsOneWidget);
    // Una sola entrada: no hay botones duplicados en la pantalla.
    expect(find.byKey(_deleteButton), findsOneWidget);
    final semantics = tester.ensureSemantics();
    expect(
      find.bySemanticsLabel(RegExp('Eliminar cuenta de forma permanente')),
      findsOneWidget,
    );
    semantics.dispose();
    // Debajo de Google Drive, como última tarjeta.
    final driveY = tester.getTopLeft(find.text('GOOGLE DRIVE')).dy;
    final deleteY = tester.getTopLeft(find.byKey(_deleteButton)).dy;
    expect(deleteY, greaterThan(driveY));
    await _openDialog(tester);
    expect(find.text('ACCIÓN PERMANENTE'), findsOneWidget);
    expect(
      find.textContaining(
        'Los archivos en tu Google Drive personal no se borran',
      ),
      findsOneWidget,
    );
    expect(h.identity.calls, isEmpty);
  });

  testWidgets('cancelar no envía nada ni toca la sesión', (tester) async {
    final h = _Harness();
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    await tester.tap(find.text('CANCELAR'));
    await tester.pumpAndSettle();
    expect(find.byKey(_confirmField), findsNothing);
    expect(h.identity.calls, isEmpty);
    expect(h.purged, isEmpty);
    expect(SessionManager.currentUserId, 'user-a');
    expect(find.byType(ProfileScreen), findsOneWidget);
  });

  testWidgets('sin escribir la confirmación el botón queda deshabilitado', (
    tester,
  ) async {
    final h = _Harness();
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    expect(_enabled(tester, find.byKey(_submit)), isFalse);
    await tester.tap(find.byKey(_submit), warnIfMissed: false);
    await tester.enterText(find.byKey(_confirmField), 'ELIMINA');
    await tester.pump();
    expect(_enabled(tester, find.byKey(_submit)), isFalse);
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pump();
    expect(h.identity.calls, isEmpty);
    await tester.enterText(find.byKey(_confirmField), ' Eliminar ');
    await tester.pump();
    expect(_enabled(tester, find.byKey(_submit)), isTrue);
    await tester.tap(find.text('CANCELAR'));
    await tester.pumpAndSettle();
  });

  testWidgets(
    '200 complete: una sola petición, limpieza local y vuelta al login',
    (tester) async {
      final h = _Harness();
      final gate = Completer<http.Response>();
      h.identity.deleteResponses.add(() => gate.future);
      await _pumpProfile(tester, h);
      await _openDialog(tester);
      await _confirm(tester);
      await tester.pump();

      // Procesando: acciones bloqueadas, sin cierre por atrás ni doble envío.
      expect(find.text('ELIMINANDO…'), findsOneWidget);
      expect(_enabled(tester, find.byKey(_submit)), isFalse);
      expect(_enabled(tester, find.text('CANCELAR')), isFalse);
      await tester.tap(find.byKey(_submit), warnIfMissed: false);
      await tester.tap(find.text('CANCELAR'), warnIfMissed: false);
      await tester.binding.handlePopRoute();
      await tester.pump();
      expect(find.text('ELIMINANDO…'), findsOneWidget);
      expect(h.identity.deletes, 1);
      // Antes de la confirmación del backend no se limpió nada.
      expect(SessionManager.currentUserId, 'user-a');
      expect(h.purged, isEmpty);

      gate.complete(_json(200, {'status': 'complete'}));
      await tester.pumpAndSettle();

      expect(find.byType(LoginScreen), findsOneWidget);
      expect(find.byType(ProfileScreen), findsNothing);
      expect(find.text('Tu cuenta fue eliminada.'), findsOneWidget);
      expect(h.identity.deletes, 1);
      expect(h.purged, ['user-a']);
      expect(SessionManager.isLoggedIn, isFalse);
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getString('auth_tokens_v1'), isNull);
      expect(prefs.getString(SessionManager.rememberedEmailKey), isNull);
      expect(await SessionManager.hasPersistedSession(), isFalse);
    },
  );

  testWidgets(
    '401 con refresh rechazado: sesión expirada, sin limpieza de datos',
    (tester) async {
      AuthedHttp.refreshOverride = (_) async =>
          RefreshResult.failure('rechazado', kind: RefreshFailureKind.rejected);
      final h = _Harness();
      h.identity.deleteResponses.add(
        () => _json(401, {
          'error': {'code': 'token_expired', 'message': 'expirado'},
        }),
      );
      await _pumpProfile(tester, h);
      await _openDialog(tester);
      await _confirm(tester);
      // Sin manejador global (en la app navega al login), el perfil queda
      // cargando con un indicador indefinido: se avanza tiempo fijo.
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.text('SESIÓN EXPIRADA'), findsOneWidget);
      expect(SessionManager.isLoggedIn, isFalse);
      expect(h.purged, isEmpty);
      expect(h.identity.statusChecks, 0);
      await tester.tap(find.text('IR A INICIAR SESIÓN'));
      await tester.pumpAndSettle();
      expect(find.byType(LoginScreen), findsOneWidget);
      expect(h.purged, isEmpty);
    },
  );

  testWidgets('404 account_not_found limpia este dispositivo', (tester) async {
    final h = _Harness();
    h.identity.deleteResponses.add(
      () => _json(404, {'error': 'account_not_found'}),
    );
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    await _confirm(tester);
    await tester.pumpAndSettle();
    expect(find.byType(LoginScreen), findsOneWidget);
    expect(find.textContaining('ya no existe en el servidor'), findsOneWidget);
    expect(h.purged, ['user-a']);
  });

  for (final status in [403, 409, 500, 503]) {
    testWidgets(
      '$status sin inicio confirmado conserva todo y permite reintentar',
      (tester) async {
        final h = _Harness();
        h.identity.deleteResponses
          ..add(() => _json(status, {'error': 'x'}))
          ..add(() => _json(200, {'status': 'complete'}));
        h.identity.statusResponses.add(
          () => _json(404, {'status': 'not_started'}),
        );
        await _pumpProfile(tester, h);
        await _openDialog(tester);
        await _confirm(tester);
        await tester.pumpAndSettle();

        expect(find.text('NO SE ELIMINÓ LA CUENTA'), findsOneWidget);
        expect(find.textContaining('($status)'), findsOneWidget);
        expect(h.identity.deletes, 1);
        expect(h.identity.statusChecks, 1);
        expect(h.purged, isEmpty);
        expect(SessionManager.currentUserId, 'user-a');

        await tester.tap(find.byKey(_retry));
        await tester.pumpAndSettle();
        expect(find.byType(LoginScreen), findsOneWidget);
        expect(h.identity.deletes, 2);
        expect(h.purged, ['user-a']);
      },
    );
  }

  testWidgets('timeout o red: no repite a ciegas y consulta el estado', (
    tester,
  ) async {
    final h = _Harness();
    h.identity.deleteResponses.add(() => throw http.ClientException('offline'));
    h.identity.statusResponses
      ..add(() => throw http.ClientException('offline'))
      ..add(() => _json(200, {'status': 'pending'}))
      ..add(() => _json(200, {'status': 'complete'}));
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    await _confirm(tester);
    await tester.pumpAndSettle();

    expect(find.text('ESTADO SIN CONFIRMAR'), findsOneWidget);
    expect(find.byKey(_retry), findsNothing);
    expect(find.byKey(_submit), findsNothing);
    expect(h.identity.deletes, 1);
    expect(h.purged, isEmpty);
    expect(SessionManager.currentUserId, 'user-a');

    await tester.tap(find.text('CONSULTAR ESTADO'));
    await tester.pumpAndSettle();
    expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);
    expect(h.purged, isEmpty);

    // La consulta automática detecta que terminó.
    await tester.pump(const Duration(seconds: 4));
    await tester.pumpAndSettle();
    expect(find.byType(LoginScreen), findsOneWidget);
    expect(find.text('Tu cuenta fue eliminada.'), findsOneWidget);
    expect(h.identity.deletes, 1);
    expect(h.identity.statusChecks, 3);
    expect(h.purged, ['user-a']);
  });

  testWidgets('202 pending: no se presenta como éxito hasta confirmarse', (
    tester,
  ) async {
    final h = _Harness();
    h.identity.deleteResponses.add(() => _json(202, {'status': 'pending'}));
    h.identity.statusResponses
      ..add(() => _json(200, {'status': 'pending'}))
      ..add(() => _json(200, {'status': 'complete'}));
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    await _confirm(tester);
    await tester.pumpAndSettle();

    expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);
    expect(find.text('PENDIENTE'), findsOneWidget);
    expect(find.text('Tu cuenta fue eliminada.'), findsNothing);
    expect(h.purged, isEmpty);
    expect(SessionManager.currentUserId, 'user-a');
    // Atrás no lo cierra: las salidas son explícitas.
    await tester.binding.handlePopRoute();
    await tester.pump();
    expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);

    await tester.pump(const Duration(seconds: 4));
    await tester.pumpAndSettle();
    expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);
    expect(h.purged, isEmpty);

    await tester.pump(const Duration(seconds: 4));
    await tester.pumpAndSettle();
    expect(find.byType(LoginScreen), findsOneWidget);
    expect(find.text('Tu cuenta fue eliminada.'), findsOneWidget);
    expect(h.identity.deletes, 1);
    expect(h.purged, ['user-a']);
  });

  testWidgets('202 pending y salir: limpia el dispositivo sin anunciar éxito', (
    tester,
  ) async {
    final h = _Harness();
    h.identity.deleteResponses.add(() => _json(202, {'status': 'pending'}));
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    await _confirm(tester);
    await tester.pumpAndSettle();
    await tester.tap(find.text('SALIR DE ESTE DISPOSITIVO'));
    await tester.pumpAndSettle();
    expect(find.byType(LoginScreen), findsOneWidget);
    expect(
      find.textContaining('sigue en curso en el servidor'),
      findsOneWidget,
    );
    expect(find.text('Tu cuenta fue eliminada.'), findsNothing);
    expect(h.purged, ['user-a']);
    expect(SessionManager.isLoggedIn, isFalse);
  });

  testWidgets('la sesión expira con el borrado pendiente: igual se purga', (
    tester,
  ) async {
    final h = _Harness();
    h.identity.deleteResponses.add(() => _json(202, {'status': 'pending'}));
    await _pumpProfile(tester, h);
    await _openDialog(tester);
    await _confirm(tester);
    await tester.pumpAndSettle();
    expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);
    // Identity rechaza el refresh desde que comienza el borrado.
    await SessionManager.expireSession();
    await tester.pumpAndSettle();
    expect(h.purged, ['user-a']);
    expect(find.byType(LoginScreen), findsOneWidget);
    expect(
      find.textContaining('sigue en curso en el servidor'),
      findsOneWidget,
    );
  });

  testWidgets(
    'otra cuenta toma la sesión con el diálogo abierto: no se borra',
    (tester) async {
      final h = _Harness();
      await _pumpProfile(tester, h);
      await _openDialog(tester);
      await SessionManager.clear();
      await SessionManager.saveSession(_jwt('user-b'), {'id': 'user-b'});
      await tester.pumpAndSettle();
      expect(find.byKey(_confirmField), findsNothing);
      expect(h.identity.calls, isEmpty);
      expect(h.purged, isEmpty);
      expect(SessionManager.currentUserId, 'user-b');
    },
  );

  group('responsive y texto ampliado', () {
    const sizes = [Size(360, 640), Size(768, 1024), Size(1440, 900)];
    const scales = [1.0, 2.0];
    String label(Size size, double scale) =>
        '${size.width.toInt()}x${size.height.toInt()} @${(scale * 100).toInt()}%';

    for (final size in sizes) {
      for (final scale in scales) {
        testWidgets('tarjeta Cuenta de Mi Perfil ${label(size, scale)}', (
          tester,
        ) async {
          final h = _Harness();
          await _pumpProfile(tester, h, size: size, textScale: scale);
          final button = find.byKey(_deleteButton);
          await tester.ensureVisible(button);
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);
          final rect = tester.getRect(button);
          expect(rect.left, greaterThanOrEqualTo(0));
          expect(rect.right, lessThanOrEqualTo(size.width));
          expect(rect.height, greaterThanOrEqualTo(44));
          expect(find.text('ZONA DE PELIGRO'), findsOneWidget);
        });

        testWidgets('diálogo en todas sus fases ${label(size, scale)}', (
          tester,
        ) async {
          tester.view.physicalSize = size;
          tester.view.devicePixelRatio = 1;
          addTearDown(tester.view.resetPhysicalSize);
          addTearDown(tester.view.resetDevicePixelRatio);
          final h = _Harness();
          h.identity.deleteResponses
            ..add(() => _json(503, {'error': 'account_deletion_unavailable'}))
            ..add(() => _json(202, {'status': 'pending'}));
          h.identity.statusResponses
            ..add(() => _json(404, {'status': 'not_started'}))
            ..add(() => _json(503, {'error': 'account_status_unavailable'}));
          AccountDeletionExit? exit;
          await tester.pumpWidget(
            MaterialApp(
              theme: AppTheme.light,
              builder: (context, child) => MediaQuery(
                data: MediaQuery.of(
                  context,
                ).copyWith(textScaler: TextScaler.linear(scale)),
                child: child!,
              ),
              home: Builder(
                builder: (context) => Scaffold(
                  body: Center(
                    child: TextButton(
                      onPressed: () async =>
                          exit = await showAccountDeletionDialog(
                            context,
                            service: h.deletion,
                            email: 'ana@example.test',
                          ),
                      child: const Text('abrir'),
                    ),
                  ),
                ),
              ),
            ),
          );
          await tester.tap(find.text('abrir'));
          await tester.pumpAndSettle();

          void expectFits(List<Finder> actions) {
            expect(tester.takeException(), isNull);
            final dialog = tester.getRect(find.byType(Dialog));
            expect(dialog.left, greaterThanOrEqualTo(0));
            expect(dialog.top, greaterThanOrEqualTo(0));
            expect(dialog.right, lessThanOrEqualTo(size.width));
            expect(dialog.bottom, lessThanOrEqualTo(size.height));
            for (final action in actions) {
              final button = find.ancestor(
                of: action,
                matching: find.byType(NeobrutalistButton),
                matchRoot: true,
              );
              final rect = tester.getRect(button.first);
              expect(dialog.contains(rect.topLeft), isTrue, reason: '$action');
              expect(
                dialog.contains(rect.bottomRight - const Offset(1, 1)),
                isTrue,
                reason: '$action',
              );
              // Área táctil del catálogo (skill: ≥ 44 dp).
              expect(rect.height, greaterThanOrEqualTo(44));
            }
          }

          // Confirmación: advertencia larga con scroll y acciones visibles.
          expectFits([find.text('CANCELAR'), find.byKey(_submit)]);
          await _confirm(tester);
          await tester.pumpAndSettle();

          // 503 + not_started: se puede reintentar.
          expect(find.text('NO SE ELIMINÓ LA CUENTA'), findsOneWidget);
          expectFits([find.text('CANCELAR'), find.byKey(_retry)]);
          await tester.tap(find.byKey(_retry));
          await tester.pumpAndSettle();

          // 202: pendiente; una consulta fallida no lo convierte en error.
          expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);
          expectFits([
            find.text('CONSULTAR ESTADO'),
            find.text('SALIR DE ESTE DISPOSITIVO'),
          ]);
          await tester.tap(find.text('CONSULTAR ESTADO'));
          await tester.pumpAndSettle();
          expect(find.text('ELIMINACIÓN EN CURSO'), findsOneWidget);
          expect(
            find.text('No se pudo consultar el estado ahora.'),
            findsOneWidget,
          );
          expectFits([find.text('SALIR DE ESTE DISPOSITIVO')]);

          await tester.tap(find.text('SALIR DE ESTE DISPOSITIVO'));
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);
          expect(find.byType(Dialog), findsNothing);
          expect(exit, AccountDeletionExit.pendingSignedOut);
          expect(h.identity.deletes, 2);
          expect(h.purged, ['user-a']);
        });
      }
    }
  });
}
