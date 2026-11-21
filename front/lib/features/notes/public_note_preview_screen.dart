import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'all_notes_screen.dart' show showNoteResourceViewer;
import 'note_content_view.dart';
import 'note_interaction_bar.dart';
import 'notes_api.dart';

export 'note_content_view.dart' show NoteResourceOpener;

/// Vista previa de un apunte de otro perfil («Ver apunte»).
///
/// Es una pantalla distinta de la lista: carga el apunte completo con
/// `GET /notes/{id}` (el backend aplica los permisos), renderiza el Markdown
/// con `NoteContentView` (`NoteMarkdownBody`) —imágenes por el endpoint autenticado de adjuntos—,
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
    this.onInteractionsChanged,
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

  /// Se llama con cada estado de like/guardado CONFIRMADO por el servidor,
  /// para que la lista de origen lo refleje al volver sin recargar.
  final ValueChanged<NoteInteractions>? onInteractionsChanged;

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
    return noteDetailResources(note);
  }

  Future<void> _open(Map<String, dynamic> resource) {
    final opener = widget.openResource ?? showNoteResourceViewer;
    return opener(context, resolveNoteResource(_resources, resource));
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
    final documents = noteDocumentResources(_resources);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        NeobrutalistCard(
          key: const Key('note-preview-content'),
          child: NoteContentView(
            note: note,
            imageBaseUrl: _api.baseUrl,
            imageClient: widget.imageClient,
            onOpenResource: _open,
          ),
        ),
        if (documents.isNotEmpty) ...<Widget>[
          const SizedBox(height: AppDimens.spaceLg),
          NeobrutalistCard(
            title: 'Adjuntos',
            child: NoteDocumentList(documents: documents, onOpen: _open),
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
              // El autor no puede guardar su propio apunte (el backend
              // también lo rechaza).
              showSave: !note.isOwn,
              onChanged: (s) {
                setState(
                  () => _note = note.copyWith(
                    likesCount: s.likesCount,
                    savesCount: s.savesCount,
                    isLiked: s.isLiked,
                    isSaved: s.isSaved,
                  ),
                );
                widget.onInteractionsChanged?.call(s);
              },
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
