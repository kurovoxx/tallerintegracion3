import 'package:flutter/material.dart';

import '../../features/auth/login_screen.dart';

/// Nombre de la ruta de login abierta al cerrar o perder la sesión.
const loginRouteName = '/login';

bool _loginOnTop(NavigatorState navigator) {
  String? top;
  // popUntil con predicado verdadero no quita nada: solo lee la ruta superior.
  navigator.popUntil((route) {
    top = route.settings.name;
    return true;
  });
  return top == loginRouteName;
}

/// Reemplaza toda la pila por el login, salvo que ya esté arriba. Devuelve si
/// navegó. Idempotente a propósito: el borrado de cuenta y la expiración de
/// sesión pueden pedir el login casi a la vez, y dos navegaciones seguidas
/// dejaban un login duplicado o, peor, un pop sobre el login recién abierto
/// (navegador vacío, pantalla negra).
bool replaceStackWithLogin(NavigatorState navigator) {
  if (_loginOnTop(navigator)) return false;
  navigator.pushAndRemoveUntil(
    MaterialPageRoute<void>(
      settings: const RouteSettings(name: loginRouteName),
      builder: (_) => const LoginScreen(),
    ),
    (route) => false,
  );
  return true;
}

/// Manejador global de sesión expirada (`SessionManager.onSessionExpired`):
/// lleva al login y avisa al usuario. Vive aquí, y no dentro de `main`, para
/// que las pruebas usen el mismo código que la app.
Future<void> Function() sessionExpiredNavigator(
  GlobalKey<NavigatorState> navigatorKey,
) {
  return () async {
    final nav = navigatorKey.currentState;
    if (nav == null) return;
    // Si otro flujo ya llevó al login (p. ej. el borrado de cuenta), ese flujo
    // ya mostró su propio mensaje.
    if (!replaceStackWithLogin(nav)) return;
    // Mensaje humano tras navegar (el contexto ya es Login).
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final ctx = navigatorKey.currentContext;
      if (ctx == null) return;
      ScaffoldMessenger.of(ctx).showSnackBar(
        const SnackBar(
          content: Text('Tu sesión expiró. Inicia sesión nuevamente.'),
        ),
      );
    });
  };
}
