import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'note_interaction_bar.dart';
import 'notes_api.dart';

/// Buscador de apuntes por ID exacto (GET /notes/{id}).
///
/// Acepta el UUID solo o un enlace `.../notes/<id>`. Muestra el apunte si el
/// usuario puede leerlo (propio, público o compartido con él), o un aviso
/// claro si no existe / no tiene permiso (404) o si el autor lo eliminó (410).
class NoteIdSearchScreen extends StatefulWidget {
  const NoteIdSearchScreen({
    super.key,
    this.api,
    this.initialQuery,
    this.onNoteCopied,
  });

  /// Cliente inyectable (tests / base URL de Android). Por defecto NotesApi().
  final NotesApi? api;

  /// Texto inicial (p. ej. un UUID pegado en la búsqueda general): si contiene
  /// un ID válido, se busca al abrir.
  final String? initialQuery;

  /// Se invoca tras copiar el apunte a los propios, para refrescar la lista.
  final VoidCallback? onNoteCopied;

  @override
  State<NoteIdSearchScreen> createState() => _NoteIdSearchScreenState();
}

class _NoteIdSearchScreenState extends State<NoteIdSearchScreen> {
  late final NotesApi _api = widget.api ?? NotesApi();
  late final TextEditingController _controller = TextEditingController(
    text: widget.initialQuery ?? '',
  );

  bool _loading = false;
  bool _copying = false;
  NoteDetail? _result;
  NotesApiException? _error;
  int _searchSeq = 0;

  @override
  void initState() {
    super.initState();
    if (extractNoteId(_controller.text) != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _search());
    }
  }

  @override
  void dispose() {
    if (widget.api == null) _api.dispose();
    _controller.dispose();
    super.dispose();
  }

  Future<void> _search() async {
    final id = extractNoteId(_controller.text);
    if (id == null) {
      setState(() {
        _result = null;
        _error = NotesApiException(
          'Pega el ID del apunte (formato xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx) o su enlace.',
          code: 'invalid_id',
        );
      });
      return;
    }
    final seq = ++_searchSeq;
    setState(() {
      _loading = true;
      _error = null;
      _result = null;
    });
    try {
      final note = await _api.getNote(id);
      if (!mounted || seq != _searchSeq) return;
      setState(() {
        _result = note;
        _loading = false;
      });
    } on NotesApiException catch (e) {
      if (!mounted || seq != _searchSeq) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    } catch (_) {
      if (!mounted || seq != _searchSeq) return;
      setState(() {
        _error = NotesApiException(notesFriendlyMessage(-1, null));
        _loading = false;
      });
    }
  }

  Future<void> _paste() async {
    final data = await Clipboard.getData(Clipboard.kTextPlain);
    final text = data?.text ?? '';
    if (text.isEmpty) return;
    _controller.text = text.trim();
    await _search();
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

  Future<void> _copyToMine(NoteDetail note) async {
    setState(() => _copying = true);
    try {
      await _api.copyNote(
        note.id,
        idempotencyKey:
            'copy-${note.id}-${DateTime.now().microsecondsSinceEpoch}',
      );
      widget.onNoteCopied?.call();
      _snack('Copia guardada en tus apuntes y en tu Drive.');
    } on NotesApiException catch (e) {
      _snack(e.message, error: true);
    } catch (_) {
      _snack(notesFriendlyMessage(-1, null), error: true);
    } finally {
      if (mounted) setState(() => _copying = false);
    }
  }

  @override
  Widget build(BuildContext context) {
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
                  const Expanded(
                    child: Text(
                      'BUSCAR APUNTE POR ID',
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
                'Pega el ID o el enlace de un apunte público, propio o compartido contigo.',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: AppColors.muted,
                ),
              ),
              const SizedBox(height: AppDimens.spaceLg),
              _buildSearchField(),
              const SizedBox(height: AppDimens.spaceLg),
              _buildState(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildSearchField() {
    final field = TextField(
      key: const Key('note-id-search-field'),
      controller: _controller,
      onSubmitted: (_) => _search(),
      textInputAction: TextInputAction.search,
      style: const TextStyle(
        fontWeight: FontWeight.w700,
        fontSize: 14,
        color: AppColors.text,
      ),
      decoration: appInputDecoration('ID del apunte o enlace').copyWith(
        prefixIcon: const Icon(Icons.tag_rounded, color: AppColors.border),
      ),
    );
    final buttons = Wrap(
      spacing: AppDimens.spaceSm,
      runSpacing: AppDimens.spaceSm,
      children: <Widget>[
        NeobrutalistButton(
          label: 'Pegar',
          icon: Icons.content_paste_rounded,
          variant: NeobrutalistButtonVariant.secondary,
          onPressed: _loading ? null : _paste,
        ),
        NeobrutalistButton(
          key: const Key('note-id-search-button'),
          label: _loading ? 'Buscando…' : 'Buscar',
          icon: Icons.search_rounded,
          variant: NeobrutalistButtonVariant.accent,
          onPressed: _loading ? null : _search,
        ),
      ],
    );
    return LayoutBuilder(
      builder: (context, constraints) {
        if (constraints.maxWidth < AppDimens.breakpointCompact) {
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[
              field,
              const SizedBox(height: AppDimens.spaceSm),
              buttons,
            ],
          );
        }
        return Row(
          children: <Widget>[
            Expanded(child: field),
            const SizedBox(width: AppDimens.spaceMd),
            buttons,
          ],
        );
      },
    );
  }

  Widget _buildState() {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.all(AppDimens.spaceXl),
        child: Center(
          child: CircularProgressIndicator(color: AppColors.border),
        ),
      );
    }
    final error = _error;
    if (error != null) return _SearchMessageCard.fromError(error);
    final note = _result;
    if (note != null) {
      return NoteSearchResultCard(
        note: note,
        copying: _copying,
        actions: <Widget>[
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
              () => _result = note.copyWith(
                likesCount: s.likesCount,
                savesCount: s.savesCount,
                isLiked: s.isLiked,
                isSaved: s.isSaved,
              ),
            ),
          ),
        ],
        onCopy: note.isOwn ? null : () => _copyToMine(note),
        onCopyId: () {
          Clipboard.setData(ClipboardData(text: note.id));
          _snack('ID copiado al portapapeles.');
        },
      );
    }
    return const _SearchMessageCard(
      icon: Icons.manage_search_rounded,
      title: 'Busca un apunte',
      message:
          'Cada apunte tiene un ID único. Pídeselo a su autor o cópialo desde "Compartir".',
    );
  }
}

