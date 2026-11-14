import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'all_notes_screen.dart' show showNoteResourceViewer;
import 'attachment_resources.dart';
import 'note_interaction_bar.dart';
import 'note_markdown_body.dart';
import 'notes_api.dart';

/// Abre un recurso (imagen o PDF) en el visor autenticado. Inyectable en
/// pruebas para no abrir diálogos ni visores reales.
typedef NoteResourceOpener =
    Future<void> Function(BuildContext context, Map<String, dynamic> resource);

/// Vista previa de un apunte de otro perfil («Ver apunte»).
///
/// Es una pantalla distinta de la lista: carga el apunte completo con
/// `GET /notes/{id}` (el backend aplica los permisos), renderiza el Markdown
/// con [NoteMarkdownBody] —imágenes por el endpoint autenticado de adjuntos—,
/// lista los PDF y documentos adjuntos para verlos en el visor y permite
/// copiar el apunte a «Mis apuntes» con sus adjuntos.
class PublicNotePreviewScreen extends StatefulWidget {
  const PublicNotePreviewScreen({
    super.key,
    required this.noteId,
    this.initialTitle,
    this.api,
    this.imageClient,
    this.openResource,
    this.onNoteCopied,
  });

  final String noteId;

  /// Título conocido por la lista, mostrado mientras carga el apunte.
  final String? initialTitle;

  /// Cliente inyectable (tests / base URL de Android). Por defecto NotesApi().
  final NotesApi? api;

  /// Cliente HTTP inyectable para las imágenes autenticadas en pruebas.
  final http.Client? imageClient;

  /// Abridor de recursos inyectable; por defecto [showNoteResourceViewer].
  final NoteResourceOpener? openResource;

  /// Se llama tras copiar con éxito (p. ej. para refrescar «Mis apuntes»).
  final VoidCallback? onNoteCopied;

  @override
  State<PublicNotePreviewScreen> createState() =>
      _PublicNotePreviewScreenState();
}

class _PublicNotePreviewScreenState extends State<PublicNotePreviewScreen> {
  late final NotesApi _api = widget.api ?? NotesApi();

  /// Una clave por apertura de la vista previa: si la primera petición se
  /// pierde por red, el reintento devuelve el mismo clon en vez de otro.
  late final String _copyKey =
      'copy-${widget.noteId}-${DateTime.now().microsecondsSinceEpoch}';

