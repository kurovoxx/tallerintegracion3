// Ruta: lib/core/theme/app_theme.dart
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Modos de color disponibles. La geometría (bordes, radios, sombras
/// desplazadas) es la misma en ambos; solo cambia la paleta.
enum AppColorMode { neobrutalist, uct }

/// Una paleta completa. Las pantallas nunca la usan directamente: leen
/// [AppColors], que delega en la paleta del modo activo.
@immutable
class AppPalette {
  const AppPalette({
    required this.bg,
    required this.surface,
    required this.surfaceLow,
    required this.scrim,
    required this.border,
    required this.outline,
    required this.shadow,
    required this.text,
    required this.muted,
    required this.mutedStrong,
    required this.accentYellow,
    required this.accentBlue,
    required this.accentBlueDeep,
    required this.discord,
    required this.error,
    required this.errorDeep,
    required this.success,
    required this.successDeep,
    required this.pending,
    required this.pendingLight,
    required this.subjectBlue,
    required this.subjectMint,
    required this.subjectPeach,
    required this.subjectCoral,
    required this.primary,
    required this.onPrimary,
    required this.onPrimaryMuted,
    required this.onAccent,
    required this.title,
    required this.appBarBg,
    required this.appBarFg,
    required this.appBarBorder,
    required this.navSelectedBg,
    required this.navSelectedFg,
    required this.navIndicator,
    required this.infoTint,
    required this.infoInk,
    required this.errorTint,
    required this.errorTintSoft,
    required this.errorInk,
    required this.warningTint,
    required this.successTint,
    required this.codeBg,
    required this.track,
    required this.softShadows,
  });

  // Superficies
  final Color bg;
  final Color surface;
  final Color surfaceLow;
  final Color scrim;

  // Tinta: [border] es la tinta estructural (rellenos oscuros, iconos);
  // [outline] es solo el trazo de contornos y divisores.
  final Color border;
  final Color outline;
  final Color shadow;
  final Color text;
  final Color muted;
  final Color mutedStrong;

  // Acentos
  final Color accentYellow;
  final Color accentBlue;
  final Color accentBlueDeep;
  final Color discord;

  // Semánticos
  final Color error;
  final Color errorDeep;
  final Color success;
  final Color successDeep;
  final Color pending;
  final Color pendingLight;

  // Bloques de asignaturas
  final Color subjectBlue;
  final Color subjectMint;
  final Color subjectPeach;
  final Color subjectCoral;

  // Roles
  final Color primary; // botón primario
  final Color onPrimary; // texto sobre primary/rellenos oscuros
  final Color onPrimaryMuted;
  final Color onAccent; // texto sobre accentYellow (nunca blanco)
  final Color title; // títulos de página
  final Color appBarBg;
  final Color appBarFg;
  final Color appBarBorder;
  final Color navSelectedBg;
  final Color navSelectedFg;
  final Color navIndicator; // barra lateral del ítem activo

  // Tintes de estado
  final Color infoTint;
  final Color infoInk;
  final Color errorTint;
  final Color errorTintSoft;
  final Color errorInk;
  final Color warningTint;
  final Color successTint;
  final Color codeBg;
  final Color track;

  /// true: sombras suaves con desenfoque (UCT); false: sombras duras.
  final bool softShadows;

  /// Paleta original Sigma Academy (neobrutalismo retro).
  static const AppPalette neobrutalist = AppPalette(
    bg: Color(0xFFF5F0E8),
    surface: Color(0xFFFFFFFF),
    surfaceLow: Color(0xFFF2EDE5),
    scrim: Color(0x8C1A1A1A),
    border: Color(0xFF1A1A1A),
    outline: Color(0xFF1A1A1A),
    shadow: Color(0xFF1A1A1A),
    text: Color(0xFF1A1A1A),
    muted: Color(0xFF555555),
    mutedStrong: Color(0xFF444444),
    accentYellow: Color(0xFFFFCC00),
    accentBlue: Color(0xFF0055FF),
    accentBlueDeep: Color(0xFF0033CC),
    discord: Color(0xFF5865F2),
    error: Color(0xFFE63B2E),
    errorDeep: Color(0xFF8B1A12),
    success: Color(0xFF2ECC71),
    successDeep: Color(0xFF25A25A),
    pending: Color(0xFF9E9E9E),
    pendingLight: Color(0xFFBDBDBD),
    subjectBlue: Color(0xFFA0C4FF),
    subjectMint: Color(0xFFB4F8C8),
    subjectPeach: Color(0xFFFFD6A5),
    subjectCoral: Color(0xFFFFB7B2),
    primary: Color(0xFF1A1A1A),
    onPrimary: Color(0xFFFFFFFF),
    onPrimaryMuted: Color(0xB3FFFFFF),
    onAccent: Color(0xFF1A1A1A),
    title: Color(0xFF1A1A1A),
    appBarBg: Color(0xFFFFFFFF),
    appBarFg: Color(0xFF1A1A1A),
    appBarBorder: Color(0xFF1A1A1A),
    navSelectedBg: Color(0xFFFFCC00),
    navSelectedFg: Color(0xFF1A1A1A),
    navIndicator: Color(0x00000000),
    infoTint: Color(0xFFD6E3FF),
    infoInk: Color(0xFF003399),
    errorTint: Color(0xFFFFDAD6),
    errorTintSoft: Color(0xFFFDE8E8),
    errorInk: Color(0xFF990000),
    warningTint: Color(0xFFFFF3CD),
    successTint: Color(0xFFE7F6E7),
    codeBg: Color(0xFFF4F4F0),
    track: Color(0xFFE2DDD4),
    softShadows: false,
  );

  /// Paleta institucional del Portal del Estudiante UCT: barra superior
  /// azul marino, azul UCT para acciones, celeste de apoyo y dorado como
  /// acento (siempre con texto oscuro encima).
  static const AppPalette uct = AppPalette(
    bg: Color(0xFFF5F5F5),
    surface: Color(0xFFFFFFFF),
    surfaceLow: Color(0xFFEEEEEE),
    scrim: Color(0x8C01283F),
    border: Color(0xFF01568E), // marino: blanco encima 7,7:1
    outline: Color(0xFFC9D2DA),
    shadow: Color(0x2401568E),
    text: Color(0xFF333333), // 11,6:1 sobre bg
    muted: Color(0xFF666666), // 5,3:1 sobre bg
    mutedStrong: Color(0xFF555555),
    accentYellow: Color(0xFFFFC700), // texto #222 encima 10,2:1
    accentBlue: Color(0xFF0075B5), // blanco encima 5,0:1
    accentBlueDeep: Color(0xFF01568E),
    discord: Color(0xFF5865F2),
    error: Color(0xFFBD362F),
    errorDeep: Color(0xFF8B1A12),
    success: Color(0xFF51A351),
    successDeep: Color(0xFF3E8E3E),
    pending: Color(0xFF999999),
    pendingLight: Color(0xFFCCCCCC),
    subjectBlue: Color(0xFFD9EDF7),
    subjectMint: Color(0xFFDFF0D8),
    subjectPeach: Color(0xFFFFF0B3),
    subjectCoral: Color(0xFFF2DEDE),
    primary: Color(0xFF0075B5),
    onPrimary: Color(0xFFFFFFFF),
    onPrimaryMuted: Color(0xCCFFFFFF),
    onAccent: Color(0xFF222222),
    title: Color(0xFF2E6492), // 5,7:1 sobre bg
    appBarBg: Color(0xFF01568E),
    appBarFg: Color(0xFFFFFFFF),
    appBarBorder: Color(0xFF57B3DF),
    navSelectedBg: Color(0xFFE1F0FA), // marino encima 6,6:1
    navSelectedFg: Color(0xFF01568E),
    navIndicator: Color(0xFFFFC700),
    infoTint: Color(0xFFD9EDF7),
    infoInk: Color(0xFF01568E),
    errorTint: Color(0xFFF2DEDE),
    errorTintSoft: Color(0xFFFBEDED),
    errorInk: Color(0xFF8B1A12),
    warningTint: Color(0xFFFFF4CC),
    successTint: Color(0xFFDFF0D8),
    codeBg: Color(0xFFF5F7F9),
    track: Color(0xFFE5E5E5),
    softShadows: true,
  );
}

/// Modo de color activo. Se persiste en SharedPreferences y al cambiar
/// reconstruye todo el árbol de widgets conservando su estado.
class AppThemeController {
  AppThemeController._();

  static const String _prefsKey = 'app_color_mode';

  static final ValueNotifier<AppColorMode> mode = ValueNotifier<AppColorMode>(
    AppColorMode.neobrutalist,
  );

  static AppPalette get palette =>
      mode.value == AppColorMode.uct ? AppPalette.uct : AppPalette.neobrutalist;

  static bool get isUct => mode.value == AppColorMode.uct;

  /// Lee el modo guardado; si falla, se queda en neobrutalista.
  static Future<void> load() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final saved = prefs.getString(_prefsKey);
      mode.value = AppColorMode.values.firstWhere(
        (m) => m.name == saved,
        orElse: () => AppColorMode.neobrutalist,
      );
    } catch (_) {
      mode.value = AppColorMode.neobrutalist;
    }
  }

  static Future<void> setMode(AppColorMode next) async {
    if (mode.value == next) return;
    mode.value = next;
    _rebuildAll();
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_prefsKey, next.name);
    } catch (_) {
      // Sin persistencia el cambio sigue aplicado en esta sesión.
    }
  }

  static Future<void> toggle() =>
      setMode(isUct ? AppColorMode.neobrutalist : AppColorMode.uct);

  /// Los colores se leen en build(); los widgets `const` no se reconstruyen
  /// solos, así que se marcan todos los elementos (incluidas rutas y
  /// diálogos abiertos) sin perder su estado.
  static void _rebuildAll() {
    final root = WidgetsBinding.instance.rootElement;
    if (root == null) return;
    void mark(Element element) {
      element.markNeedsBuild();
      element.visitChildren(mark);
    }

    root.visitChildren(mark);
  }
}

