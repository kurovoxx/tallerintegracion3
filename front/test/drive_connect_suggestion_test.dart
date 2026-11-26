import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database_manager.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/profile_screen.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/drive_connect_suggestion.dart';

const _userA = 'aaaaaaaa-0000-4000-8000-00000000000a';
const _userB = 'bbbbbbbb-0000-4000-8000-00000000000b';

final _suggestion = find.byKey(const Key('drive-connect-suggestion'));

Future<void> _pumpSuggestion(
  WidgetTester tester, {
  required DriveStatusChecker check,
  VoidCallback? onOpenProfile,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: DriveConnectSuggestion(
          checkStatus: check,
          onOpenProfile: onOpenProfile,
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    DriveConnectSuggestion.resetSessionStateForTesting();
  });
  tearDown(() async => SessionManager.clear());

  group('DriveConnectSuggestion', () {
    testWidgets('C · con Drive conectado no aparece', (tester) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => true);
      expect(_suggestion, findsNothing);
    });

    testWidgets('A · sin Drive aparece de inmediato, informativo', (
      tester,
    ) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => false);
      expect(_suggestion, findsOneWidget);
      expect(find.text('Google Drive no está conectado'), findsOneWidget);
      // Explica dónde conectarlo, con la ruta exacta hasta el botón real.
      expect(find.text('¿DÓNDE CONECTARLO?'), findsOneWidget);
      expect(find.text('Perfil'), findsOneWidget);
      expect(find.text('Google Drive'), findsOneWidget);
      expect(find.text('CONECTAR DRIVE'), findsOneWidget);
      expect(find.text('CONECTAR GOOGLE DRIVE'), findsOneWidget);
      expect(find.byTooltip('Ocultar por ahora'), findsOneWidget);
      for (final tech in ['OAuth', 'backend', 'credenciales', 'Storage']) {
        expect(find.textContaining(tech), findsNothing);
      }
      expect(tester.takeException(), isNull);
    });

    testWidgets('ocultar dura la sesión, no para siempre', (tester) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => false);
      await tester.tap(find.byKey(const Key('drive-suggestion-dismiss')));
      await tester.pumpAndSettle();
      expect(_suggestion, findsNothing);

      // Volver a Apuntes en la misma sesión: respeta la decisión.
      await tester.pumpWidget(const SizedBox());
      await _pumpSuggestion(tester, check: () async => false);
      expect(_suggestion, findsNothing);

      // No se guarda nada persistente: nueva sesión de la app → vuelve.
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getKeys().where((k) => k.contains('drive')), isEmpty);
      DriveConnectSuggestion.resetSessionStateForTesting();
      await tester.pumpWidget(const SizedBox());
      await _pumpSuggestion(tester, check: () async => false);
      expect(_suggestion, findsOneWidget);
    });

    testWidgets('al cerrar sesión se olvida el «ocultar»', (tester) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => false);
      await tester.tap(find.byKey(const Key('drive-suggestion-dismiss')));
      await tester.pumpAndSettle();
      await tester.pumpWidget(const SizedBox());

      await SessionManager.clear();
      await SessionManager.saveSession('jwt-2', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => false);
      expect(_suggestion, findsOneWidget);
    });

    testWidgets('B · «Conectar Google Drive» navega al perfil', (tester) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      var opened = 0;
      await _pumpSuggestion(
        tester,
        check: () async => false,
        onOpenProfile: () => opened++,
      );
      await tester.tap(find.byKey(const Key('drive-suggestion-open-profile')));
      await tester.pumpAndSettle();
      expect(opened, 1);
    });

    testWidgets('B · conectar lo oculta y desconectar lo devuelve', (
      tester,
    ) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      var connected = false;
      Future<void> reopen() async {
        await tester.pumpWidget(const SizedBox());
        await _pumpSuggestion(tester, check: () async => connected);
      }

      await _pumpSuggestion(tester, check: () async => connected);
      expect(_suggestion, findsOneWidget);
      // Lo oculta, luego conecta Drive.
      await tester.tap(find.byKey(const Key('drive-suggestion-dismiss')));
      await tester.pumpAndSettle();
      connected = true;
      await reopen();
      expect(_suggestion, findsNothing);
      // Desconecta Drive: vuelve aunque antes lo hubiera ocultado.
      connected = false;
      await reopen();
      expect(_suggestion, findsOneWidget);
    });

    testWidgets('D · el estado de una cuenta no afecta a otra', (tester) async {
      await SessionManager.saveSession('jwt-a', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => false);
      await tester.tap(find.byKey(const Key('drive-suggestion-dismiss')));
      await tester.pumpAndSettle();
      expect(_suggestion, findsNothing);

      // Cambio de cuenta con la pantalla abierta: B sin Drive lo ve.
      await SessionManager.saveSession('jwt-b', {'id': _userB});
      await tester.pumpAndSettle();
      expect(_suggestion, findsOneWidget);
    });

    testWidgets('E · si el estado no se puede consultar no afirma nada', (
      tester,
    ) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      await _pumpSuggestion(
        tester,
        check: () async => throw Exception('sin red'),
      );
      expect(_suggestion, findsNothing);
      expect(find.textContaining('conectado'), findsNothing);
      expect(tester.takeException(), isNull);
    });

    testWidgets('sin sesión no consulta ni aparece', (tester) async {
      var checks = 0;
      await _pumpSuggestion(
        tester,
        check: () async {
          checks++;
          return false;
        },
      );
      expect(_suggestion, findsNothing);
      expect(checks, 0);
    });

    testWidgets('en móvil (360 px) no desborda', (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      await SessionManager.saveSession('jwt', {'id': _userA});
      await _pumpSuggestion(tester, check: () async => false);
      expect(_suggestion, findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  });

  group('Apuntes (AllNotesScreen)', () {
    tearDown(() async => AppDatabaseManager.instance.resetForTesting());

    Future<void> pumpNotes(WidgetTester tester, Widget screen) async {
      tester.view.physicalSize = const Size(1280, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(MaterialApp(home: screen));
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 300)),
      );
      for (var i = 0; i < 8; i++) {
        await tester.pump(const Duration(milliseconds: 250));
      }
    }

    testWidgets('en el shell (con onOpenProfile) muestra la sugerencia', (
      tester,
    ) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      var opened = 0;
      await pumpNotes(
        tester,
        AllNotesScreen(
          onOpenProfile: () => opened++,
          driveStatusChecker: () async => false,
        ),
      );
      expect(_suggestion, findsOneWidget);
      await tester.tap(find.byKey(const Key('drive-suggestion-open-profile')));
      await tester.pump();
      expect(opened, 1);
    });

    testWidgets('fuera del shell no consulta ni muestra la sugerencia', (
      tester,
    ) async {
      await SessionManager.saveSession('jwt', {'id': _userA});
      var checks = 0;
      await pumpNotes(
        tester,
        AllNotesScreen(
          driveStatusChecker: () async {
            checks++;
            return false;
          },
        ),
      );
      expect(_suggestion, findsNothing);
      expect(checks, 0);
    });
  });

  group('Perfil: llegada desde la sugerencia', () {
    ProfileService service() => ProfileService(
      client: MockClient((req) async {
        final body = req.url.path.endsWith('/auth/google-drive/status')
            ? {'connected': false}
            : {
                'display_name': 'Usuaria Prueba',
                'visibility': 'public',
                'institution': 'UCT',
                'public_id': 'b8kp92mqwx7r',
              };
        return http.Response(
          jsonEncode(body),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    Future<double> pumpProfile(WidgetTester tester, bool scrollToDrive) async {
      tester.view.physicalSize = const Size(800, 600);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      await SessionManager.saveSession('jwt', {'id': _userA});
      final s = service();
      addTearDown(s.dispose);
      await tester.pumpWidget(
        MaterialApp(
          home: ProfileScreen(service: s, scrollToDrive: scrollToDrive),
        ),
      );
      await tester.pumpAndSettle();
      final scrollable = tester.state<ScrollableState>(
        find.byType(Scrollable).first,
      );
      return scrollable.position.pixels;
    }

    testWidgets('scrollToDrive deja visible la tarjeta «Google Drive»', (
      tester,
    ) async {
      final offset = await pumpProfile(tester, true);
      expect(offset, greaterThan(0));
      final drive = find.text('CONECTAR DRIVE');
      expect(drive, findsOneWidget);
      final rect = tester.getRect(drive);
      expect(rect.top, greaterThanOrEqualTo(0));
      expect(rect.bottom, lessThanOrEqualTo(600));
      expect(tester.takeException(), isNull);
    });

    testWidgets('sin scrollToDrive el perfil abre arriba (sin cambios)', (
      tester,
    ) async {
      final offset = await pumpProfile(tester, false);
      expect(offset, 0);
    });
  });
}
