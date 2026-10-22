import 'dart:ui' show AppExitResponse;

import 'package:flutter/material.dart';

import 'core/database/app_database_manager.dart';
import 'core/services/insecure_http.dart';
import 'core/services/session_manager.dart';
import 'core/theme/app_theme.dart';
import 'core/widgets/main_shell.dart';
import 'features/auth/login_screen.dart';

/// Navegación global para expiración de sesión (ver AuthedHttp).
final GlobalKey<NavigatorState> appNavigatorKey = GlobalKey<NavigatorState>();

Future<void> main() async {
  // Confianza por huella del certificado de Pillán en desktop/móvil.
  // En web el navegador valida TLS. Debe ir antes de cualquier http.Client.
  configurePillanCertificateTrust();
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

class TallerIntegracionApp extends StatefulWidget {
  const TallerIntegracionApp({super.key, this.home});

  final Widget? home;

  @override
  State<TallerIntegracionApp> createState() => _TallerIntegracionAppState();
}

class _TallerIntegracionAppState extends State<TallerIntegracionApp> {
  late final AppLifecycleListener _lifecycle;

  @override
  void initState() {
    super.initState();
    // Cierre controlado de SQLite: espera operaciones en vuelo antes de que
    // el proceso termine (evita "isolate channel connection was closed").
    _lifecycle = AppLifecycleListener(
      onExitRequested: () async {
        await AppDatabaseManager.instance.close();
        return AppExitResponse.exit;
      },
    );
  }

  @override
  void dispose() {
    _lifecycle.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Sigma Academy',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light,
      navigatorKey: appNavigatorKey,
      home: widget.home ?? const LoginScreen(),
    );
  }
}
