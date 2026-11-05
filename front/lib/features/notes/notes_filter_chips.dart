import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Fila de chips de filtro neobrutalistas (selección = amarillo resaltador).
class NotesFilterChips<T> extends StatelessWidget {
  const NotesFilterChips({
    super.key,
    required this.value,
    required this.options,
    required this.onChanged,
    this.keyPrefix = 'filter-',
  });

  final T value;

  /// Prefijo de las keys de cada chip (`<prefijo><etiqueta>`), para poder
  /// tener dos grupos con etiquetas repetidas en la misma pantalla.
  final String keyPrefix;
  final Map<T, String> options;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: AppDimens.spaceSm,
      runSpacing: AppDimens.spaceSm,
      children: <Widget>[
        for (final entry in options.entries)
          NeobrutalistButton(
            key: Key('$keyPrefix${entry.value}'),
            label: entry.value,
            variant: entry.key == value
                ? NeobrutalistButtonVariant.accent
                : NeobrutalistButtonVariant.secondary,
            padding: const EdgeInsets.symmetric(
              horizontal: AppDimens.spaceMd,
              vertical: AppDimens.spaceXs + 2,
            ),
            onPressed: () => onChanged(entry.key),
          ),
      ],
    );
  }
}
