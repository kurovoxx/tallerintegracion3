import 'package:flutter/material.dart';

import 'theme/app_theme.dart';
/// Etiqueta uppercase arriba de cada input (CORREO, CONTRASEÑA...).
class AppFieldLabel extends StatelessWidget {
  const AppFieldLabel(this.text, {super.key});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w900,
        color: AppColors.text,
        letterSpacing: 0.5,
      ),
    );
  }
}

/// Estilo compartido de todos los TextFormField (borde grueso, fill gris,
/// error rojo).
InputDecoration appInputDecoration(String hint) {
  return InputDecoration(
    hintText: hint,
    hintStyle: const TextStyle(color: AppColors.muted, fontWeight: FontWeight.w600, fontSize: 14),
    filled: true,
    fillColor: AppColors.bg,
    contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      borderSide: const BorderSide(color: AppColors.border, width: AppDimens.borderWidth),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      borderSide: const BorderSide(color: AppColors.border, width: AppDimens.borderWidth),
    ),
    errorBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      borderSide: const BorderSide(color: AppColors.error, width: AppDimens.borderWidth),
    ),
    focusedErrorBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      borderSide: const BorderSide(color: AppColors.error, width: AppDimens.borderWidth),
    ),
  );
}

/// Botón negro con flecha (login, registro, y cualquier acción principal futura).
class SubmitButton extends StatelessWidget {
  const SubmitButton({super.key, required this.text, required this.onPressed});

  final String text;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onPressed,
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 14),
          decoration: BoxDecoration(
            color: AppColors.border,
            border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: const [
              BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0),
            ],
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text(
                text,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w800,
                  color: Colors.white,
                  letterSpacing: 0.5,
                ),
              ),
              const SizedBox(width: 8),
              const Icon(Icons.arrow_forward_rounded, color: Colors.white, size: 16),
            ],
          ),
        ),
      ),
    );
  }
}

/// Tarjeta con icono + título + descripción (panel derecho del login,
/// y reutilizable en cualquier grid de "features" a futuro).
class FeatureCard extends StatelessWidget {
  const FeatureCard({super.key, required this.icon, required this.title, required this.desc});

  final IconData icon;
  final String title;
  final String desc;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(22),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [
          BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 42,
            height: 42,
            decoration: BoxDecoration(
              color: AppColors.bg,
              border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: const [
                BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0),
              ],
            ),
            child: Icon(icon, color: AppColors.text, size: 20),
          ),
          const SizedBox(height: 14),
          Text(
            title,
            style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w900, color: AppColors.text),
          ),
          const SizedBox(height: 8),
          Text(
            desc,
            style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: AppColors.muted, height: 1.35),
          ),
        ],
      ),
    );
  }
}