  bool _loading = true;
  NoteDetail? _note;
  NotesApiException? _error;
  bool _copying = false;
  bool _copied = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    if (widget.api == null) _api.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final note = await _api.getNote(widget.noteId);
      if (!mounted) return;
      setState(() {
        _note = note;
        _loading = false;
      });
    } on NotesApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _error = NotesApiException(notesFriendlyMessage(-1, null));
        _loading = false;
      });
    }
  }

  List<Map<String, dynamic>> get _resources {
    final note = _note;
    if (note == null) return const [];
    return attachmentResources(note.id, note.attachments);
  }

  /// Resuelve el recurso tocado en el Markdown contra los adjuntos reales para
  /// que el visor use siempre el endpoint autenticado (note_id + attachment_id).
  Map<String, dynamic> _resolve(Map<String, dynamic> resource) {
    if ((resource['attachment_id'] as String?)?.isNotEmpty ?? false) {
      return resource;
    }
    final url = (resource['url'] as String?) ?? '';
    for (final r in _resources) {
      if (urlMatchesAttachment(r, url)) return r;
    }
    return resource;
  }

  Future<void> _open(Map<String, dynamic> resource) {
    final opener = widget.openResource ?? showNoteResourceViewer;
    return opener(context, _resolve(resource));
  }

  void _snack(String message, {bool error = false}) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: error ? AppColors.errorDeep : AppColors.border,
        content: Text(
          message,
          style: const TextStyle(
            color: AppColors.surface,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  Future<void> _copy(NoteDetail note) async {
    if (_copying || _copied) return;
    setState(() => _copying = true);
    try {
      await _api.copyNote(note.id, idempotencyKey: _copyKey);
      if (!mounted) return;
      setState(() => _copied = true);
      widget.onNoteCopied?.call();
      _snack('Copia guardada en tus apuntes y en tu Drive.');
    } on NotesApiException catch (e) {
      if (!mounted) return;
      if (e.code == 'already_copied') setState(() => _copied = true);
      _snack(e.message, error: e.code != 'already_copied');
    } catch (_) {
      _snack(notesFriendlyMessage(-1, null), error: true);
    } finally {
      if (mounted) setState(() => _copying = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final title = _note?.title ?? widget.initialTitle ?? 'Apunte';
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: MaxWidthContainer(
          maxWidth: 860,
          padding: EdgeInsets.zero,
          child: ListView(
            padding: const EdgeInsets.all(AppDimens.gutter),
            children: <Widget>[
              Row(
                children: <Widget>[
                  NeobrutalistIconButton(
                    icon: Icons.arrow_back_rounded,
                    tooltip: 'Volver',
                    onPressed: () => Navigator.of(context).maybePop(),
                  ),
                  const SizedBox(width: AppDimens.spaceMd),
                  Expanded(
                    child: Text(
                      title,
                      key: const Key('note-preview-title'),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w900,
                        color: AppColors.text,
                        letterSpacing: -0.5,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: AppDimens.spaceLg),
              _buildBody(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.all(AppDimens.spaceXl),
        child: Center(child: CircularProgressIndicator(color: AppColors.text)),
      );
    }
    final error = _error;
    if (error != null) return _buildError(error);
    final note = _note!;
    final resources = _resources;
    final documents = resources
        .where((r) => r['type'] != 'image')
        .toList(growable: false);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        NeobrutalistCard(
          key: const Key('note-preview-content'),
          child: note.content.trim().isEmpty
              ? const Text(
                  '(Apunte sin contenido)',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: AppColors.muted,
                  ),
                )
              : NoteMarkdownBody(
                  content: note.content,
                  resources: resources,
                  imageBaseUrl: _api.baseUrl,
                  imageClient: widget.imageClient,
                  onOpenResource: _open,
                ),
        ),
        if (documents.isNotEmpty) ...<Widget>[
          const SizedBox(height: AppDimens.spaceLg),
          NeobrutalistCard(
            title: 'Adjuntos',
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: <Widget>[
                for (final doc in documents) _buildDocumentTile(doc),
              ],
            ),
          ),
        ],
        const SizedBox(height: AppDimens.spaceLg),
        Wrap(
          spacing: AppDimens.spaceMd,
          runSpacing: AppDimens.spaceMd,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: <Widget>[
            NoteLikeSaveBar(
              api: _api,
              initial: NoteInteractions(
                noteId: note.id,
                likesCount: note.likesCount,
                savesCount: note.savesCount,
                isLiked: note.isLiked,
                isSaved: note.isSaved,
              ),
              onChanged: (s) => setState(
                () => _note = note.copyWith(
                  likesCount: s.likesCount,
                  savesCount: s.savesCount,
                  isLiked: s.isLiked,
                  isSaved: s.isSaved,
                ),
              ),
            ),
            if (!note.isOwn)
              NeobrutalistButton(
                key: const Key('note-preview-copy'),
                label: _copied
                    ? 'Copiado en tus apuntes'
                    : (_copying ? 'Copiando…' : 'Copiar a mis apuntes'),
                icon: _copied
                    ? Icons.check_rounded
                    : Icons.content_copy_rounded,
                variant: NeobrutalistButtonVariant.accent,
                onPressed: (_copying || _copied) ? null : () => _copy(note),
              ),
          ],
        ),
      ],
    );
  }

  Widget _buildDocumentTile(Map<String, dynamic> doc) {
    final isPdf = doc['type'] == 'pdf';
    final name = (doc['name'] as String?) ?? 'Adjunto';
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: NeobrutalistButton(
        key: Key('note-preview-attachment-${doc['attachment_id']}'),
        label: name,
        icon: isPdf ? Icons.picture_as_pdf_rounded : Icons.description_rounded,
        variant: NeobrutalistButtonVariant.secondary,
        expand: true,
        semanticLabel: isPdf ? 'Ver PDF $name' : 'Abrir $name',
        onPressed: () => _open(doc),
      ),
    );
  }

  Widget _buildError(NotesApiException e) {
    final deleted = e.statusCode == 410 || e.code == 'note_deleted';
    final notFound = e.statusCode == 404 && e.code != 'note_unavailable';
    final title = deleted
        ? 'Apunte eliminado'
        : (notFound ? 'Apunte no disponible' : 'No se pudo abrir el apunte');
    final message = deleted
        ? e.message
        : (notFound
              ? 'Este apunte ya no es público o no tienes permiso para verlo.'
              : e.message);
    return NeobrutalistCard(
      key: const Key('note-preview-error'),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text(
            title,
            style: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Text(
            message,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: AppColors.muted,
            ),
          ),
          if (!deleted && !notFound) ...<Widget>[
            const SizedBox(height: AppDimens.spaceMd),
            NeobrutalistButton(
              label: 'Reintentar',
              icon: Icons.refresh_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: _load,
            ),
          ],
        ],
      ),
    );
  }
}
