import 'dart:typed_data';

import 'package:http/http.dart' as http;

import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';

/// Error de descarga de un adjunto por el endpoint autenticado de Notes.
///
/// Nunca incluye el token ni URLs de Drive del autor: solo el código HTTP y un
/// mensaje apto para mostrar al usuario.
class AttachmentContentException implements Exception {
  const AttachmentContentException(this.message, {this.statusCode});

  final String message;
  final int? statusCode;

  @override
  String toString() => message;
}

/// Bytes y MIME de un adjunto obtenidos con la sesión del usuario actual.
class AttachmentContent {
  const AttachmentContent({
    required this.bytes,
    required this.mimeType,
    required this.fileName,
  });

  final Uint8List bytes;
  final String mimeType;
  final String fileName;

  bool get isPdf => mimeType == 'application/pdf';
}

/// Límite del backend para adjuntos (10 MB). Se replica aquí para no descargar
/// respuestas anómalas en memoria.
const int maxAttachmentBytes = 10 * 1024 * 1024;

final RegExp _uuidPattern = RegExp(
  r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
);

/// URL del endpoint autenticado de contenido de adjuntos.
///
/// Es la única vía admitida para documentos de una nota: respeta la
/// autorización del backend y no depende de que el archivo de Drive del autor
/// siga compartido.
Uri attachmentContentUri({
  required String baseUrl,
  required String noteId,
  required String attachmentId,
}) {
  final base = baseUrl.trim().replaceFirst(RegExp(r'/+$'), '');
  return Uri.parse(
    '$base/notes/${Uri.encodeComponent(noteId.trim())}'
    '/attachments/${Uri.encodeComponent(attachmentId.trim())}/content',
  );
}

/// Nombre seguro para escribir el adjunto en disco o descargarlo en web.
String safeAttachmentFileName(String name, {String? mimeType}) {
  var base = name.trim().replaceAll(RegExp(r'[\\/:*?"<>|\x00-\x1F]'), '_');
  if (base.isEmpty || base == '.' || base == '..') base = 'adjunto';
  final lower = base.toLowerCase();
  final hasExtension = RegExp(r'\.[a-z0-9]{2,5}$').hasMatch(lower);
  if (hasExtension || mimeType == null) return base;
  switch (mimeType.toLowerCase()) {
    case 'application/pdf':
      return '$base.pdf';
    case 'image/png':
      return '$base.png';
    case 'image/jpeg':
      return '$base.jpg';
    case 'image/gif':
      return '$base.gif';
    case 'image/webp':
      return '$base.webp';
    default:
      return base;
  }
}

/// Descarga un adjunto mediante
/// `GET {baseUrl}/notes/{noteId}/attachments/{attachmentId}/content`.
///
/// - Usa `Authorization: Bearer <access token>` y el reintento/refresh de
///   [AuthedHttp], igual que [AuthenticatedAttachmentImage].
/// - Valida UUIDs antes de construir la URL.
/// - No sigue enlaces de Drive ni usa URLs públicas del autor.
Future<AttachmentContent> fetchAttachmentContent({
  required String baseUrl,
  required String noteId,
  required String attachmentId,
  required String fileName,
  http.Client? client,
}) async {
  final note = noteId.trim();
  final attachment = attachmentId.trim();
  if (!_uuidPattern.hasMatch(note) || !_uuidPattern.hasMatch(attachment)) {
    throw const AttachmentContentException('Referencia de adjunto inválida.');
  }
  final uri = attachmentContentUri(
    baseUrl: baseUrl,
    noteId: note,
    attachmentId: attachment,
  );
  final httpClient = client ?? http.Client();
  try {
    final response = await AuthedHttp.run(() async {
      final token = SessionManager.token;
      if (token == null || token.isEmpty) {
        throw const AttachmentContentException('Sesión no disponible.');
      }
      return httpClient
          .get(uri, headers: <String, String>{'Authorization': 'Bearer $token'})
          .timeout(const Duration(seconds: 30));
    });
    if (response.statusCode != 200) {
      throw AttachmentContentException(
        response.statusCode == 403 || response.statusCode == 404
            ? 'No tienes acceso a este adjunto.'
            : 'No se pudo descargar el adjunto (${response.statusCode}).',
        statusCode: response.statusCode,
      );
    }
    final bytes = response.bodyBytes;
    if (bytes.isEmpty) {
      throw const AttachmentContentException('El adjunto está vacío.');
    }
    if (bytes.length > maxAttachmentBytes) {
      throw const AttachmentContentException(
        'El adjunto supera el límite de 10 MB.',
      );
    }
    final rawMime = (response.headers['content-type'] ?? '')
        .split(';')
        .first
        .trim()
        .toLowerCase();
    final mime = rawMime.isEmpty ? 'application/octet-stream' : rawMime;
    return AttachmentContent(
      bytes: bytes,
      mimeType: mime,
      fileName: safeAttachmentFileName(fileName, mimeType: mime),
    );
  } on AttachmentContentException {
    rethrow;
  } catch (_) {
    throw const AttachmentContentException(
      'No se pudo descargar el adjunto. Inténtalo de nuevo.',
    );
  } finally {
    if (client == null) httpClient.close();
  }
}
