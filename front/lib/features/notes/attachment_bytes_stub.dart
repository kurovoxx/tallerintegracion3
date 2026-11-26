import 'dart:typed_data';

/// Fallback para plataformas sin implementación de archivos (no ocurre en las
/// plataformas soportadas: io para escritorio/móvil, html para web).
Future<bool> openAttachmentBytes(
  Uint8List bytes,
  String fileName,
  String mimeType,
) async => false;

Future<String?> saveAttachmentBytes(
  Uint8List bytes,
  String fileName,
  String mimeType,
) async => null;
