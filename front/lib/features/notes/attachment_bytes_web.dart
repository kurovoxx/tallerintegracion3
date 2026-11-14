// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:html' as html;
import 'dart:typed_data';

import 'attachment_content.dart' show safeAttachmentFileName;

/// Abre el PDF en una pestaña nueva usando un blob local con los bytes ya
/// autenticados. Nunca se expone el token ni una URL de Drive del autor.
Future<bool> openAttachmentBytes(
  Uint8List bytes,
  String fileName,
  String mimeType,
) async {
  try {
    final blob = html.Blob(<Object>[bytes], mimeType);
    final url = html.Url.createObjectUrlFromBlob(blob);
    html.window.open(url, '_blank');
    return true;
  } catch (_) {
    return false;
  }
}

/// Descarga el adjunto como archivo en el navegador.
Future<String?> saveAttachmentBytes(
  Uint8List bytes,
  String fileName,
  String mimeType,
) async {
  try {
    final name = safeAttachmentFileName(fileName, mimeType: mimeType);
    final blob = html.Blob(<Object>[bytes], mimeType);
    final url = html.Url.createObjectUrlFromBlob(blob);
    _downloadBlob(url, name);
    return name;
  } catch (_) {
    return null;
  }
}

void _downloadBlob(String url, String fileName) {
  final anchor = html.AnchorElement(href: url)
    ..download = fileName
    ..style.display = 'none';
  html.document.body?.children.add(anchor);
  anchor.click();
  anchor.remove();
  html.Url.revokeObjectUrl(url);
}
