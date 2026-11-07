import 'dart:async';

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'note_id_search_screen.dart';
import 'notes_filter_chips.dart';
import 'note_interaction_bar.dart';
import 'notes_api.dart';

/// Filtro local de la vista de públicos: distingue los que ya guardaste.
enum PublicSavedFilter { all, saved, notSaved }

/// "Explorar apuntes públicos" de otros usuarios (GET /notes/public), con
/// like y guardar en cada tarjeta y filtro por guardados.
class PublicNotesScreen extends StatefulWidget {
  const PublicNotesScreen({super.key, this.api, this.onNoteCopied});

  final NotesApi? api;
  final VoidCallback? onNoteCopied;

  @override
  State<PublicNotesScreen> createState() => _PublicNotesScreenState();
}

class _PublicNotesScreenState extends State<PublicNotesScreen> {
  late final NotesApi _api = widget.api ?? NotesApi();
  final TextEditingController _search = TextEditingController();
  Timer? _debounce;

  List<PublicNoteSummary> _items = const [];
  String _cursor = '';
  bool _loading = true;
  bool _loadingMore = false;
  String? _error;
  PublicSavedFilter _filter = PublicSavedFilter.all;
  int _seq = 0;

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
      final page = await _api.listPublic(
        query: _search.text,
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

  void _onSearchChanged(String _) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 350), _load);
  }

  void _updateItem(PublicNoteSummary item, NoteInteractions state) {
    setState(() {
      _items = [
        for (final it in _items)
          if (it.id == item.id)
            PublicNoteSummary(
              id: it.id,
              userId: it.userId,
              title: it.title,
              subjectId: it.subjectId,
              updatedAt: it.updatedAt,
              interactions: state,
            )
          else
            it,
      ];
    });
  }

  List<PublicNoteSummary> get _visible {
    switch (_filter) {
      case PublicSavedFilter.saved:
        return _items.where((n) => n.interactions.isSaved).toList();
      case PublicSavedFilter.notSaved:
        return _items.where((n) => !n.interactions.isSaved).toList();
      case PublicSavedFilter.all:
        return _items;
    }
  }

  Future<void> _open(PublicNoteSummary note) async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => NoteIdSearchScreen(
          api: _api,
          initialQuery: note.id,
          onNoteCopied: widget.onNoteCopied,
        ),
      ),
    );
    // Al volver se relee la página: likes/guardados pudieron cambiar dentro.
    if (mounted) await _load();
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
                        'APUNTES PÚBLICOS',
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
                  'Apuntes que otros estudiantes publicaron. Dales like o guárdalos para después.',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: AppColors.muted,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                TextField(
                  key: const Key('public-notes-search'),
                  controller: _search,
                  onChanged: _onSearchChanged,
                  onSubmitted: (_) => _load(),
                  style: const TextStyle(
                    fontWeight: FontWeight.w700,
                    fontSize: 14,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration('Buscar por título…').copyWith(
                    prefixIcon: const Icon(
                      Icons.search_rounded,
                      color: AppColors.border,
                    ),
                  ),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                NotesFilterChips<PublicSavedFilter>(
                  value: _filter,
                  options: const {
                    PublicSavedFilter.all: 'Todos',
                    PublicSavedFilter.saved: 'Guardados',
                    PublicSavedFilter.notSaved: 'No guardados',
                  },
                  onChanged: (v) => setState(() => _filter = v),
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
    final visible = _visible;
    if (visible.isEmpty) {
      return const <Widget>[
        NeobrutalistCard(
          child: Text(
            'No hay apuntes públicos que coincidan.',
            style: TextStyle(
              fontWeight: FontWeight.w800,
              color: AppColors.text,
            ),
          ),
        ),
      ];
    }
    return <Widget>[
      for (final note in visible)
        Padding(
          padding: const EdgeInsets.only(bottom: AppDimens.spaceLg),
          child: PublicNoteCard(
            key: ValueKey('public-${note.id}'),
            note: note,
            api: _api,
            onOpen: () => _open(note),
            onChanged: (s) => _updateItem(note, s),
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

/// Tarjeta de un apunte público con like/guardar.
class PublicNoteCard extends StatelessWidget {
  const PublicNoteCard({
    super.key,
    required this.note,
    required this.api,
    this.onOpen,
    this.onChanged,
  });

  final PublicNoteSummary note;
  final NotesApi api;
  final VoidCallback? onOpen;
  final ValueChanged<NoteInteractions>? onChanged;

  @override
  Widget build(BuildContext context) {
    final author = note.userId.length >= 8
        ? note.userId.substring(0, 8)
        : note.userId;
    final date = note.updatedAt;
    return NeobrutalistCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text(
            note.title,
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
              const NeobrutalistBadge(
                label: 'Público',
                tone: NeobrutalistTone.success,
                compact: true,
              ),
              NeobrutalistBadge(
                label: 'Autor $author',
                icon: Icons.person_rounded,
                compact: true,
              ),
              if (date != null)
                NeobrutalistBadge(
                  label:
                      '${date.day.toString().padLeft(2, '0')}/${date.month.toString().padLeft(2, '0')}/${date.year}',
                  compact: true,
                ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceMd),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: <Widget>[
              NoteLikeSaveBar(
                api: api,
                initial: note.interactions,
                onChanged: onChanged,
                compact: true,
              ),
              if (onOpen != null)
                NeobrutalistButton(
                  label: 'Ver apunte',
                  icon: Icons.open_in_new_rounded,
                  variant: NeobrutalistButtonVariant.info,
                  padding: const EdgeInsets.symmetric(
                    horizontal: AppDimens.spaceSm,
                    vertical: AppDimens.spaceXs + 2,
                  ),
                  onPressed: onOpen,
                ),
            ],
          ),
        ],
      ),
    );
  }
}
