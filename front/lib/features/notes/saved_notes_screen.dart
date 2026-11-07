import 'dart:async';

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'note_id_search_screen.dart';
import 'note_interaction_bar.dart';
import 'notes_api.dart';
import 'notes_filter_chips.dart';

/// Texto canónico de un guardado cuyo original ya no se puede abrir.
String savedUnavailableLabel(SavedNoteItem item) => item.isDeleted
    ? 'Apunte no disponible (eliminado por el autor)'
    : 'Apunte no disponible (el autor ya no lo comparte)';

/// Sección "Apuntes guardados" (GET /notes/saved).
///
/// Lista los marcadores del usuario con filtros (texto, de otros / míos,
/// disponibilidad), contador de likes, quitar de guardados y copiar a mis
/// apuntes. Un guardado cuyo original fue borrado o dejó de compartirse se
/// muestra deshabilitado y solo permite quitarlo.
class SavedNotesScreen extends StatefulWidget {
  const SavedNotesScreen({super.key, this.api, this.onNoteCopied});

  final NotesApi? api;
  final VoidCallback? onNoteCopied;

  @override
  State<SavedNotesScreen> createState() => _SavedNotesScreenState();
}

class _SavedNotesScreenState extends State<SavedNotesScreen> {
  late final NotesApi _api = widget.api ?? NotesApi();
  final TextEditingController _search = TextEditingController();
  Timer? _debounce;

