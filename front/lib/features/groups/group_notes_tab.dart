import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/services/notes_sharing_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../notes/attachment_resources.dart';
import '../notes/authenticated_attachment_image.dart';

/// Pestaña "NOTAS" de la vista de grupo: lista y abre las notas compartidas
/// con el grupo, permite compartir una nota propia eligiendo modo
/// (link/restricted) y dejar de compartir. Diseño Neo-Brutalism y estados de
/// carga/error/vacío explícitos con refresco tras cada mutación.
class GroupNotesTab extends StatefulWidget {
  const GroupNotesTab({super.key, this.groupId, NotesSharingService? service})
    : _serviceOverride = service;

  final String? groupId;
  final NotesSharingService? _serviceOverride;

  @override
  State<GroupNotesTab> createState() => _GroupNotesTabState();
}

class _GroupNotesTabState extends State<GroupNotesTab> {
  late final NotesSharingService _service;

  bool _loading = false;
  String? _error;
  List<SharedGroupNote> _notes = const [];

  /// Vínculos creados en esta sesión: noteId -> sharedNoteId. Habilita
  /// "dejar de compartir" para lo que el usuario acaba de compartir (el
  /// listado del backend no expone el shared_note_id).
  final Map<String, String> _sharedByMe = {};

  String? _busyNoteId;

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? NotesSharingService();
    _load();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  String get _groupId => widget.groupId?.trim() ?? '';

