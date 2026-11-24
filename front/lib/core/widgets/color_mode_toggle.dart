import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

/// Botón de la barra superior que alterna entre la paleta neobrutalista
/// (por defecto) y la paleta institucional UCT. Funciona como el típico
/// interruptor de modo oscuro: un solo ícono, sin texto.
class ColorModeToggle extends StatelessWidget {
  const ColorModeToggle({super.key});

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<AppColorMode>(
      valueListenable: AppThemeController.mode,
      builder: (context, mode, _) {
        final isUct = mode == AppColorMode.uct;
        return Semantics(
          toggled: isUct,
          label: 'Modo color UCT',
          child: IconButton(
            key: const ValueKey('color-mode-toggle'),
            tooltip: isUct ? 'Volver a colores Sigma' : 'Modo color UCT',
            onPressed: AppThemeController.toggle,
            icon: AnimatedContainer(
              duration: AppMotion.medium,
              curve: AppMotion.standard,
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: isUct ? AppColors.accentYellow : Colors.transparent,
                shape: BoxShape.circle,
              ),
              child: Icon(
                isUct ? Icons.palette_rounded : Icons.palette_outlined,
                size: 20,
                color: isUct ? AppColors.onAccent : AppColors.appBarFg,
              ),
            ),
          ),
        );
      },
    );
  }
}
