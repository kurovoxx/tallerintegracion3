import 'package:flutter/material.dart';

import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';

/// Qué permiso de grupo controla el switch.
enum GroupPermission { shareNotes, createMeetings, editKanban }

/// Switch SOLO PARA ADMINS que habilita (o retira) un permiso para TODOS los
/// miembros del grupo: compartir notas, agendar reuniones o editar el
/// kanban/sprint. Para miembros comunes no dibuja nada.
///
/// El estado se comparte entre instancias (p. ej. Sprint y Kanban usan el mismo
/// permiso) mediante un notificador por grupo+permiso.
class GroupPermissionSwitch extends StatefulWidget {
  const GroupPermissionSwitch({
    super.key,
    required this.groupId,
    required this.permission,
    required this.label,
    SocialService? service,
  }) : _serviceOverride = service;

  final String? groupId;
  final GroupPermission permission;

  /// Texto corto en mayúsculas, p. ej. "TODOS PUEDEN AGENDAR".
  final String label;
  final SocialService? _serviceOverride;

  @override
  State<GroupPermissionSwitch> createState() => _GroupPermissionSwitchState();
}

class _GroupPermissionSwitchState extends State<GroupPermissionSwitch> {
  static final Map<String, ValueNotifier<bool>> _shared = {};

  late final SocialService _service;
  ValueNotifier<bool>? _value;
  bool _isAdmin = false;
  bool _saving = false;

  String get _groupId => widget.groupId?.trim() ?? '';

  String get _key => '$_groupId|${widget.permission.name}';

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? SocialService();
    _load();
  }

  @override
  void dispose() {
    _value?.removeListener(_onShared);
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  void _onShared() {
    if (mounted) setState(() {});
  }

  bool _read(GroupMemberDefaults d) => switch (widget.permission) {
    GroupPermission.shareNotes => d.canShareNotes,
    GroupPermission.createMeetings => d.canCreateMeetings,
    GroupPermission.editKanban => d.canEditKanban,
  };

  Future<void> _load() async {
    if (_groupId.isEmpty) return;
    try {
      final detail = await _service.getGroup(_groupId);
      if (detail.role != 'admin') return;
      final defaults = await _service.getGroupPermissions(_groupId);
      if (!mounted) return;
      final notifier = _shared.putIfAbsent(_key, () => ValueNotifier(false));
      notifier.value = _read(defaults);
      notifier.addListener(_onShared);
      setState(() {
        _isAdmin = true;
        _value = notifier;
      });
    } catch (_) {
      // Sin datos no hay switch: el control solo aparece cuando se pudo
      // confirmar que quien mira es admin y conocer el estado real.
    }
  }

  Future<void> _toggle(bool next) async {
    final notifier = _value;
    if (notifier == null || _saving) return;
    final previous = notifier.value;
    setState(() => _saving = true);
    notifier.value = next;
    try {
      await _service.setGroupPermissions(
        groupId: _groupId,
        canShareNotes: widget.permission == GroupPermission.shareNotes
            ? next
            : null,
        canCreateMeetings: widget.permission == GroupPermission.createMeetings
            ? next
            : null,
        canEditKanban: widget.permission == GroupPermission.editKanban
            ? next
            : null,
      );
    } catch (e) {
      notifier.value = previous;
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              e is SocialApiException
                  ? e.message
                  : 'No se pudo cambiar el permiso.',
            ),
            backgroundColor: AppColors.error,
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final notifier = _value;
    if (!_isAdmin || notifier == null) return const SizedBox.shrink();
    final on = notifier.value;
    return Semantics(
      container: true,
      label: widget.label,
      toggled: on,
      child: Container(
        padding: const EdgeInsets.only(
          left: AppDimens.spaceMd,
          right: AppDimens.spaceXs,
        ),
        decoration: BoxDecoration(
          color: on ? AppColors.accentYellow : AppColors.surface,
          border: Border.all(
            color: AppColors.outline,
            width: AppDimens.borderWidth,
          ),
          boxShadow: AppShadows.badge,
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Flexible(
              child: Text(
                widget.label,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontWeight: FontWeight.w900,
                  fontSize: 11,
                  letterSpacing: 0.5,
                  color: AppColors.text,
                ),
              ),
            ),
            const SizedBox(width: AppDimens.spaceXs),
            Switch(
              value: on,
              activeThumbColor: AppColors.text,
              activeTrackColor: AppColors.surface,
              inactiveThumbColor: AppColors.text,
              inactiveTrackColor: AppColors.surfaceLow,
              trackOutlineColor: WidgetStatePropertyAll(AppColors.border),
              onChanged: _saving ? null : _toggle,
            ),
          ],
        ),
      ),
    );
  }
}
