import 'package:flutter/material.dart';

import 'core/services/session_manager.dart';
import 'core/theme/app_theme.dart';
import 'core/widgets/main_shell.dart';
import 'features/auth/login_screen.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
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
      home: home ?? const LoginScreen(),
    );
  }
}
