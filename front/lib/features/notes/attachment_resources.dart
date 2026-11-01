/// Los enlaces de Drive abren un visor HTML, no contienen bytes de imagen.
bool isDriveViewerUrl(String url) {
  final uri = Uri.tryParse(url);
  return uri != null &&
      (uri.scheme == 'https' || uri.scheme == 'http') &&
      uri.host.toLowerCase() == 'drive.google.com';
}

String resourceIdentity(String url) {
  final uri = Uri.tryParse(url);
  if (isDriveViewerUrl(url) && uri != null) {
    final parts = uri.pathSegments;
    final index = parts.indexOf('d');
    if (index >= 0 && index + 1 < parts.length) {
      return 'drive:${parts[index + 1]}';
    }
    final id = uri.queryParameters['id'];
    if (id != null) return 'drive:$id';
  }
  return url;
}

List<Map<String, dynamic>> attachmentResources(
  String noteId,
  List<dynamic> attachments,
) {
  return attachments
      .whereType<Map>()
      .where((a) => a['id'] is String && a['note_id'] == noteId)
      .map((a) {
        final mime = (a['file_type'] as String? ?? '').toLowerCase();
        final type = mime.startsWith('image/')
            ? 'image'
            : (mime == 'application/pdf' ? 'pdf' : 'doc');
        return <String, dynamic>{
          'type': type,
          'name': a['file_name'] ?? 'Adjunto',
          'url':
              a['file_url'] ??
              'https://drive.google.com/file/d/${a['external_file_id']}/view',
          'size': type == 'pdf' ? 'PDF' : 'Adjunto',
          'note_id': noteId,
          'attachment_id': a['id'],
          'external_file_id': a['external_file_id'],
        };
      })
      .toList();
}

/// Extrae el UUID real de una referencia `attachment:<uuid>`.
/// Devuelve '' si no es una referencia inline válida (pending, vacía, etc).
String attachmentIdFromUrl(String url) {
  final trimmed = url.trim();
  if (!trimmed.startsWith('attachment:')) return '';
  if (trimmed.startsWith('attachment-pending://')) return '';
  final id = trimmed.substring('attachment:'.length).trim();
  // UUID real del backend (36 con guiones). Nunca temporal ni external_file_id.
  if (RegExp(
    r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
  ).hasMatch(id)) {
    return id;
  }
  return '';
}

/// Clasifica cada referencia una sola vez, incluyendo enlaces sin extensión.
/// La marca ! distingue una imagen de un documento con la misma clase de URL.
List<Map<String, dynamic>> resourcesFromMarkdown(String content) {
  final result = <Map<String, dynamic>>[];
  final seen = <String>{};
  final references = RegExp(r'(!?)\[([^\]]*)\]\(([^)]+)\)');
  for (final match in references.allMatches(content)) {
    final image = match.group(1) == '!';
    final label = match.group(2)!.trim();
    final url = match.group(3)!.trim();
    if (url.isEmpty) continue;
    // Preview en diálogos muestra el pending como chip temporal;
    // nunca se persiste (se filtra con _withoutPendingAttachments) ni se
    // convierte en tarjeta de RECURSOS ADJUNTOS (merge lo ignora).
    final path = (Uri.tryParse(url)?.path ?? url).toLowerCase();
    final pdf = path.endsWith('.pdf') || label.toLowerCase().endsWith('.pdf');
    final document =
        url.startsWith('attachment:') ||
        url.startsWith('attachment-pending://') ||
        pdf ||
        isDriveViewerUrl(url) ||
        RegExp(r'\.(docx?|odt)$').hasMatch(path);
    if (!image && !document) continue;
    final type = image ? 'image' : (pdf ? 'pdf' : 'doc');
    if (!seen.add('$type::$url')) continue;
    result.add({
      'type': type,
      'name': label.isEmpty ? url.split('/').last : label,
      'url': url,
      'size': image ? 'Adjunto' : (pdf ? 'PDF' : 'Documento'),
    });
  }
  return result;
}

bool urlMatchesAttachment(Map<String, dynamic> resource, String url) {
  final attachmentId = resource['attachment_id'] as String?;
  if (attachmentId != null && attachmentId.isNotEmpty) {
    // Comparación exacta por UUID real: la única fuente confiable.
    if (url.trim() == 'attachment:$attachmentId') return true;
  }
  final resourceUrl = resource['url'] as String? ?? '';
  if (resourceUrl.isEmpty || url.trim().isEmpty) return false;
  // Fallback por identidad Drive (misma file id aunque el formato cambie).
  return resourceIdentity(url.trim()) == resourceIdentity(resourceUrl);
}

/// Fusiona adjuntos reales + referencias Markdown deduplicando por
/// attachment.id real. 1 attachment ID = 1 tarjeta en RECURSOS ADJUNTOS.
/// Las referencias `attachment:<uuid>` que resuelven a un adjunto real se
/// reemplazan por ese adjunto; las que no resuelven (stale) se excluyen de
/// recursos para no duplicar una tarjeta genérica junto a la real.
List<Map<String, dynamic>> mergeAttachmentResources(
  List<Map<String, dynamic>> attachments,
  List<Map<String, dynamic>> markdownResources,
) {
  final byId = <String, Map<String, dynamic>>{};
  for (final a in attachments) {
    final id = a['attachment_id'] as String?;
    if (id != null && id.isNotEmpty) byId[id] = a;
  }
  final out = <Map<String, dynamic>>[];
  final seenIds = <String>{};
  final seenUrls = <String>{};
  for (final a in attachments) {
    final id = a['attachment_id'] as String?;
    if (id != null && id.isNotEmpty) {
      if (seenIds.add(id)) out.add(a);
    } else {
      final key = resourceIdentity(a['url'] as String? ?? '');
      if (seenUrls.add(key)) out.add(a);
    }
  }
  for (final r in markdownResources) {
    final url = (r['url'] ?? '') as String;
    // Placeholders temporales nunca generan tarjeta en RECURSOS ADJUNTOS.
    if (url.startsWith('attachment-pending://')) continue;
    final inlineId = attachmentIdFromUrl(url);
    if (inlineId.isNotEmpty) {
      // Referencia inline con UUID real.
      if (byId.containsKey(inlineId)) {
        // Ya representada por la tarjeta real: no duplicar.
        continue;
      }
      // Stale (apunta a un adjunto que ya no existe): no crear tarjeta
      // genérica en Recursos Adjuntos; el cuerpo muestra fallback compacto.
      continue;
    }
    // Documento/imagen por URL Drive o http directa: deduplicar por identidad.
    final matches = attachments.where((a) => urlMatchesAttachment(a, url));
    if (matches.isNotEmpty) continue; // Ya cubierta por tarjeta real.
    final key = resourceIdentity(url);
    if (seenUrls.add(key)) out.add(r);
  }
  return out;
}

String removeAttachmentReferences(
  String content,
  Map<String, dynamic> resource,
) => content.replaceAllMapped(
  RegExp(r'!?\[[^\]]*\]\(([^)]+)\)'),
  (match) => urlMatchesAttachment(resource, match.group(1)!.trim())
      ? ''
      : match.group(0)!,
);
