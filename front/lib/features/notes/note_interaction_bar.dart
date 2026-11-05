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
  });

  final NotesApi api;
  final NoteInteractions initial;

  /// Se llama con el estado confirmado por el servidor.
  final ValueChanged<NoteInteractions>? onChanged;

  /// Variante compacta para tarjetas en lista.
  final bool compact;

  @override
  State<NoteLikeSaveBar> createState() => _NoteLikeSaveBarState();
}

class _NoteLikeSaveBarState extends State<NoteLikeSaveBar> {
  late NoteInteractions _state = widget.initial;
  bool _likeBusy = false;
  bool _saveBusy = false;

  @override
  void didUpdateWidget(covariant NoteLikeSaveBar oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.initial.noteId != widget.initial.noteId) {
      _state = widget.initial;
    }
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
          style: const TextStyle(
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
      final confirmed = await widget.api.setLiked(before.noteId, liked);
      if (!mounted) return;
      setState(() => _state = confirmed);
      widget.onChanged?.call(confirmed);
    } catch (e) {
      if (!mounted) return;
      setState(() => _state = before);
      _error(e);
    } finally {
      if (mounted) setState(() => _likeBusy = false);
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
      final confirmed = await widget.api.setSaved(before.noteId, saved);
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
            style: const TextStyle(
              color: AppColors.surface,
              fontWeight: FontWeight.w800,
            ),
          ),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      setState(() => _state = before);
      _error(e);
    } finally {
      if (mounted) setState(() => _saveBusy = false);
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
