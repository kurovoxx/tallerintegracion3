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
        };
      })
      .toList();
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
    final path = (Uri.tryParse(url)?.path ?? url).toLowerCase();
    final pdf = path.endsWith('.pdf') || label.toLowerCase().endsWith('.pdf');
    final document =
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
