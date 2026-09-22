// Ruta: lib/core/theme/app_theme.dart
import 'package:flutter/material.dart';

/// Paleta canónica Sigma Academy (neobrutalismo retro).
/// Fuente de verdad de color: prohibido declarar colores sueltos en las
/// pantallas (ver .skills/flutter_neobrutalism_ui.md, sección 2).
class AppColors {
  AppColors._();

  // Superficies
  static const Color bg = Color(0xFFF5F0E8); // Canvas pergamino
  static const Color surface = Color(0xFFFFFFFF); // Tarjetas/etiquetas
  static const Color surfaceLow = Color(0xFFF2EDE5); // Relieve, filas alternas
  static const Color scrim = Color(0x8C1A1A1A); // Velo de modales

  // Tinta
  static const Color border = Color(0xFF1A1A1A);
  static const Color text = Color(0xFF1A1A1A);
  static const Color muted = Color(0xFF555555);
  static const Color mutedStrong = Color(0xFF444444); // AAA sobre bg

  // Acentos
  static const Color accentYellow = Color(0xFFFFCC00); // Highlighter
  static const Color accentBlue = Color(0xFF0055FF); // Cobalt
  static const Color accentBlueDeep = Color(0xFF0033CC); // AAA con blanco
  static const Color discord = Color(0xFF5865F2); // Comunidad/social

  // Semánticos
  static const Color error = Color(0xFFE63B2E);
  static const Color errorDeep = Color(0xFF8B1A12); // AAA con blanco
  static const Color success = Color(0xFF2ECC71);
  static const Color successDeep = Color(0xFF25A25A);
  static const Color pending = Color(0xFF9E9E9E);
  static const Color pendingLight = Color(0xFFBDBDBD); // AAA con negro

  // Estado académico (alias semánticos)
  static const Color gradeApproved = success;
  static const Color gradeInProgress = accentYellow;
  static const Color gradeFailed = errorDeep;
  static const Color gradePending = pendingLight;

  // Paleta plana retro para bloques de asignaturas (horario/matriz).
  static const Color subjectBlue = Color(0xFFA0C4FF); // Azul técnico/celeste
  static const Color subjectMint = Color(0xFFB4F8C8); // Menta pastel
  static const Color subjectPeach = Color(0xFFFFD6A5); // Durazno
  static const Color subjectCoral = Color(0xFFFFB7B2); // Coral
}

/// Medidas, radios, breakpoints y layout canónicos.
class AppDimens {
  AppDimens._();

  // Bordes
  static const double borderWidth = 2; // Estándar estructural
  static const double borderWidthAction = 2.5; // Botones de acción/FAB
  static const double borderWidthThick = 3; // Modales

  // Radios (0 = 90 grados; chips excepcionales)
  static const double radius = 0;
  static const double radiusChip = 4;
  static const double radiusSoft = 6; // Máximo permitido, nunca estructural

  // Breakpoints unificados
  static const double breakpointCompact = 600;
  static const double breakpointMedium = 1024;
  static const double breakpointDesktop = breakpointMedium; // compatibilidad

  // Shell / layout
  static const double sidebarExpanded = 280;
  static const double sidebarCollapsed = 72;
  static const double contentMaxWidth = 1280; // rango permitido 1200-1400
  static const double gutter = 16;

  // Escala de espaciado
  static const double spaceXs = 4;
  static const double spaceSm = 8;
  static const double spaceMd = 12;
  static const double spaceLg = 16;
  static const double spaceXl = 24;
  static const double spaceXxl = 32;
  static const double spaceHero = 48;
}

/// Sombras duras canónicas (blur y spread SIEMPRE en 0).
class AppShadows {
  AppShadows._();

  static const Offset offsetBadge = Offset(2, 2);
  static const Offset offsetButton = Offset(3, 3);
  static const Offset offsetCard = Offset(4, 4);
  static const Offset offsetHero = Offset(6, 6); // modales/hero, excepcional

  static const BoxShadow hard2 = BoxShadow(
    color: AppColors.border,
    offset: offsetBadge,
    blurRadius: 0,
    spreadRadius: 0,
  );
  static const BoxShadow hard3 = BoxShadow(
    color: AppColors.border,
    offset: offsetButton,
    blurRadius: 0,
    spreadRadius: 0,
  );
  static const BoxShadow hard4 = BoxShadow(
    color: AppColors.border,
    offset: offsetCard,
    blurRadius: 0,
    spreadRadius: 0,
  );
  static const BoxShadow hard6 = BoxShadow(
    color: AppColors.border,
    offset: offsetHero,
    blurRadius: 0,
    spreadRadius: 0,
  );

  /// Sombra lateral de la sidebar/rail (hard-edge hacia la derecha).
  static const BoxShadow hardRight = BoxShadow(
    color: AppColors.border,
    offset: Offset(4, 0),
    blurRadius: 0,
    spreadRadius: 0,
  );

  static const List<BoxShadow> badge = <BoxShadow>[hard2];
  static const List<BoxShadow> button = <BoxShadow>[hard3];
  static const List<BoxShadow> card = <BoxShadow>[hard4];
  static const List<BoxShadow> dialog = <BoxShadow>[hard6];
}

/// Duraciones y curvas del sistema de movimiento.
class AppMotion {
  AppMotion._();

  static const Duration press = Duration(milliseconds: 90);
  static const Duration fast = Duration(milliseconds: 120);
  static const Duration medium = Duration(milliseconds: 180);
  static const Duration expand = Duration(milliseconds: 220);

  static const Curve standard = Curves.easeOutCubic;
  static const Curve shell = Curves.easeInOut;
}

class AppTheme {
  AppTheme._();

  static ThemeData get light => ThemeData(
    scaffoldBackgroundColor: AppColors.bg,
    fontFamily: 'sans-serif',
    // Sin ripple Material: el feedback lo da el hundimiento neobrutalista.
    splashFactory: NoSplash.splashFactory,
    highlightColor: Colors.transparent,
  );
}
