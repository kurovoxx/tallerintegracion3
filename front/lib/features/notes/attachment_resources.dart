/// Los enlaces de Drive abren un visor HTML, no contienen bytes de imagen.
bool isDriveViewerUrl(String url) {
  final uri = Uri.tryParse(url);
  return uri != null &&
      (uri.scheme == 'https' || uri.scheme == 'http') &&
      uri.host.toLowerCase() == 'drive.google.com';
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
