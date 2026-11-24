import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/theme/app_theme.dart';
import 'package:taller_integracion_front/core/widgets/color_mode_toggle.dart';

/// Widget `const` que lee el color en build(): comprueba que el cambio de
/// modo lo reconstruye aunque su instancia no cambie.
class _Swatch extends StatelessWidget {
  const _Swatch();

  @override
  Widget build(BuildContext context) {
    return Container(
      key: const ValueKey('swatch'),
      width: 10,
      height: 10,
      color: AppColors.primary,
    );
  }
}

Color _swatchColor(WidgetTester tester) =>
    tester.widget<Container>(find.byKey(const ValueKey('swatch'))).color!;

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    AppThemeController.mode.value = AppColorMode.neobrutalist;
  });

  tearDown(() => AppThemeController.mode.value = AppColorMode.neobrutalist);

  test('la paleta neobrutalista conserva los colores originales', () {
    expect(AppColors.bg, const Color(0xFFF5F0E8));
    expect(AppColors.border, const Color(0xFF1A1A1A));
    expect(AppColors.accentYellow, const Color(0xFFFFCC00));
    expect(AppColors.accentBlue, const Color(0xFF0055FF));
    expect(AppColors.primary, const Color(0xFF1A1A1A));
    expect(AppShadows.hard4.blurRadius, 0);
    expect(AppShadows.hard4.offset, AppShadows.offsetCard);
  });

  test('la paleta UCT usa los colores del portal', () {
    AppThemeController.mode.value = AppColorMode.uct;
    expect(AppColors.appBarBg, const Color(0xFF01568E));
    expect(AppColors.primary, const Color(0xFF0075B5));
    expect(AppColors.accentYellow, const Color(0xFFFFC700));
    expect(AppColors.appBarBorder, const Color(0xFF57B3DF));
    expect(AppColors.title, const Color(0xFF2E6492));
    // Sobre el dorado nunca va texto blanco.
    expect(AppColors.onAccent, isNot(const Color(0xFFFFFFFF)));
    expect(AppShadows.hard4.blurRadius, greaterThan(0));
  });

  testWidgets('el botón alterna el modo, reconstruye y lo guarda', (
    tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: Column(children: <Widget>[ColorModeToggle(), _Swatch()]),
        ),
      ),
    );
    expect(_swatchColor(tester), AppPalette.neobrutalist.primary);
    expect(find.byIcon(Icons.palette_outlined), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('color-mode-toggle')));
    await tester.pumpAndSettle();

    expect(AppThemeController.isUct, isTrue);
    expect(_swatchColor(tester), AppPalette.uct.primary);
    expect(find.byIcon(Icons.palette_rounded), findsOneWidget);
    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getString('app_color_mode'), 'uct');

    await tester.tap(find.byKey(const ValueKey('color-mode-toggle')));
    await tester.pumpAndSettle();
    expect(AppThemeController.isUct, isFalse);
    expect(_swatchColor(tester), AppPalette.neobrutalist.primary);
    expect(prefs.getString('app_color_mode'), 'neobrutalist');
  });

  test('load recupera el modo guardado', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{
      'app_color_mode': 'uct',
    });
    await AppThemeController.load();
    expect(AppThemeController.isUct, isTrue);
  });
}
