import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/auth/login_screen.dart';

void main() {
  const validToken =
      'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjMiLCJlbWFpbCI6InRlc3RAdWN0LmNsIiwiZXhwIjoxODkzNDU2MDAwfQ.signature';
  const expiredToken =
      'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjMiLCJlbWFpbCI6InRlc3RAdWN0LmNsIiwiZXhwIjoxMDAwMDAwMDAwfQ.signature';
  const testUserData = {'id': 'user-1', 'email': 'test@uct.cl'};

  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  tearDown(() async {
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
      final restored = await SessionManager.hasPersistedSession();
      expect(restored, isTrue);
      expect(SessionManager.isLoggedIn, isTrue);
      expect(SessionManager.token, equals(validToken));
      expect(SessionManager.user?['email'], equals('test@uct.cl'));
    });

    test('hasPersistedSession rechaza token expirado y limpia persistencia', () async {
      await SessionManager.saveSession(expiredToken, testUserData, rememberMe: true);

      final restored = await SessionManager.hasPersistedSession();
      expect(restored, isFalse);
      expect(SessionManager.isLoggedIn, isFalse);

      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getBool('auth_remember_me'), isNull);
      expect(prefs.getString('auth_token'), isNull);
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

