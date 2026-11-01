import 'dart:typed_data';
import 'package:file_selector/file_selector.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/notes/note_file_picker.dart';

void main() {
  tearDown(() => NoteFilePicker.pickOverride = null);
  for (final entry in {
    'foto.jpg': [255, 216, 255, 224],
    'foto.jpeg': [255, 216, 255, 224],
    'foto.png': [137, 80, 78, 71, 13, 10, 26, 10],
    'guia.pdf': [37, 80, 68, 70, 45, 49],
  }.entries) {
    test('selector ${entry.key} obtiene bytes y nombre sin ruta', () async {
      NoteFilePicker.pickOverride = (_) async => XFile.fromData(
        Uint8List.fromList(entry.value),
        path: 'C:\\Users\\Persona\\${entry.key}',
      );
      final image = !entry.key.endsWith('.pdf');
      final result = await NoteFilePicker.pick(image: image);
      expect(result!.name, entry.key);
      expect(result.bytes, entry.value);
      expect(result.isImage, image);
      expect(result.alt, entry.key.split('.').first);
    });
  }
  test('cancelar no crea adjunto', () async {
    NoteFilePicker.pickOverride = (_) async => null;
    expect(await NoteFilePicker.pick(image: true), isNull);
  });
  test('PDF no pasa por imagen; imagen no pasa por PDF', () {
    expect(
      () => NoteFilePicker.validate(
        'a.pdf',
        Uint8List.fromList([37, 80, 68, 70, 45]),
        image: true,
      ),
      throwsFormatException,
    );
    expect(
      () => NoteFilePicker.validate(
        'a.jpg',
        Uint8List.fromList([255, 216, 255]),
        image: false,
      ),
      throwsFormatException,
    );
  });
  test('límite 10 MB y MIME real invalidan archivos', () {
    expect(
      () => NoteFilePicker.validate(
        'a.jpg',
        Uint8List(NoteFilePicker.maxBytes + 1),
        image: true,
      ),
      throwsFormatException,
    );
    expect(
      () => NoteFilePicker.validate(
        'a.jpg',
        Uint8List.fromList([1, 2, 3, 4]),
        image: true,
      ),
      throwsFormatException,
    );
    expect(
      () => NoteFilePicker.validate(
        'a.png',
        Uint8List.fromList([255, 216, 255]),
        image: true,
      ),
      throwsFormatException,
    );
  });
}
