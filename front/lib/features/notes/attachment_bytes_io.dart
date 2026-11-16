import 'dart:io';
import 'dart:typed_data';

import 'package:path_provider/path_provider.dart';
import 'package:url_launcher/url_launcher.dart';

import 'attachment_content.dart' show safeAttachmentFileName;

String _safeName(String fileName, String mimeType) =>
    safeAttachmentFileName(fileName, mimeType: mimeType);

/// Escribe el adjunto en un archivo temporal estable y lo abre con el visor
/// predeterminado del sistema. Devuelve true solo si el lanzamiento funcionó.
Future<bool> openAttachmentBytes(
  Uint8List bytes,
  String fileName,
  String mimeType,
) async {
  try {
    final dir = await getTemporaryDirectory();
    final folder = Directory(
      '${dir.path}${Platform.pathSeparator}sigma_attachments',
    );
    if (!folder.existsSync()) folder.createSync(recursive: true);
    final file = File(
      '${folder.path}${Platform.pathSeparator}'
      '${_safeName(fileName, mimeType)}',
    );
    await file.writeAsBytes(bytes, flush: true);
    if (Platform.isWindows) {
      // En pruebas automatizadas no se abren visores externos reales.
      if (Platform.environment.containsKey('FLUTTER_TEST')) return true;
      final result = await Process.run('cmd', <String>[
        '/c',
        'start',
        '',
        file.absolute.path,
      ]);
      return result.exitCode == 0;
    }
    return await launchUrl(
      Uri.file(file.absolute.path),
      mode: LaunchMode.externalApplication,
    );
  } catch (_) {
    return false;
  }
}

/// Copia el adjunto a Descargas (o a Documentos si la plataforma no expone
/// Descargas) sin sobrescribir archivos existentes. Devuelve la ruta final.
Future<String?> saveAttachmentBytes(
  Uint8List bytes,
  String fileName,
  String mimeType,
) async {
  try {
    final dir =
        await getDownloadsDirectory() ??
        await getApplicationDocumentsDirectory();
    if (dir.path.isEmpty) return null;
    final name = _safeName(fileName, mimeType);
    final dot = name.lastIndexOf('.');
    final stem = dot > 0 ? name.substring(0, dot) : name;
    final extension = dot > 0 ? name.substring(dot) : '';
    var target = File('${dir.path}${Platform.pathSeparator}$name');
    var counter = 1;
    while (target.existsSync()) {
      target = File(
        '${dir.path}${Platform.pathSeparator}$stem ($counter)$extension',
      );
      counter++;
    }
    await target.writeAsBytes(bytes, flush: true);
    return target.path;
  } catch (_) {
    return null;
  }
}