  void _snack(String message, {bool error = false}) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: error ? AppColors.errorDeep : AppColors.border,
      ),
    );
  }

  Future<void> _load() async {
    final gid = _groupId;
    if (gid.isEmpty) {
      setState(() {
        _loading = false;
        _error = 'Grupo no disponible.';
        _notes = const [];
      });
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final notes = await _service.listGroupNotes(gid);
      if (!mounted) return;
      setState(() {
        _notes = notes;
        _loading = false;
      });
    } on NotesApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = e.message;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = 'No se pudieron cargar las notas del grupo.';
      });
    }
  }

  Future<void> _openNote(SharedGroupNote note) async {
    setState(() => _busyNoteId = note.noteId);
    SharedNoteContent content;
    try {
      content = await _service.getNoteContent(note.noteId);
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
      if (mounted) setState(() => _busyNoteId = null);
      return;
    } catch (_) {
      _snack('No se pudo abrir la nota.', error: true);
      if (mounted) setState(() => _busyNoteId = null);
      return;
    }
    if (mounted) setState(() => _busyNoteId = null);
    if (!mounted) return;
    await showNeobrutalistDialog<void>(
      context: context,
      dialog: NeobrutalistDialog(
        title: content.title,
        cancelLabel: 'Cerrar',
        maxWidth: 760,
        content: _NoteViewer(content: content),
      ),
    );
  }

  Future<void> _shareFlow() async {
    final gid = _groupId;
    if (gid.isEmpty) {
      _snack('Grupo no disponible.', error: true);
      return;
    }
    List<MyNoteSummary> mine;
    try {
      mine = await _service.listMyNotes();
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
      return;
    } catch (_) {
      _snack('No se pudieron cargar tus notas.', error: true);
      return;
    }
    if (!mounted) return;
    if (mine.isEmpty) {
      _snack('No tienes notas para compartir.', error: true);
      return;
    }
    final choice = await showNeobrutalistDialog<_ShareChoice>(
      context: context,
      dialog: _ShareNoteDialog(notes: mine),
    );
    if (choice == null || !mounted) return;
    try {
      final sharedId = await _service.shareNote(
        noteId: choice.noteId,
        groupId: gid,
        accessMode: choice.accessMode,
      );
      _sharedByMe[choice.noteId] = sharedId;
      if (!mounted) return;
      await _load();
      _snack('Nota compartida con el grupo.');
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
    } catch (_) {
      _snack('No se pudo compartir la nota.', error: true);
    }
  }

  Future<void> _unshare(SharedGroupNote note) async {
    final sharedId = _sharedByMe[note.noteId];
    if (sharedId == null) {
      _snack(
        'Solo puedes dejar de compartir las notas que compartiste en esta sesión.',
        error: true,
      );
      return;
    }
    final ok = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: NeobrutalistDialog(
        title: 'Dejar de compartir',
        cancelLabel: 'Cancelar',
        confirmLabel: 'Retirar',
        confirmVariant: NeobrutalistButtonVariant.danger,
        closeOnConfirm: false,
        onConfirm: () => Navigator.of(context).pop(true),
        content: Text('¿Retirar "${note.title}" de este grupo?'),
      ),
    );
    if (ok != true || !mounted) return;
    setState(() => _busyNoteId = note.noteId);
    try {
      await _service.unshare(sharedId);
      _sharedByMe.remove(note.noteId);
      if (!mounted) return;
      await _load();
      _snack('Se dejó de compartir la nota.');
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
    } catch (_) {
      _snack('No se pudo dejar de compartir la nota.', error: true);
    } finally {
      if (mounted) setState(() => _busyNoteId = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      color: AppColors.bg,
      child: Column(
        children: [
          _header(),
          const Divider(
            color: AppColors.border,
            thickness: AppDimens.borderWidth,
            height: 1,
          ),
          Expanded(child: _body()),
        ],
      ),
    );
  }

  Widget _header() {
    return Container(
      color: AppColors.surface,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceLg,
        vertical: AppDimens.spaceSm,
      ),
      child: LayoutBuilder(
        builder: (context, constraints) {
          // En pantallas angostas o con texto ampliado el botón con etiqueta
          // dejaba al título ~13 px de ancho (una letra por línea). Ahí el
          // refresco pasa a botón de ícono con tooltip.
          final textScale = MediaQuery.textScalerOf(context).scale(1);
          final compact = constraints.maxWidth < 420 || textScale > 1.3;
          return Row(
            children: [
              const Icon(
                Icons.description_rounded,
                size: 16,
                color: AppColors.text,
              ),
              const SizedBox(width: AppDimens.spaceSm),
              const Expanded(
                child: Text(
                  'NOTAS COMPARTIDAS',
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 12,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
              const SizedBox(width: AppDimens.spaceSm),
              if (compact)
                NeobrutalistIconButton(
                  icon: Icons.refresh_rounded,
                  tooltip: 'Refrescar',
                  size: 40,
                  onPressed: _loading ? null : _load,
                )
              else
                NeobrutalistButton(
                  label: 'Refrescar',
                  icon: Icons.refresh_rounded,
                  variant: NeobrutalistButtonVariant.secondary,
                  padding: const EdgeInsets.symmetric(
                    horizontal: AppDimens.spaceMd,
                    vertical: AppDimens.spaceSm,
                  ),
                  onPressed: _loading ? null : _load,
                ),
            ],
          );
        },
      ),
    );
  }

  Widget _body() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      // Scroll: con texto ampliado (200 %) el estado no cabe en móviles.
      return Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(AppDimens.spaceXl),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded, size: 32),
              const SizedBox(height: AppDimens.spaceSm),
              Text(_error!, textAlign: TextAlign.center),
              const SizedBox(height: AppDimens.spaceMd),
              NeobrutalistButton(
                label: 'Reintentar',
                variant: NeobrutalistButtonVariant.secondary,
                onPressed: _load,
              ),
            ],
          ),
        ),
      );
    }
    return Column(
      children: [
        Expanded(
          child: _notes.isEmpty
              ? _empty()
              : ListView.builder(
                  padding: const EdgeInsets.all(AppDimens.spaceMd),
                  itemCount: _notes.length,
                  itemBuilder: (_, i) => _noteCard(_notes[i]),
                ),
        ),
        Padding(
          padding: const EdgeInsets.all(AppDimens.spaceMd),
          child: NeobrutalistButton(
            label: 'Compartir una nota',
            icon: Icons.share_rounded,
            variant: NeobrutalistButtonVariant.accent,
            expand: true,
            onPressed: _groupId.isEmpty ? null : _shareFlow,
          ),
        ),
      ],
    );
  }

  Widget _empty() {
    return const Center(
      child: SingleChildScrollView(
        padding: EdgeInsets.all(AppDimens.spaceXl),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.folder_open_rounded, size: 36),
            SizedBox(height: AppDimens.spaceSm),
            Text(
              'Aún no hay notas compartidas con este grupo.',
              textAlign: TextAlign.center,
              style: TextStyle(fontWeight: FontWeight.w700),
            ),
          ],
        ),
      ),
    );
  }

  Widget _noteCard(SharedGroupNote note) {
    final busy = _busyNoteId == note.noteId;
    final canUnshare = _sharedByMe.containsKey(note.noteId);
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      child: NeobrutalistCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              note.title,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
              ),
            ),
            const SizedBox(height: AppDimens.spaceSm),
            Wrap(
              spacing: AppDimens.spaceSm,
              runSpacing: AppDimens.spaceXs,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                NeobrutalistBadge(
                  label: note.visibility == 'public' ? 'Pública' : 'Privada',
                  tone: note.visibility == 'public'
                      ? NeobrutalistTone.info
                      : NeobrutalistTone.neutral,
                ),
                NeobrutalistBadge(
                  label: '${note.likesCount} likes',
                  tone: NeobrutalistTone.accent,
                  icon: Icons.favorite_rounded,
                ),
              ],
            ),
            const SizedBox(height: AppDimens.spaceMd),
            Wrap(
              spacing: AppDimens.spaceSm,
              runSpacing: AppDimens.spaceSm,
              children: [
                NeobrutalistButton(
                  label: 'Abrir',
                  icon: Icons.open_in_new_rounded,
                  variant: NeobrutalistButtonVariant.primary,
                  padding: const EdgeInsets.symmetric(
                    horizontal: AppDimens.spaceMd,
                    vertical: AppDimens.spaceSm,
                  ),
                  onPressed: busy ? null : () => _openNote(note),
                ),
                if (canUnshare)
                  NeobrutalistButton(
                    label: 'Dejar de compartir',
                    icon: Icons.link_off_rounded,
                    variant: NeobrutalistButtonVariant.danger,
                    padding: const EdgeInsets.symmetric(
                      horizontal: AppDimens.spaceMd,
                      vertical: AppDimens.spaceSm,
                    ),
                    onPressed: busy ? null : () => _unshare(note),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// Visor neobrutalista del contenido de una nota compartida + sus recursos
/// adjuntos (`attachment:<id>` resueltos con los adjuntos reales).
class _NoteViewer extends StatelessWidget {
  const _NoteViewer({required this.content});

  final SharedNoteContent content;

  Future<void> _openDrive(String url) async {
    final uri = Uri.tryParse(url.trim());
    if (uri == null) return;
    try {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    final real = attachmentResources(content.noteId, content.attachments);
    final fromMarkdown = resourcesFromMarkdown(content.content);
    final resources = mergeAttachmentResources(real, fromMarkdown);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SelectableText(
          content.content.isEmpty ? '(sin contenido)' : content.content,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w600,
            color: AppColors.text,
            height: 1.4,
          ),
        ),
        if (resources.isNotEmpty) ...[
          const SizedBox(height: AppDimens.spaceLg),
          const Divider(
            color: AppColors.border,
            thickness: AppDimens.borderWidth,
            height: 1,
          ),
          const SizedBox(height: AppDimens.spaceMd),
          const Text(
            'RECURSOS ADJUNTOS',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          ...resources.map(_resourceTile),
        ],
      ],
    );
  }

  Widget _resourceTile(Map<String, dynamic> resource) {
    final type = resource['type'] as String? ?? 'doc';
    final name = (resource['name'] as String? ?? 'Adjunto').trim();
    final url = resource['url'] as String? ?? '';
    final attachmentId = resource['attachment_id'] as String? ?? '';
    final isImage = type == 'image' && attachmentId.isNotEmpty;

    if (isImage) {
      return Padding(
        padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
        child: Container(
          height: 220,
          decoration: BoxDecoration(
            color: AppColors.surfaceLow,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.badge,
          ),
          child: AuthenticatedAttachmentImage(
            key: ValueKey('att:$attachmentId'),
            contentUri: Uri.parse(
              '${NotesSharingService.baseUrl}/notes/${content.noteId}/attachments/$attachmentId/content',
            ),
            fit: BoxFit.contain,
            onOpenDrive: () => _openDrive(url),
            debugLabel: 'SHARED_NOTE',
          ),
        ),
      );
    }

    final icon = type == 'pdf'
        ? Icons.picture_as_pdf_rounded
        : Icons.attach_file_rounded;
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: Container(
        padding: const EdgeInsets.all(AppDimens.spaceMd),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: AppShadows.badge,
        ),
        child: Row(
          children: [
            Icon(icon, size: 18, color: AppColors.text),
            const SizedBox(width: AppDimens.spaceSm),
            Expanded(
              child: Text(
                name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontWeight: FontWeight.w800,
                  fontSize: 13,
                ),
              ),
            ),
            if (url.isNotEmpty)
              NeobrutalistButton(
                label: 'Abrir',
                icon: Icons.open_in_new_rounded,
                variant: NeobrutalistButtonVariant.info,
                padding: const EdgeInsets.symmetric(
                  horizontal: AppDimens.spaceMd,
                  vertical: AppDimens.spaceSm,
                ),
                onPressed: () => _openDrive(url),
              ),
          ],
        ),
      ),
    );
  }
}

