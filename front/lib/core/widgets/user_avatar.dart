import 'dart:convert';

import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

/// Iniciales de un nombre o correo, sin mostrar un UUID desnudo.
String userInitials(String label, {String fallback = '?'}) {
  final clean = label.trim();
  if (clean.isEmpty) return fallback;
  final parts = clean.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
  if (parts.length >= 2) {
    return '${parts.first[0]}${parts.elementAt(1)[0]}'.toUpperCase();
  }
  final word = parts.first;
  final alnum = word.replaceAll(RegExp(r'[^A-Za-zÁÉÍÓÚÑáéíóúñ0-9]'), '');
  if (alnum.length >= 2) return alnum.substring(0, 2).toUpperCase();
  if (alnum.isNotEmpty) return alnum[0].toUpperCase();
  return clean.substring(0, clean.length >= 2 ? 2 : 1).toUpperCase();
}

/// Resuelve data URLs (foto recortada o miniatura) y URLs remotas.
ImageProvider<Object>? userAvatarProvider(String? rawUrl) {
  final url = rawUrl?.trim() ?? '';
  if (url.isEmpty) return null;
  const marker = ';base64,';
  if (url.startsWith('data:image/')) {
    final comma = url.indexOf(marker);
    if (comma <= 0) return null;
    try {
      return MemoryImage(base64Decode(url.substring(comma + marker.length)));
    } on FormatException {
      return null;
    }
  }
  return NetworkImage(url);
}

/// Avatar compartido para integrantes de grupos: foto/miniatura con fallback a
/// iniciales. Mantiene el estilo neobrutalista del resto de la app.
class UserAvatar extends StatelessWidget {
  const UserAvatar({
    super.key,
    required this.name,
    required this.size,
    this.photoUrl,
    this.backgroundColor = AppColors.accentYellow,
    this.foregroundColor = AppColors.text,
    this.borderRadius,
    this.border,
    this.boxShadow,
    this.fontSize,
  });

  final String name;
  final double size;
  final String? photoUrl;
  final Color backgroundColor;
  final Color foregroundColor;
  final BorderRadius? borderRadius;
  final BoxBorder? border;
  final List<BoxShadow>? boxShadow;
  final double? fontSize;

  @override
  Widget build(BuildContext context) {
    final provider = userAvatarProvider(photoUrl);
    return Container(
      width: size,
      height: size,
      clipBehavior: Clip.hardEdge,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: backgroundColor,
        border:
            border ??
            Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius:
            borderRadius ?? BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: boxShadow,
      ),
      child: provider == null
          ? _fallback()
          : Image(
              image: provider,
              fit: BoxFit.cover,
              width: size,
              height: size,
              errorBuilder: (_, _, _) => _fallback(),
            ),
    );
  }

  Widget _fallback() {
    return Center(
      child: Text(
        userInitials(name),
        maxLines: 1,
        textAlign: TextAlign.center,
        style: TextStyle(
          fontSize: fontSize ?? size * 0.34,
          fontWeight: FontWeight.w900,
          letterSpacing: 0.2,
          color: foregroundColor,
        ),
      ),
    );
  }
}