  SavedNotesFilter _filter = const SavedNotesFilter();
  List<SavedNoteItem> _items = const [];
  String _cursor = '';
  bool _loading = true;
  bool _loadingMore = false;
  String? _error;
  int _seq = 0;
  final Set<String> _busy = <String>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    if (widget.api == null) _api.dispose();
    super.dispose();
  }

  Future<void> _load({bool more = false}) async {
    final seq = more ? _seq : ++_seq;
    setState(() {
      if (more) {
        _loadingMore = true;
      } else {
        _loading = true;
        _error = null;
      }
    });
    try {
      final page = await _api.listSaved(
        filter: _filter,
        cursor: more ? _cursor : null,
      );
      if (!mounted || seq != _seq) return;
      setState(() {
        _items = more ? [..._items, ...page.items] : page.items;
        _cursor = page.nextCursor;
      });
    } on NotesApiException catch (e) {
      if (!mounted || seq != _seq) return;
      setState(() => _error = e.message);
    } catch (_) {
      if (!mounted || seq != _seq) return;
      setState(() => _error = notesFriendlyMessage(-1, null));
    } finally {
      if (mounted && seq == _seq) {
        setState(() {
          _loading = false;
          _loadingMore = false;
        });
      }
    }
  }

  void _setFilter(SavedNotesFilter f) {
    setState(() => _filter = f);
    _load();
  }

  void _onSearchChanged(String text) {
    _debounce?.cancel();
    _debounce = Timer(
      const Duration(milliseconds: 350),
      () => _setFilter(_filter.copyWith(query: text)),
    );
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

  Future<void> _unsave(SavedNoteItem item) async {
    setState(() => _busy.add(item.savedId));
    try {
      await _api.setSaved(item.noteId, false);
      if (!mounted) return;
      setState(() {
        _items = _items.where((i) => i.savedId != item.savedId).toList();
      });
      _snack('Quitado de guardados.');
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
    } catch (_) {
      _snack(notesFriendlyMessage(-1, null), error: true);
    } finally {
      if (mounted) setState(() => _busy.remove(item.savedId));
    }
  }

  Future<void> _copy(SavedNoteItem item) async {
    setState(() => _busy.add(item.savedId));
    try {
      await _api.copyNote(
        item.noteId,
        idempotencyKey:
            'copy-${item.noteId}-${DateTime.now().microsecondsSinceEpoch}',
      );
      widget.onNoteCopied?.call();
      _snack('Copia guardada en tus apuntes y en tu Drive.');
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
      // Si el autor lo borró mientras tanto, se refleja en la lista.
      if (e.statusCode == 410) unawaited(_load());
    } catch (_) {
      _snack(notesFriendlyMessage(-1, null), error: true);
    } finally {
      if (mounted) setState(() => _busy.remove(item.savedId));
    }
  }

  Future<void> _open(SavedNoteItem item) async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => NoteIdSearchScreen(
          api: _api,
          initialQuery: item.noteId,
          onNoteCopied: widget.onNoteCopied,
        ),
      ),
    );
    if (mounted) await _load();
  }

  void _updateInteractions(SavedNoteItem item, NoteInteractions s) {
    setState(() {
      _items = [
        for (final it in _items)
          if (it.savedId == item.savedId)
            SavedNoteItem(
              savedId: it.savedId,
              noteId: it.noteId,
              title: it.title,
              isAvailable: it.isAvailable,
              unavailableReason: it.unavailableReason,
              authorId: it.authorId,
              subjectId: it.subjectId,
              isOwn: it.isOwn,
              savedAt: it.savedAt,
              updatedAt: it.updatedAt,
              deletedAt: it.deletedAt,
              interactions: s,
            )
          else
            it,
      ];
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: MaxWidthContainer(
          maxWidth: 960,
          padding: EdgeInsets.zero,
          child: RefreshIndicator(
            color: AppColors.border,
            onRefresh: _load,
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
                    const Expanded(
                      child: Text(
                        'APUNTES GUARDADOS',
                        style: TextStyle(
                          fontSize: 22,
                          fontWeight: FontWeight.w900,
                          color: AppColors.text,
                          letterSpacing: -0.5,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: AppDimens.spaceSm),
                const Text(
                  'Tus marcadores de apuntes propios y de otros estudiantes.',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: AppColors.muted,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                TextField(
                  key: const Key('saved-notes-search'),
                  controller: _search,
                  onChanged: _onSearchChanged,
                  style: const TextStyle(
                    fontWeight: FontWeight.w700,
                    fontSize: 14,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration('Buscar en guardados…')
                      .copyWith(
                        prefixIcon: const Icon(
                          Icons.search_rounded,
                          color: AppColors.border,
                        ),
                      ),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                const _FilterLabel('Autor'),
                NotesFilterChips<String>(
                  keyPrefix: 'scope-',
                  value: _filter.scope,
                  options: const {
                    'all': 'Todos',
                    'others': 'De otros',
                    'own': 'Míos',
                  },
                  onChanged: (v) => _setFilter(_filter.copyWith(scope: v)),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                const _FilterLabel('Estado'),
                NotesFilterChips<String>(
                  keyPrefix: 'availability-',
                  value: _filter.availability,
                  options: const {
                    'all': 'Todos',
                    'available': 'Disponibles',
                    'unavailable': 'No disponibles',
                  },
                  onChanged: (v) =>
                      _setFilter(_filter.copyWith(availability: v)),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                ..._buildContent(),
              ],
            ),
          ),
        ),
      ),
    );
  }

  List<Widget> _buildContent() {
    if (_loading) {
      return const <Widget>[
        Padding(
          padding: EdgeInsets.all(AppDimens.spaceXl),
          child: Center(
            child: CircularProgressIndicator(color: AppColors.border),
          ),
        ),
      ];
    }
    if (_error != null) {
      return <Widget>[
        NeobrutalistCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                _error!,
                style: const TextStyle(
                  fontWeight: FontWeight.w800,
                  color: AppColors.text,
                ),
              ),
              const SizedBox(height: AppDimens.spaceMd),
              NeobrutalistButton(
                label: 'Reintentar',
                icon: Icons.refresh_rounded,
                variant: NeobrutalistButtonVariant.accent,
                onPressed: _load,
              ),
            ],
          ),
        ),
      ];
    }
    if (_items.isEmpty) {
      return const <Widget>[
        NeobrutalistCard(
          key: Key('saved-notes-empty'),
          child: Text(
            'No tienes apuntes guardados con estos filtros. Guarda apuntes '
            'públicos desde "Explorar públicos" o "Buscar por ID".',
            style: TextStyle(
              fontWeight: FontWeight.w800,
              color: AppColors.text,
            ),
          ),
        ),
      ];
    }
    return <Widget>[
      for (final item in _items)
        Padding(
          padding: const EdgeInsets.only(bottom: AppDimens.spaceLg),
          child: SavedNoteCard(
            key: ValueKey('saved-${item.savedId}'),
            item: item,
            api: _api,
            busy: _busy.contains(item.savedId),
            onUnsave: () => _unsave(item),
            onCopy: () => _copy(item),
            onOpen: () => _open(item),
            onInteractionsChanged: (s) => _updateInteractions(item, s),
          ),
        ),
      if (_cursor.isNotEmpty)
        Center(
          child: NeobrutalistButton(
            label: _loadingMore ? 'Cargando…' : 'Cargar más',
            icon: Icons.expand_more_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: _loadingMore ? null : () => _load(more: true),
          ),
        ),
    ];
  }
}

class _FilterLabel extends StatelessWidget {
  const _FilterLabel(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: AppDimens.spaceXs),
    child: Text(
      text.toUpperCase(),
      style: const TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w900,
        letterSpacing: 0.6,
        color: AppColors.mutedStrong,
      ),
    ),
  );
}