/// Paleta canónica Sigma Academy. Fuente de verdad de color: prohibido
/// declarar colores sueltos en las pantallas (ver
/// .skills/flutter_neobrutalism_ui.md, sección 2). Los valores dependen del
/// modo activo de [AppThemeController].
class AppColors {
  AppColors._();

  static AppPalette get _p => AppThemeController.palette;

  // Superficies
  static Color get bg => _p.bg; // Canvas
  static Color get surface => _p.surface; // Tarjetas/etiquetas
  static Color get surfaceLow => _p.surfaceLow; // Relieve, filas alternas
  static Color get scrim => _p.scrim; // Velo de modales

  // Tinta
  static Color get border => _p.border;
  static Color get outline => _p.outline;
  static Color get shadow => _p.shadow;
  static Color get text => _p.text;
  static Color get muted => _p.muted;
  static Color get mutedStrong => _p.mutedStrong; // AAA sobre bg

  // Acentos
  static Color get accentYellow => _p.accentYellow;
  static Color get accentBlue => _p.accentBlue;
  static Color get accentBlueDeep => _p.accentBlueDeep; // AAA con blanco
  static Color get discord => _p.discord; // Comunidad/social

  // Semánticos
  static Color get error => _p.error;
  static Color get errorDeep => _p.errorDeep; // AAA con blanco
  static Color get success => _p.success;
  static Color get successDeep => _p.successDeep;
  static Color get pending => _p.pending;
  static Color get pendingLight => _p.pendingLight;

