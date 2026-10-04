import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/notes/attachment_resources.dart';

void main() {
  for (final extension in ['png', 'jpg']) {
    test('$extension local y Drive: una imagen y cero documentos', () {
      for (final url in [
        'foto.$extension',
        'https://drive.google.com/file/d/private/view?usp=drivesdk',
      ]) {
        final resources = resourcesFromMarkdown('![Foto]($url)');
        expect(resources, hasLength(1));
        expect(resources.single['type'], 'image');
        expect(resources.single['name'], 'Foto');
      }
    });
  }
  test('PDF local y Drive: cero imágenes y un documento', () {
    for (final url in [
      'guia.pdf',
      'https://drive.google.com/file/d/pdf/view',
    ]) {
      final resources = resourcesFromMarkdown('[guia.pdf]($url)');
      expect(resources, hasLength(1));
      expect(resources.single['type'], 'pdf');
      expect(resources.single['name'], 'guia.pdf');
    }
  });
  test('imagen y PDF explícitos conservan recursos independientes', () {
    final resources = resourcesFromMarkdown(
      '![Foto](https://drive.google.com/file/d/image/view)\n'
      '[guia.pdf](https://drive.google.com/file/d/document/view)',
    );
    expect(resources.map((r) => r['type']), ['image', 'pdf']);
  });
  test('documentos sin tipo conocido no reciben extensión PDF inventada', () {
    final resources = resourcesFromMarkdown(
      '[Informe](https://drive.google.com/file/d/doc/view)',
    );
    expect(resources.single['type'], 'doc');
    expect(resources.single['name'], 'Informe');
  });
  test('DOCX y PDF con query conservan nombre y clasificación', () {
    expect(
      resourcesFromMarkdown('[Informe.docx](informe.docx)').single['type'],
      'doc',
    );
    expect(
      resourcesFromMarkdown(
        '[Guía](https://example.com/guia.pdf?download=1)',
      ).single['type'],
      'pdf',
    );
  });
  test('repetir una imagen no crea un documento ni otro recurso', () {
    const image = '![Foto](https://drive.google.com/file/d/image/view)';
    expect(resourcesFromMarkdown('$image\n$image'), hasLength(1));
  });
  test('un attachment real + su ref inline => una sola tarjeta', () {
    const id = 'abcdefab-1234-4234-8234-123456789abc';
    final merged = mergeAttachmentResources(
      [
        {
          'attachment_id': id,
          'type': 'image',
          'name': 'foto.png',
          'url': 'https://drive.google.com/file/d/ext/view',
        },
      ],
      [
        {
          'type': 'image',
          'name': 'foto',
          'url': 'attachment:$id',
          'size': 'Adjunto',
        },
      ],
    );
    expect(merged, hasLength(1));
    expect(merged.single['attachment_id'], id);
  });
  test('dos attachments distintos con mismo filename => dos tarjetas', () {
    Map<String, dynamic> att(String id) => {
      'attachment_id': id,
      'type': 'image',
      'name': 'foto.png',
      'url': 'https://drive.google.com/file/d/$id/view',
    };
    final merged = mergeAttachmentResources(
      [att('aaaaaaaa-1234-4234-8234-123456789abc')],
      [],
    );
    final merged2 = mergeAttachmentResources(
      [
        att('aaaaaaaa-1234-4234-8234-123456789abc'),
        att('bbbbbbbb-1234-4234-8234-123456789abc'),
      ],
      [],
    );
    expect(merged, hasLength(1));
    // Nunca se colapsan dos IDs distintos aunque compartan filename.
    expect(merged2.map((r) => r['attachment_id']).toSet(), hasLength(2));
  });
  test('mismo attachment_id duplicado en fuentes => una tarjeta', () {
    const id = 'abcdefab-1234-4234-8234-123456789abc';
    Map<String, dynamic> att() => {
      'attachment_id': id,
      'type': 'pdf',
      'name': 'guia.pdf',
      'url': 'https://drive.google.com/file/d/ext/view',
    };
    final merged = mergeAttachmentResources(
      [att(), att()],
      [
        {
          'type': 'pdf',
          'name': 'guia.pdf',
          'url': 'attachment:$id',
          'size': 'PDF',
        },
      ],
    );
    expect(merged, hasLength(1));
  });
  test('solo el dominio Drive se trata como visor', () {
    expect(isDriveViewerUrl('https://drive.google.com/file/d/id/view'), isTrue);
    expect(isDriveViewerUrl('https://example.com/foto.png'), isFalse);
    expect(
      isDriveViewerUrl('https://drive.google.com.example.com/foto.png'),
      isFalse,
    );
  });
}
