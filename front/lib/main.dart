import 'package:flutter/material.dart';

import 'core/services/insecure_http.dart';
import 'core/services/session_manager.dart';
import 'core/theme/app_theme.dart';
import 'core/widgets/main_shell.dart';
import 'features/auth/login_screen.dart';

/// Navegación global para expiración de sesión (ver AuthedHttp).
final GlobalKey<NavigatorState> appNavigatorKey = GlobalKey<NavigatorState>();

Future<void> main() async {
  // Solo-dev desktop: permite self-signed de pillan con ALLOW_INSECURE=true.
  // En web es no-op. Debe ir antes de cualquier http.Client.
  maybeAllowInsecureCerts();
  WidgetsFlutterBinding.ensureInitialized();
  SessionManager.onSessionExpired = () async {
    final nav = appNavigatorKey.currentState;
    if (nav == null) return;
    nav.pushAndRemoveUntil(
      MaterialPageRoute(builder: (_) => const LoginScreen()),
      (route) => false,
    );
    // Mensaje humano tras navegar (el contexto ya es Login).
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final ctx = appNavigatorKey.currentContext;
      if (ctx == null) return;
      ScaffoldMessenger.of(ctx).showSnackBar(
        const SnackBar(
          content: Text('Tu sesión expiró. Inicia sesión nuevamente.'),
        ),
      );
    });
  };
  final hasPersistedSession = await SessionManager.hasPersistedSession();
  runApp(
    TallerIntegracionApp(
      home: hasPersistedSession
          ? MainShell(userData: SessionManager.user, initialIndex: 7)
          : const LoginScreen(),
    ),
  );
}

class TallerIntegracionApp extends StatelessWidget {
  const TallerIntegracionApp({super.key, this.home});

  final Widget? home;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Sigma Academy',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light,
      navigatorKey: appNavigatorKey,
      home: home ?? const LoginScreen(),
    );
  }
}
