// Ruta: lib/theme/app_theme.dart
import 'package:flutter/material.dart';

/// Paleta y medidas extraídas de styles.css. Cualquier pantalla nueva
/// (chat, malla, perfil) reutiliza esto en vez de declarar colores sueltos.
class AppColors {
  AppColors._();

  static const Color bg = Color(0xFFF5F0E8);
  static const Color surface = Color(0xFFFFFFFF);
  static const Color surfaceLow = Color(0xFFF2EDE5);
  static const Color border = Color(0xFF1A1A1A);
  static const Color accentYellow = Color(0xFFFFCC00);
  static const Color accentBlue = Color(0xFF0055FF);
  static const Color text = Color(0xFF1A1A1A);
  static const Color muted = Color(0xFF555555);
  static const Color error = Color(0xFFE63B2E);
  static const Color discord = Color(0xFF5865F2);
}

class AppDimens {
  AppDimens._();

  static const double borderWidth = 2;
  static const double radius = 0;
  static const double breakpointDesktop = 1024;
}

class AppTheme {
  AppTheme._();

  static ThemeData get light => ThemeData(
        scaffoldBackgroundColor: AppColors.bg,
        fontFamily: 'sans-serif',
      );
}
