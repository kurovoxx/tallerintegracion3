import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'attachment_resources.dart';
import 'note_markdown_body.dart';
import 'notes_api.dart';

/// Abre un recurso (imagen o PDF) en el visor autenticado. Inyectable en
/// pruebas para no abrir diálogos ni visores reales.
typedef NoteResourceOpener =
    Future<void> Function(BuildContext context, Map<String, dynamic> resource);

/// Adjuntos reales de un apunte cargado con `GET /notes/{id}`, en el formato
/// que entiende [NoteMarkdownBody] (note_id + attachment_id para el endpoint
/// autenticado de contenido).
List<Map<String, dynamic>> noteDetailResources(NoteDetail note) =>
    attachmentResources(note.id, note.attachments);

/// Resuelve el recurso tocado en el Markdown contra los adjuntos reales para
/// que el visor use siempre el endpoint autenticado (note_id + attachment_id).
Map<String, dynamic> resolveNoteResource(
  List<Map<String, dynamic>> resources,
  Map<String, dynamic> resource,
) {
  if ((resource['attachment_id'] as String?)?.isNotEmpty ?? false) {
    return resource;
  }
  final url = (resource['url'] as String?) ?? '';
  for (final r in resources) {
    if (urlMatchesAttachment(r, url)) return r;
  }
  return resource;
}

/// Adjuntos que no son imágenes (PDF, documentos): se listan aparte porque
/// las imágenes ya se ven dentro del Markdown.
List<Map<String, dynamic>> noteDocumentResources(
  List<Map<String, dynamic>> resources,
) => resources.where((r) => r['type'] != 'image').toList(growable: false);

/// Contenido de un apunte de la API renderizado como Markdown.
///
/// Es el renderizado de la vista previa de apuntes de otros perfiles
/// («Ver apunte» desde el buscador de perfiles), extraído para que la búsqueda
/// por ID —a la que llevan «Ver» en Guardados y «Ver apunte» en Públicos—
/// muestre lo mismo en vez del texto crudo. Usa [NoteMarkdownBody] con los
/// adjuntos reales del apunte: las imágenes `attachment:<uuid>` se cargan por
/// el endpoint autenticado de Notes y no se modifica el contenido guardado.
class NoteContentView extends StatelessWidget {
  const NoteContentView({
    super.key,
    required this.note,
    required this.onOpenResource,
    this.imageBaseUrl,
    this.imageClient,
  });

  final NoteDetail note;

  /// Recibe el recurso ya resuelto contra los adjuntos reales.
  final ValueChanged<Map<String, dynamic>> onOpenResource;

  /// Base del microservicio de notas (endpoint autenticado de adjuntos).
  final String? imageBaseUrl;

  /// Cliente HTTP inyectable para las imágenes autenticadas en pruebas.
  final http.Client? imageClient;

  @override
  Widget build(BuildContext context) {
    if (note.content.trim().isEmpty) {
      return Text(
        '(Apunte sin contenido)',
        style: TextStyle(
          fontSize: 13,
          fontWeight: FontWeight.w700,
          color: AppColors.muted,
        ),
      );
    }
    final resources = noteDetailResources(note);
    return NoteMarkdownBody(
      content: note.content,
      resources: resources,
      imageBaseUrl: imageBaseUrl,
      imageClient: imageClient,
      onOpenResource: (r) => onOpenResource(resolveNoteResource(resources, r)),
    );
  }
}

/// Botones de los adjuntos que no son imágenes (PDF y documentos).
class NoteDocumentList extends StatelessWidget {
  const NoteDocumentList({
    super.key,
    required this.documents,
    required this.onOpen,
  });

  final List<Map<String, dynamic>> documents;
  final ValueChanged<Map<String, dynamic>> onOpen;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        for (final doc in documents)
          Padding(
            padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
            child: NeobrutalistButton(
              key: Key('note-preview-attachment-${doc['attachment_id']}'),
              label: (doc['name'] as String?) ?? 'Adjunto',
              icon: doc['type'] == 'pdf'
                  ? Icons.picture_as_pdf_rounded
                  : Icons.description_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              expand: true,
              semanticLabel: doc['type'] == 'pdf'
                  ? 'Ver PDF ${(doc['name'] as String?) ?? 'Adjunto'}'
                  : 'Abrir ${(doc['name'] as String?) ?? 'Adjunto'}',
              onPressed: () => onOpen(doc),
            ),
          ),
      ],
    );
  }
}