  // Estado académico (alias semánticos)
  static Color get gradeApproved => success;
  static Color get gradeInProgress => accentYellow;
  static Color get gradeFailed => errorDeep;
  static Color get gradePending => pendingLight;

  // Bloques de asignaturas (horario/matriz)
  static Color get subjectBlue => _p.subjectBlue;
  static Color get subjectMint => _p.subjectMint;
  static Color get subjectPeach => _p.subjectPeach;
  static Color get subjectCoral => _p.subjectCoral;

  // Roles
  static Color get primary => _p.primary;
  static Color get onPrimary => _p.onPrimary;
  static Color get onPrimaryMuted => _p.onPrimaryMuted;
  static Color get onAccent => _p.onAccent;
  static Color get title => _p.title;
  static Color get appBarBg => _p.appBarBg;
  static Color get appBarFg => _p.appBarFg;
  static Color get appBarBorder => _p.appBarBorder;
  static Color get navSelectedBg => _p.navSelectedBg;
  static Color get navSelectedFg => _p.navSelectedFg;
  static Color get navIndicator => _p.navIndicator;

  // Tintes de estado
  static Color get infoTint => _p.infoTint;
  static Color get infoInk => _p.infoInk;
  static Color get errorTint => _p.errorTint;
  static Color get errorTintSoft => _p.errorTintSoft;
  static Color get errorInk => _p.errorInk;
  static Color get warningTint => _p.warningTint;
  static Color get successTint => _p.successTint;
  static Color get codeBg => _p.codeBg;
  static Color get track => _p.track;
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

/// Sombras canónicas. En modo neobrutalista son duras (blur y spread en 0);
/// en modo UCT son suaves y sin desplazamiento lateral.
class AppShadows {
  AppShadows._();

  static const Offset offsetBadge = Offset(2, 2);
  static const Offset offsetButton = Offset(3, 3);
  static const Offset offsetCard = Offset(4, 4);
  static const Offset offsetHero = Offset(6, 6); // modales/hero, excepcional

  static BoxShadow _shadow(Offset hard, double softY, double softBlur) {
    final p = AppThemeController.palette;
    if (p.softShadows) {
      return BoxShadow(
        color: p.shadow,
        offset: Offset(0, softY),
        blurRadius: softBlur,
      );
    }
    return BoxShadow(color: p.shadow, offset: hard);
  }

  static BoxShadow get hard2 => _shadow(offsetBadge, 1, 2);
  static BoxShadow get hard3 => _shadow(offsetButton, 1, 3);
  static BoxShadow get hard4 => _shadow(offsetCard, 2, 6);
  static BoxShadow get hard6 => _shadow(offsetHero, 8, 24);

  /// Sombra lateral de la sidebar/rail (hard-edge hacia la derecha).
  static BoxShadow get hardRight {
    final p = AppThemeController.palette;
    return BoxShadow(
      color: p.softShadows ? p.outline : p.shadow,
      offset: Offset(p.softShadows ? 1 : 4, 0),
    );
  }

  static List<BoxShadow> get badge => <BoxShadow>[hard2];
  static List<BoxShadow> get button => <BoxShadow>[hard3];
  static List<BoxShadow> get card => <BoxShadow>[hard4];
  static List<BoxShadow> get dialog => <BoxShadow>[hard6];
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