/// Tarjeta de un guardado. Si el original no está disponible se pinta
/// deshabilitada (relieve bajo, sin sombra) y solo ofrece quitarla.
class SavedNoteCard extends StatelessWidget {
  const SavedNoteCard({
    super.key,
    required this.item,
    required this.api,
    required this.onUnsave,
    this.onCopy,
    this.onOpen,
    this.onInteractionsChanged,
    this.busy = false,
  });

  final SavedNoteItem item;
  final NotesApi api;
  final VoidCallback onUnsave;
  final VoidCallback? onCopy;
  final VoidCallback? onOpen;
  final ValueChanged<NoteInteractions>? onInteractionsChanged;
  final bool busy;

  static const EdgeInsets _compact = EdgeInsets.symmetric(
    horizontal: AppDimens.spaceSm,
    vertical: AppDimens.spaceXs + 2,
  );

  String _fmt(DateTime d) =>
      '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';

  @override
  Widget build(BuildContext context) {
    return item.isAvailable ? _available() : _unavailable();
  }

  Widget _unsaveButton() => NeobrutalistButton(
    key: Key('unsave-${item.noteId}'),
    label: busy ? 'Quitando…' : 'Quitar de guardados',
    icon: Icons.bookmark_remove_rounded,
    variant: NeobrutalistButtonVariant.danger,
    padding: _compact,
    onPressed: busy ? null : onUnsave,
  );

  Widget _available() {
    final author = (item.authorId ?? '').length >= 8
        ? item.authorId!.substring(0, 8)
        : (item.authorId ?? '—');
    return NeobrutalistCard(
      semanticLabel: 'Apunte guardado ${item.title}',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text(
            item.title,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            children: <Widget>[
              if (item.isOwn)
                const NeobrutalistBadge(
                  label: 'Tuyo',
                  tone: NeobrutalistTone.accent,
                  compact: true,
                )
              else
                NeobrutalistBadge(
                  label: 'Autor $author',
                  icon: Icons.person_rounded,
                  compact: true,
                ),
              if (item.savedAt != null)
                NeobrutalistBadge(
                  label: 'Guardado ${_fmt(item.savedAt!)}',
                  icon: Icons.bookmark_rounded,
                  compact: true,
                ),
              NeobrutalistBadge(
                label: '${item.interactions.savesCount} guardados',
                compact: true,
              ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceMd),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            children: <Widget>[
              NoteLikeSaveBar(
                api: api,
                initial: item.interactions,
                compact: true,
                showSave: false,
                onChanged: onInteractionsChanged,
              ),
              if (onOpen != null)
                NeobrutalistButton(
                  label: 'Ver',
                  icon: Icons.open_in_new_rounded,
                  variant: NeobrutalistButtonVariant.info,
                  padding: _compact,
                  onPressed: busy ? null : onOpen,
                ),
              if (!item.isOwn && onCopy != null)
                NeobrutalistButton(
                  key: Key('copy-${item.noteId}'),
                  label: 'Copiar a mis apuntes',
                  icon: Icons.copy_rounded,
                  variant: NeobrutalistButtonVariant.accent,
                  padding: _compact,
                  onPressed: busy ? null : onCopy,
                ),
              _unsaveButton(),
            ],
          ),
        ],
      ),
    );
  }

  Widget _unavailable() {
    return NeobrutalistCard(
      key: Key('saved-unavailable-${item.noteId}'),
      backgroundColor: AppColors.surfaceLow,
      // Relieve bajo: sombra mínima para que se lea "apagada".
      shadow: AppShadows.hard2,
      semanticLabel: savedUnavailableLabel(item),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Icon(
                item.isDeleted
                    ? Icons.delete_forever_rounded
                    : Icons.lock_rounded,
                color: AppColors.mutedStrong,
                size: 20,
              ),
              const SizedBox(width: AppDimens.spaceSm),
              Expanded(
                child: Text(
                  item.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w900,
                    color: AppColors.mutedStrong,
                    decoration: TextDecoration.lineThrough,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceSm),
          NeobrutalistBadge(
            label: savedUnavailableLabel(item),
            tone: NeobrutalistTone.pending,
            icon: Icons.block_rounded,
          ),
          if (item.deletedAt != null) ...<Widget>[
            const SizedBox(height: AppDimens.spaceSm),
            Text(
              'Eliminado el ${_fmt(item.deletedAt!)}. Solo puedes quitarlo de tus guardados.',
              style: const TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ] else ...<Widget>[
            const SizedBox(height: AppDimens.spaceSm),
            const Text(
              'Solo puedes quitarlo de tus guardados.',
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
          const SizedBox(height: AppDimens.spaceMd),
          _unsaveButton(),
        ],
      ),
    );
  }
}
