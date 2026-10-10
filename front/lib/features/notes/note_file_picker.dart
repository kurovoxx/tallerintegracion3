import 'dart:typed_data';
import 'package:file_selector/file_selector.dart';

class PickedNoteFile {
  const PickedNoteFile(this.name, this.bytes, this.mime);
  final String name, mime;
  final Uint8List bytes;
  bool get isImage => mime.startsWith('image/');
  String get alt => name.replaceFirst(RegExp(r'\.[^.]+$'), '');
  String get sizeLabel =>
      '${(bytes.length / (1024 * 1024)).toStringAsFixed(2)} MB';
}

class NoteFilePicker {
  static const maxBytes = 10 * 1024 * 1024;
  static Future<XFile?> Function(bool image)? pickOverride;
  static Future<PickedNoteFile?> pick({required bool image}) async {
    final file =
        await (pickOverride?.call(image) ??
            openFile(
              acceptedTypeGroups: [
                image
                    ? const XTypeGroup(
                        label: 'Imagen',
                        extensions: ['png', 'jpg', 'jpeg'],
                        mimeTypes: ['image/png', 'image/jpeg'],
                        uniformTypeIdentifiers: ['public.png', 'public.jpeg'],
                      )
                    : const XTypeGroup(
                        label: 'PDF',
                        extensions: ['pdf'],
                        mimeTypes: ['application/pdf'],
                        uniformTypeIdentifiers: ['com.adobe.pdf'],
                      ),
              ],
            ));
    if (file == null) return null;
    if (await file.length() > maxBytes) {
      throw const FormatException('El archivo supera el límite de 10 MB.');
    }
    final bytes = await file.readAsBytes();
    return validate(file.name, bytes, image: image);
  }

  static PickedNoteFile validate(
    String name,
    Uint8List bytes, {
    required bool image,
  }) {
    if (bytes.length > maxBytes) {
      throw const FormatException('El archivo supera el límite de 10 MB.');
    }
    final safeName = name.replaceAll('\\', '/').split('/').last;
    final lower = safeName.toLowerCase();
    bool starts(List<int> signature) =>
        bytes.length >= signature.length &&
        List.generate(
          signature.length,
          (i) => bytes[i] == signature[i],
        ).every((v) => v);
    final mime = starts([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])
        ? 'image/png'
        : starts([0xff, 0xd8, 0xff])
        ? 'image/jpeg'
        : starts([0x25, 0x50, 0x44, 0x46, 0x2d])
        ? 'application/pdf'
        : null;
    if (mime == null) {
      throw const FormatException(
        'El contenido del archivo no es una imagen o PDF válido.',
      );
    }
    if (image && !mime.startsWith('image/')) {
      throw const FormatException(
        'Selecciona una imagen PNG o JPG. El PDF se adjunta desde PDF.',
      );
    }
    if (!image && mime != 'application/pdf') {
      throw const FormatException('Selecciona un archivo PDF.');
    }
    final validExtension = mime == 'image/png'
        ? lower.endsWith('.png')
        : mime == 'image/jpeg'
        ? (lower.endsWith('.jpg') || lower.endsWith('.jpeg'))
        : lower.endsWith('.pdf');
    if (!validExtension) {
      throw const FormatException(
        'La extensión no coincide con el contenido del archivo.',
      );
    }
    return PickedNoteFile(safeName, bytes, mime);
  }
}
