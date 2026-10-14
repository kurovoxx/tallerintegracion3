import 'package:flutter/material.dart';

import '../../core/services/notes_sharing_service.dart';

/// Pestaña "NOTAS" de la vista de grupo: lista y abre las notas compartidas
/// con el grupo, permite compartir una nota propia eligiendo modo
/// (link/restricted) y dejar de compartir. Estados de carga/error/vacío
/// explícitos y refresco tras cada mutación.
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
  /// "dejar de compartir" para lo que el usuario acaba de compartir (el listado
  /// del backend no expone el shared_note_id).
  final Map<String, String> _sharedByMe = {};

  /// Nota con una operación en curso (abrir/compartir sería global).
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
        backgroundColor: error ? const Color(0xFFB00020) : null,
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
    try {
      final content = await _service.getNoteContent(note.noteId);
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text(content.title),
          content: SizedBox(
            width: 520,
            child: SingleChildScrollView(
              child: SelectableText(
                content.content.isEmpty ? '(sin contenido)' : content.content,
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(),
              child: const Text('CERRAR'),
            ),
          ],
        ),
      );
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
    } catch (_) {
      _snack('No se pudo abrir la nota.', error: true);
    } finally {
      if (mounted) setState(() => _busyNoteId = null);
    }
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
    final choice = await showDialog<_ShareChoice>(
      context: context,
      builder: (_) => _ShareNoteDialog(notes: mine),
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
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('DEJAR DE COMPARTIR'),
        content: Text('¿Retirar "${note.title}" de este grupo?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('CANCELAR'),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('RETIRAR'),
          ),
        ],
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
      color: const Color(0xFFF5F0E8),
      child: Column(
        children: [
          _header(),
          Container(height: 2, color: Colors.black),
          Expanded(child: _body()),
        ],
      ),
    );
  }

  Widget _header() {
    return Container(
      color: Colors.white,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
      child: Row(
        children: [
          const Icon(Icons.description_rounded, size: 16, color: Colors.black),
          const SizedBox(width: 8),
          const Expanded(
            child: Text(
              'NOTAS COMPARTIDAS',
              style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12),
            ),
          ),
          TextButton.icon(
            onPressed: _loading ? null : _load,
            icon: const Icon(Icons.refresh_rounded, size: 16, color: Colors.black),
            label: const Text(
              'REFRESCAR',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 11,
                color: Colors.black,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _body() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded, size: 32),
              const SizedBox(height: 8),
              Text(_error!, textAlign: TextAlign.center),
              const SizedBox(height: 12),
              ElevatedButton(
                onPressed: _load,
                child: const Text('REINTENTAR'),
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
                  padding: const EdgeInsets.all(12),
                  itemCount: _notes.length,
                  itemBuilder: (_, i) => _noteCard(_notes[i]),
                ),
        ),
        Padding(
          padding: const EdgeInsets.all(12),
          child: SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              onPressed: _groupId.isEmpty ? null : _shareFlow,
              icon: const Icon(Icons.share_rounded, size: 16),
              label: const Text('COMPARTIR UNA NOTA'),
              style: ElevatedButton.styleFrom(
                backgroundColor: const Color(0xFFFFCC00),
                foregroundColor: Colors.black,
                shape: const RoundedRectangleBorder(
                  borderRadius: BorderRadius.zero,
                  side: BorderSide(color: Colors.black, width: 2),
                ),
                padding: const EdgeInsets.symmetric(vertical: 14),
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _empty() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: const [
            Icon(Icons.folder_open_rounded, size: 36),
            SizedBox(height: 8),
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
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: Colors.black, width: 2),
        boxShadow: const [
          BoxShadow(color: Colors.black, offset: Offset(3, 3), blurRadius: 0),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            note.title,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14),
          ),
          const SizedBox(height: 6),
          Row(
            children: [
              _chip(note.visibility == 'public' ? 'PÚBLICA' : 'PRIVADA'),
              const SizedBox(width: 8),
              const Icon(Icons.favorite_rounded, size: 13),
              const SizedBox(width: 3),
              Text('${note.likesCount}',
                  style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700)),
            ],
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              OutlinedButton.icon(
                onPressed: busy ? null : () => _openNote(note),
                icon: const Icon(Icons.open_in_new_rounded, size: 15),
                label: const Text('ABRIR'),
              ),
              const SizedBox(width: 8),
              if (canUnshare)
                TextButton.icon(
                  onPressed: busy ? null : () => _unshare(note),
                  icon: const Icon(Icons.link_off_rounded, size: 15),
                  label: const Text('DEJAR DE COMPARTIR'),
                ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _chip(String label) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        border: Border.all(color: Colors.black, width: 1),
        color: const Color(0xFFF5F0E8),
      ),
      child: Text(
        label,
        style: const TextStyle(fontSize: 9, fontWeight: FontWeight.w900),
      ),
    );
  }
}

class _ShareChoice {
  const _ShareChoice(this.noteId, this.accessMode);

  final String noteId;
  final String accessMode;
}

/// Diálogo para elegir la nota propia y el modo de compartición.
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
    return AlertDialog(
      title: const Text('COMPARTIR UNA NOTA'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text('Nota', style: TextStyle(fontWeight: FontWeight.w900)),
            const SizedBox(height: 4),
            DropdownButtonFormField<String>(
              initialValue: _noteId,
              isExpanded: true,
              items: widget.notes
                  .map(
                    (n) => DropdownMenuItem<String>(
                      value: n.noteId,
                      child: Text(
                        n.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  )
                  .toList(),
              onChanged: (v) => setState(() => _noteId = v ?? _noteId),
            ),
            const SizedBox(height: 16),
            const Text('Modo', style: TextStyle(fontWeight: FontWeight.w900)),
            const SizedBox(height: 4),
            SegmentedButton<String>(
              segments: const [
                ButtonSegment<String>(
                  value: NotesSharingService.accessModeLink,
                  label: Text('LINK'),
                ),
                ButtonSegment<String>(
                  value: NotesSharingService.accessModeRestricted,
                  label: Text('RESTRICTED'),
                ),
              ],
              selected: {_accessMode},
              onSelectionChanged: (s) => setState(() => _accessMode = s.first),
            ),
            const SizedBox(height: 6),
            Text(
              _accessMode == NotesSharingService.accessModeLink
                  ? 'Cualquiera con el enlace del grupo puede verla.'
                  : 'Solo los miembros del grupo acceden (permisos por correo).',
              style: const TextStyle(fontSize: 11),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('CANCELAR'),
        ),
        TextButton(
          onPressed: () =>
              Navigator.of(context).pop(_ShareChoice(_noteId, _accessMode)),
          child: const Text('COMPARTIR'),
        ),
      ],
    );
  }
}
