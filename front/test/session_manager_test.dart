import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/auth/login_screen.dart';
import 'support/controlled_preferences_store.dart';

void main() {
  const validToken =
      'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjMiLCJlbWFpbCI6InRlc3RAdWN0LmNsIiwiZXhwIjoxODkzNDU2MDAwfQ.signature';
  const expiredToken =
      'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjMiLCJlbWFpbCI6InRlc3RAdWN0LmNsIiwiZXhwIjoxMDAwMDAwMDAwfQ.signature';
  const testUserData = {'id': 'user-1', 'email': 'test@uct.cl'};
  late ControlledPreferencesStore storage;

  setUp(() {
    storage = ControlledPreferencesStore()..install();
    SessionManager.clockOverride = null;
  });

  tearDown(() async {
    storage.holdSessionClear = false;
    if (!storage.releaseClear.isCompleted) storage.releaseClear.complete();
    storage.throwOnTokenPairWrite = false;
    storage.tokenPairFailuresRemaining = 0;
    SessionManager.onSessionExpired = null;
    await SessionManager.clear();
  });

  group('SessionManager Remember Me y Persistencia', () {
    test('saveSession sin rememberMe guarda solo en memoria volatil', () async {
      await SessionManager.saveSession(validToken, testUserData, rememberMe: false);

      expect(SessionManager.isLoggedIn, isTrue);
      expect(SessionManager.token, equals(validToken));
      expect(SessionManager.user, equals(testUserData));

      // Limpiamos memoria para verificar si quedó persistido en disco
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getBool('auth_remember_me'), isNull);
      expect(prefs.getString('auth_token'), isNull);

      SessionManager.resetInMemory();
      expect(await SessionManager.hasPersistedSession(), isFalse);
    });

    test('saveSession con rememberMe: true persiste en disco y se restaura', () async {
      await SessionManager.saveSession(validToken, testUserData, rememberMe: true);

      expect(SessionManager.isLoggedIn, isTrue);

      // Simulamos reinicio de la app (vaciando memoria)
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getBool('auth_remember_me'), isTrue);
      expect(prefs.getString('auth_token'), equals(validToken));

      // Simulamos nueva sesión limpia en memoria
      // (llamando hasPersistedSession para restaurar)
      SessionManager.resetInMemory();
      final restored = await SessionManager.hasPersistedSession();
      expect(restored, isTrue);
      expect(SessionManager.isLoggedIn, isTrue);
      expect(SessionManager.token, equals(validToken));
      expect(SessionManager.user?['email'], equals('test@uct.cl'));
    });

    test('hasPersistedSession conserva refresh cuando access ya expiró', () async {
      await SessionManager.saveSession(
        expiredToken,
        testUserData,
        rememberMe: true,
        refreshToken: 'refresh-vigente',
      );
      SessionManager.resetInMemory();

      final restored = await SessionManager.hasPersistedSession();
      expect(restored, isTrue);
      expect(SessionManager.isLoggedIn, isTrue);
      expect(SessionManager.refreshToken, 'refresh-vigente');

      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getBool('auth_remember_me'), isTrue);
      expect(prefs.getString('refresh_token'), 'refresh-vigente');
    });

    test('descarta access expirado si no hay refresh para renovarlo', () async {
      await SessionManager.saveSession(expiredToken, testUserData, rememberMe: true);
      SessionManager.resetInMemory();

      final restored = await SessionManager.hasPersistedSession();

      expect(restored, isFalse);
      expect(SessionManager.isLoggedIn, isFalse);
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getBool('auth_remember_me'), isNull);
      expect(prefs.getString('auth_token'), isNull);
    });

    test('validación JWT usa el reloj controlado en pruebas', () {
      SessionManager.clockOverride = () => DateTime.utc(2029, 12, 31);
      expect(SessionManager.isAccessTokenValid(validToken), isTrue);

      SessionManager.clockOverride = () => DateTime.utc(2030, 1, 1);
      expect(SessionManager.isAccessTokenValid(validToken), isFalse);
    });

    test('clear() limpia tanto memoria como disco', () async {
      await SessionManager.saveSession(validToken, testUserData, rememberMe: true);
      expect(SessionManager.isLoggedIn, isTrue);

      await SessionManager.clear();

      expect(SessionManager.isLoggedIn, isFalse);
      expect(SessionManager.token, isNull);
      expect(SessionManager.user, isNull);

      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getBool('auth_remember_me'), isNull);
      expect(prefs.getString('auth_token'), isNull);
      expect(prefs.getString('auth_user'), isNull);
    });

    for (final failureMode in ['returns false', 'throws']) {
      test('updateTokens conserva la rotación en memoria si $failureMode', () async {
        await SessionManager.saveSession(
          'access-anterior',
          testUserData,
          rememberMe: true,
          refreshToken: 'refresh-anterior',
        );
        final sessionRevision = SessionManager.revision.value;
        storage.tokenPairWriteAttempts = 0;
        if (failureMode == 'returns false') {
          storage.tokenPairFailuresRemaining = 2;
        } else {
          storage.throwOnTokenPairWrite = true;
        }

        final result = await SessionManager.updateTokens(
          'access-rotado',
          'refresh-rotado',
          expectedRevision: sessionRevision,
          expectedRefreshToken: 'refresh-anterior',
        );

        expect(
          result,
          TokenUpdateResult.rotationAppliedInMemoryPersistenceFailed,
        );
        expect(SessionManager.token, 'access-rotado');
        expect(SessionManager.refreshToken, 'refresh-rotado');
        expect(storage.tokenPairWriteAttempts, 2);

        // El cache de SharedPreferences puede contener escrituras fallidas;
        // se descarta para leer lo que realmente quedó en el almacenamiento.
        SharedPreferences.resetStatic();
        var preferences = await SharedPreferences.getInstance();
        expect(
          jsonDecode(preferences.getString('auth_tokens_v1')!),
          {
            'access_token': 'access-anterior',
            'refresh_token': 'refresh-anterior',
          },
        );

        // Reinicio simulado: solo vuelve a cargar el par que persistió.
        SessionManager.resetInMemory();
        SharedPreferences.resetStatic();
        expect(await SessionManager.hasPersistedSession(), isTrue);
        expect(SessionManager.token, 'access-anterior');
        expect(SessionManager.refreshToken, 'refresh-anterior');

        preferences = await SharedPreferences.getInstance();
        expect(
          jsonDecode(preferences.getString('auth_tokens_v1')!),
          {
            'access_token': 'access-anterior',
            'refresh_token': 'refresh-anterior',
          },
        );
      });
    }

    test('updateTokens reintenta una vez y confirma el par persistido', () async {
      await SessionManager.saveSession(
        'access-anterior',
        testUserData,
        rememberMe: true,
        refreshToken: 'refresh-anterior',
      );
      final sessionRevision = SessionManager.revision.value;
      storage.tokenPairWriteAttempts = 0;
      storage.tokenPairFailuresRemaining = 1;

      final result = await SessionManager.updateTokens(
        'access-rotado',
        'refresh-rotado',
        expectedRevision: sessionRevision,
        expectedRefreshToken: 'refresh-anterior',
      );

      expect(result, TokenUpdateResult.rotationAppliedAndPersisted);
      expect(storage.tokenPairWriteAttempts, 2);
      SharedPreferences.resetStatic();
      final preferences = await SharedPreferences.getInstance();
      expect(
        jsonDecode(preferences.getString('auth_tokens_v1')!),
        {
          'access_token': 'access-rotado',
          'refresh_token': 'refresh-rotado',
        },
      );
    });

    test('expiración antigua no avisa después de iniciar otra sesión', () async {
      await SessionManager.saveSession(
        'access-A',
        {'id': 'A'},
        rememberMe: true,
        refreshToken: 'refresh-A',
      );
      var expiredCallbacks = 0;
      SessionManager.onSessionExpired = () async {
        expiredCallbacks++;
      };
      storage.holdSessionClear = true;

      final expiration = SessionManager.expireSession(
        expectedRevision: SessionManager.revision.value,
        expectedRefreshToken: 'refresh-A',
        checkRefreshToken: true,
      );
      await storage.clearStarted.future;

      final newLogin = SessionManager.saveSession(
        'access-B',
        {'id': 'B'},
        rememberMe: true,
        refreshToken: 'refresh-B',
      );
      expect(SessionManager.token, 'access-B');
      expect(SessionManager.refreshToken, 'refresh-B');

      storage.holdSessionClear = false;
      storage.releaseClear.complete();
      await Future.wait([expiration, newLogin]);

      expect(expiredCallbacks, 0);
      expect(SessionManager.token, 'access-B');
      expect(SessionManager.refreshToken, 'refresh-B');

      SharedPreferences.resetStatic();
      var preferences = await SharedPreferences.getInstance();
      expect(
        jsonDecode(preferences.getString('auth_tokens_v1')!),
        {'access_token': 'access-B', 'refresh_token': 'refresh-B'},
      );

      // Reiniciar confirma que el guardado en cola terminó con la sesión B.
      SessionManager.resetInMemory();
      SharedPreferences.resetStatic();
      expect(await SessionManager.hasPersistedSession(), isTrue);
      expect(SessionManager.token, 'access-B');
      expect(SessionManager.refreshToken, 'refresh-B');
      preferences = await SharedPreferences.getInstance();
      expect(
        jsonDecode(preferences.getString('auth_tokens_v1')!),
        {'access_token': 'access-B', 'refresh_token': 'refresh-B'},
      );
    });

    testWidgets('LoginScreen muestra checkbox neobrutalista RECORDARME marcado por defecto', (
      tester,
    ) async {
      await tester.binding.setSurfaceSize(const Size(1280, 900));
      addTearDown(() => tester.binding.setSurfaceSize(null));

      await tester.pumpWidget(
        const MaterialApp(
          home: LoginScreen(),
        ),
      );
      await tester.pump();

      expect(find.text('RECORDARME'), findsOneWidget);
      final checkboxFinder = find.byType(Checkbox);
      expect(checkboxFinder, findsOneWidget);

      final checkbox = tester.widget<Checkbox>(checkboxFinder);
      expect(checkbox.value, isTrue);

      // Tap para desmarcar
      await tester.tap(checkboxFinder);
      await tester.pump();

      final checkboxUnchecked = tester.widget<Checkbox>(checkboxFinder);
      expect(checkboxUnchecked.value, isFalse);
    });
  });
}