class _ShareChoice {
  const _ShareChoice(this.noteId, this.accessMode);

  final String noteId;
  final String accessMode;
}

/// Diálogo neobrutalista para elegir la nota propia y el modo de compartición.
class _ShareNoteDialog extends StatefulWidget {
  const _ShareNoteDialog({required this.notes});

  final List<MyNoteSummary> notes;

  @override
  State<_ShareNoteDialog> createState() => _ShareNoteDialogState();
}

class _ShareNoteDialogState extends State<_ShareNoteDialog> {
  late String _noteId = widget.notes.first.noteId;
  String _accessMode = NotesSharingService.accessModeLink;

  @override
  Widget build(BuildContext context) {
    return NeobrutalistDialog(
      title: 'Compartir una nota',
      cancelLabel: 'Cancelar',
      confirmLabel: 'Compartir',
      closeOnConfirm: false,
      onConfirm: () =>
          Navigator.of(context).pop(_ShareChoice(_noteId, _accessMode)),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            'NOTA',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.5,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          ConstrainedBox(
            constraints: const BoxConstraints(maxHeight: 220),
            child: ListView.builder(
              shrinkWrap: true,
              itemCount: widget.notes.length,
              itemBuilder: (_, i) {
                final n = widget.notes[i];
                final selected = n.noteId == _noteId;
                return Padding(
                  padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
                  child: GestureDetector(
                    onTap: () => setState(() => _noteId = n.noteId),
                    child: Container(
                      padding: const EdgeInsets.all(AppDimens.spaceMd),
                      decoration: BoxDecoration(
                        color: selected
                            ? AppColors.accentYellow
                            : AppColors.surface,
                        border: Border.all(
                          color: AppColors.border,
                          width: AppDimens.borderWidth,
                        ),
                        borderRadius: BorderRadius.circular(AppDimens.radius),
                        boxShadow: selected ? AppShadows.badge : null,
                      ),
                      child: Row(
                        children: [
                          Icon(
                            selected
                                ? Icons.radio_button_checked_rounded
                                : Icons.radio_button_unchecked_rounded,
                            size: 18,
                            color: AppColors.text,
                          ),
                          const SizedBox(width: AppDimens.spaceSm),
                          Expanded(
                            child: Text(
                              n.title,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: const TextStyle(
                                fontWeight: FontWeight.w800,
                                fontSize: 13,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                );
              },
            ),
          ),
          const SizedBox(height: AppDimens.spaceMd),
          const Text(
            'MODO',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.5,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          // Wrap: en 360 px o con texto al 200 % los dos modos no caben en una
          // fila y el Row desbordaba por la derecha.
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            children: [
              NeobrutalistButton(
                label: 'Link',
                icon: Icons.link_rounded,
                variant: _accessMode == NotesSharingService.accessModeLink
                    ? NeobrutalistButtonVariant.accent
                    : NeobrutalistButtonVariant.secondary,
                padding: const EdgeInsets.symmetric(
                  horizontal: AppDimens.spaceMd,
                  vertical: AppDimens.spaceSm,
                ),
                onPressed: () => setState(
                  () => _accessMode = NotesSharingService.accessModeLink,
                ),
              ),
              NeobrutalistButton(
                label: 'Restricted',
                icon: Icons.group_rounded,
                variant: _accessMode == NotesSharingService.accessModeRestricted
                    ? NeobrutalistButtonVariant.accent
                    : NeobrutalistButtonVariant.secondary,
                padding: const EdgeInsets.symmetric(
                  horizontal: AppDimens.spaceMd,
                  vertical: AppDimens.spaceSm,
                ),
                onPressed: () => setState(
                  () => _accessMode = NotesSharingService.accessModeRestricted,
                ),
              ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Text(
            _accessMode == NotesSharingService.accessModeLink
                ? 'Cualquiera con el enlace del grupo puede verla.'
                : 'Solo los miembros del grupo acceden (permisos por correo).',
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: AppColors.muted,
            ),
          ),
        ],
      ),
    );
  }
}
