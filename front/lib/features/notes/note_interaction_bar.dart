import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'notes_api.dart';

/// Botones de like y guardar de un apunte, con contadores exactos.
///
/// Aplica el cambio de forma optimista (feedback < 100 ms), lo envía al
/// backend y luego adopta los contadores reales que devuelve el servidor. Si
/// la petición falla, revierte y muestra el motivo.
class NoteLikeSaveBar extends StatefulWidget {
  const NoteLikeSaveBar({
    super.key,
    required this.api,
    required this.initial,
    this.onChanged,
    this.compact = false,
    this.showSave = true,
  });

  final NotesApi api;
  final NoteInteractions initial;

  /// Se llama con el estado confirmado por el servidor.
  final ValueChanged<NoteInteractions>? onChanged;

  /// Variante compacta para tarjetas en lista.
  final bool compact;

  /// false oculta el botón de guardar (p. ej. en la lista de guardados, que
  /// tiene su propio "Quitar de guardados").
  final bool showSave;

  @override
  State<NoteLikeSaveBar> createState() => _NoteLikeSaveBarState();
}

class _NoteLikeSaveBarState extends State<NoteLikeSaveBar> {
  late NoteInteractions _state = widget.initial;
  bool _likeBusy = false;
  bool _saveBusy = false;

  /// El padre entregó un estado confirmado nuevo (p. ej. desde la vista del
  /// apunte) mientras había una petición propia en vuelo. La respuesta de esa
  /// petición es entonces una foto del servidor posiblemente ANTERIOR, así
  /// que no se aplica tal cual: se reconcilia con [_reconcile].
  bool _parentUpdatedWhileBusy = false;

  bool get _busy => _likeBusy || _saveBusy;

  @override
  void didUpdateWidget(covariant NoteLikeSaveBar oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.initial.noteId != widget.initial.noteId) {
      _state = widget.initial;
      _parentUpdatedWhileBusy = false;
      return;
    }
    if (sameNoteInteractions(oldWidget.initial, widget.initial)) return;
    // El padre actualizó el estado (p. ej. al volver de la vista del apunte,
    // donde se dio like o se guardó). Sin peticiones propias se adopta ya;
    // con una en vuelo se marca para reconciliar cuando responda.
    if (_busy) {
      _parentUpdatedWhileBusy = true;
    } else {
      _state = widget.initial;
    }
  }

  /// Decide qué estado aplicar cuando responde una petición propia. Si el
  /// padre no cambió durante el vuelo, la respuesta es la verdad más reciente.
  /// Si cambió, se relee `GET /notes/{id}/stats` (una sola petición, solo en
  /// esta carrera) para no pisar el estado confirmado más nuevo con una foto
  /// antigua. Si esa lectura falla, se combina: el campo que tocó esta
  /// petición sale de su respuesta y el resto del estado más reciente del
  /// padre.
  Future<NoteInteractions> _reconcile(
    NoteInteractions confirmed, {
    required bool like,
  }) async {
    if (!_parentUpdatedWhileBusy) return confirmed;
    try {
      return await widget.api.stats(confirmed.noteId);
    } catch (_) {
      final latest = widget.initial;
      return like
          ? latest.copyWith(
              isLiked: confirmed.isLiked,
              likesCount: confirmed.likesCount,
            )
          : latest.copyWith(
              isSaved: confirmed.isSaved,
              savesCount: confirmed.savesCount,
            );
    }
  }

  /// Estado al que volver si una petición falla: el del padre si cambió
  /// durante el vuelo (más reciente que el previo a la petición).
  NoteInteractions _rollbackTo(NoteInteractions before) =>
      _parentUpdatedWhileBusy ? widget.initial : before;

  void _endRequest() {
    if (!_busy) _parentUpdatedWhileBusy = false;
  }

  void _error(Object e) {
    if (!mounted) return;
    final message = e is NotesApiException
        ? e.message
        : notesFriendlyMessage(-1, null);
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: AppColors.errorDeep,
        content: Text(
          message,
          style: TextStyle(
            color: AppColors.surface,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  Future<void> _toggleLike() async {
    if (_likeBusy) return;
    final before = _state;
    final liked = !before.isLiked;
    setState(() {
      _likeBusy = true;
      _state = before.copyWith(
        isLiked: liked,
        likesCount: (before.likesCount + (liked ? 1 : -1)).clamp(0, 1 << 30),
      );
    });
    try {
      final response = await widget.api.setLiked(before.noteId, liked);
      final confirmed = await _reconcile(response, like: true);
      if (!mounted) return;
      setState(() => _state = confirmed);
      widget.onChanged?.call(confirmed);
    } catch (e) {
      if (!mounted) return;
      setState(() => _state = _rollbackTo(before));
      _error(e);
    } finally {
      if (mounted) setState(() => _likeBusy = false);
      _endRequest();
    }
  }

  Future<void> _toggleSave() async {
    if (_saveBusy) return;
    final before = _state;
    final saved = !before.isSaved;
    setState(() {
      _saveBusy = true;
      _state = before.copyWith(
        isSaved: saved,
        savesCount: (before.savesCount + (saved ? 1 : -1)).clamp(0, 1 << 30),
      );
    });
    try {
      final response = await widget.api.setSaved(before.noteId, saved);
      final confirmed = response == null
          ? null
          : await _reconcile(response, like: false);
      if (!mounted) return;
      final next = confirmed ?? _state;
      setState(() => _state = next);
      widget.onChanged?.call(next);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: AppColors.border,
          duration: const Duration(seconds: 1),
          content: Text(
            saved ? 'Apunte guardado' : 'Quitado de guardados',
            style: TextStyle(
              color: AppColors.surface,
              fontWeight: FontWeight.w800,
            ),
          ),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      setState(() => _state = _rollbackTo(before));
      _error(e);
    } finally {
      if (mounted) setState(() => _saveBusy = false);
      _endRequest();
    }
  }

  @override
  Widget build(BuildContext context) {
    final padding = widget.compact
        ? const EdgeInsets.symmetric(
            horizontal: AppDimens.spaceSm,
            vertical: AppDimens.spaceXs + 2,
          )
        : const EdgeInsets.symmetric(
            horizontal: AppDimens.spaceMd,
            vertical: AppDimens.spaceSm,
          );
    return Wrap(
      spacing: AppDimens.spaceSm,
      runSpacing: AppDimens.spaceSm,
      children: <Widget>[
        NeobrutalistButton(
          key: Key('like-${_state.noteId}'),
          label: '${_state.likesCount}',
          icon: _state.isLiked
              ? Icons.favorite_rounded
              : Icons.favorite_border_rounded,
          variant: _state.isLiked
              ? NeobrutalistButtonVariant.danger
              : NeobrutalistButtonVariant.secondary,
          padding: padding,
          semanticLabel: _state.isLiked ? 'Quitar like' : 'Dar like',
          onPressed: _likeBusy ? null : _toggleLike,
        ),
        if (widget.showSave)
          NeobrutalistButton(
            key: Key('save-${_state.noteId}'),
            label: _state.isSaved
                ? 'Guardado · ${_state.savesCount}'
                : 'Guardar · ${_state.savesCount}',
            icon: _state.isSaved
                ? Icons.bookmark_rounded
                : Icons.bookmark_border_rounded,
            variant: _state.isSaved
                ? NeobrutalistButtonVariant.accent
                : NeobrutalistButtonVariant.secondary,
            padding: padding,
            semanticLabel: _state.isSaved ? 'Quitar de guardados' : 'Guardar',
            onPressed: _saveBusy ? null : _toggleSave,
          ),
      ],
    );
  }
}