/// Tarjeta del apunte encontrado: título, estado, contadores y vista previa.
class NoteSearchResultCard extends StatelessWidget {
  const NoteSearchResultCard({
    super.key,
    required this.note,
    this.onCopy,
    this.onCopyId,
    this.copying = false,
    this.actions = const <Widget>[],
  });

  final NoteDetail note;
  final VoidCallback? onCopy;
  final VoidCallback? onCopyId;
  final bool copying;

  /// Acciones extra (like / guardar) que añade quien la usa.
  final List<Widget> actions;

  static const int _previewChars = 1200;

  @override
  Widget build(BuildContext context) {
    final preview = note.content.length > _previewChars
        ? '${note.content.substring(0, _previewChars)}…'
        : note.content;
    return NeobrutalistCard(
      key: const Key('note-search-result'),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: <Widget>[
          Text(
            note.title,
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            children: <Widget>[
              NeobrutalistBadge(
                label: note.isPublic ? 'Público' : 'Privado / compartido',
                tone: note.isPublic
                    ? NeobrutalistTone.success
                    : NeobrutalistTone.neutral,
                compact: true,
              ),
              if (note.isOwn)
                const NeobrutalistBadge(
                  label: 'Tuyo',
                  tone: NeobrutalistTone.accent,
                  compact: true,
                ),
              NeobrutalistBadge(
                label: '${note.likesCount} likes',
                icon: Icons.favorite_rounded,
                compact: true,
              ),
              NeobrutalistBadge(
                label: '${note.savesCount} guardados',
                icon: Icons.bookmark_rounded,
                compact: true,
              ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceMd),
          Container(
            padding: const EdgeInsets.all(AppDimens.spaceMd),
            decoration: BoxDecoration(
              color: AppColors.bg,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radius),
            ),
            child: SelectableText(
              preview.isEmpty ? '(Apunte sin contenido)' : preview,
              style: const TextStyle(
                fontSize: 13,
                height: 1.45,
                fontWeight: FontWeight.w600,
                color: AppColors.text,
              ),
            ),
          ),
          const SizedBox(height: AppDimens.spaceMd),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            children: <Widget>[
              ...actions,
              if (onCopy != null)
                NeobrutalistButton(
                  key: const Key('note-search-copy'),
                  label: copying ? 'Copiando…' : 'Copiar a mis apuntes',
                  icon: Icons.copy_rounded,
                  variant: NeobrutalistButtonVariant.accent,
                  onPressed: copying ? null : onCopy,
                ),
              if (onCopyId != null)
                NeobrutalistButton(
                  label: 'Copiar ID',
                  icon: Icons.tag_rounded,
                  variant: NeobrutalistButtonVariant.secondary,
                  onPressed: onCopyId,
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class _SearchMessageCard extends StatelessWidget {
  const _SearchMessageCard({
    required this.icon,
    required this.title,
    required this.message,
    this.tone = NeobrutalistTone.neutral,
  });

  factory _SearchMessageCard.fromError(NotesApiException e) {
    if (e.statusCode == 410 || e.code == 'note_deleted') {
      return _SearchMessageCard(
        icon: Icons.delete_forever_rounded,
        title: 'Apunte no disponible',
        message: e.message,
        tone: NeobrutalistTone.pending,
      );
    }
    if (e.statusCode == 404 && e.code != 'note_unavailable') {
      return const _SearchMessageCard(
        icon: Icons.search_off_rounded,
        title: 'Sin resultados',
        message:
            'No existe un apunte con ese ID o no tienes permiso para verlo.',
        tone: NeobrutalistTone.danger,
      );
    }
    return _SearchMessageCard(
      icon: Icons.error_outline_rounded,
      title: e.code == 'invalid_id' ? 'ID inválido' : 'No se pudo buscar',
      message: e.message,
      tone: NeobrutalistTone.danger,
    );
  }

  final IconData icon;
  final String title;
  final String message;
  final NeobrutalistTone tone;

  @override
  Widget build(BuildContext context) {
    return NeobrutalistCard(
      key: const Key('note-search-message'),
      backgroundColor: AppColors.surface,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Container(
            padding: const EdgeInsets.all(AppDimens.spaceSm),
            decoration: BoxDecoration(
              color: tone.background,
              border: Border.all(color: AppColors.border, width: 1.5),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            ),
            child: Icon(icon, color: tone.foreground, size: 22),
          ),
          const SizedBox(width: AppDimens.spaceMd),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  title.toUpperCase(),
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.6,
                    color: AppColors.text,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceXs),
                Text(
                  message,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: AppColors.mutedStrong,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
