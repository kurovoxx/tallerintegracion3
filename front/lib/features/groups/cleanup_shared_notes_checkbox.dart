import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';

/// Casilla reutilizable para las acciones de grupo: «Retirar del grupo las
/// notas compartidas por este integrante». Marcada por defecto; el usuario
/// puede desactivarla antes de confirmar. El valor se envía como
/// `cleanup_shared_notes` a la API de Social.
class CleanupSharedNotesCheckbox extends StatelessWidget {
  const CleanupSharedNotesCheckbox({
    super.key,
    required this.value,
    required this.onChanged,
    this.label =
        'Retirar del grupo las notas compartidas por este integrante',
  });

  final bool value;
  final ValueChanged<bool> onChanged;
  final String label;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: () => onChanged(!value),
      child: Container(
        padding: const EdgeInsets.symmetric(
          horizontal: AppDimens.spaceMd,
          vertical: AppDimens.spaceSm,
        ),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: AppShadows.badge,
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // IgnorePointer: el toggle lo maneja el GestureDetector del
            // contenedor, evitando un doble cambio al tocar el checkbox.
            IgnorePointer(
              child: Checkbox(
                value: value,
                onChanged: (_) {},
                activeColor: AppColors.border,
                checkColor: AppColors.surface,
                side: const BorderSide(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
                materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
            const SizedBox(width: AppDimens.spaceSm),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: AppColors.text,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
