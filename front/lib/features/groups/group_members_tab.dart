import 'package:flutter/material.dart';

import '../../core/models/profile_models.dart';
import '../../core/models/social_models.dart';
import '../../core/services/profile_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../../core/widgets/user_avatar.dart';
import 'cleanup_shared_notes_checkbox.dart';

/// Pestaña "MIEMBROS" del grupo: lista integrantes con su rol, permite ver su
/// información y, si el usuario actual es admin, gestionar roles (hacer/quitar
/// admin, transferir) y expulsar o banear a miembros (endpoints de Social).
/// Estilo Neo-Brutalism.
class GroupMembersTab extends StatefulWidget {
  const GroupMembersTab({super.key, this.groupId, SocialService? service})
    : _serviceOverride = service;

  final String? groupId;
  final SocialService? _serviceOverride;

  @override
  State<GroupMembersTab> createState() => _GroupMembersTabState();
}

class _GroupMembersTabState extends State<GroupMembersTab> {
  late final SocialService _service;
  late final ProfileService _profileService;

  bool _loading = false;
  String? _error;
  List<GroupMember> _members = const [];
  String? _busyUserId;
  Map<String, bool> _followingStatus = {}; // userId -> following

  /// Permisos habilitados para TODO el grupo (switch de admin en cada pestaña).
  /// Se suman al permiso propio de cada miembro: efectivo = propio OR grupo.
  GroupMemberDefaults _groupDefaults = const GroupMemberDefaults();

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? SocialService();
    _profileService = widget._serviceOverride == null
        ? ProfileService()
        : ProfileService();
    _load();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _service.dispose();
    _profileService.dispose();
    super.dispose();
  }

  String get _groupId => widget.groupId?.trim() ?? '';
  String? get _me => SessionManager.currentUserId;

  bool get _isAdmin {
    final me = _me;
    if (me == null) return false;
    return _members.any((m) => m.userId == me && m.role == 'admin');
  }

  void _snack(String message, {bool error = false}) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: error ? AppColors.errorDeep : null,
      ),
    );
  }

  Future<void> _load() async {
    final gid = _groupId;
    if (gid.isEmpty) {
      setState(() {
        _loading = false;
        _error = 'Grupo no disponible.';
        _members = const [];
      });
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait<Object>([
        _service.listMembers(gid),
        _fetchGroupDefaults(gid),
      ]);
      final members = results[0] as List<GroupMember>;
      members.sort((a, b) {
        if (a.role != b.role) return a.role == 'admin' ? -1 : 1;
        return a.displayLabel.toLowerCase().compareTo(
          b.displayLabel.toLowerCase(),
        );
      });
      if (!mounted) return;
      setState(() {
        _members = members;
        _groupDefaults = results[1] as GroupMemberDefaults;
        _loading = false;
      });
      _loadFollowStatuses();
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = e.message;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = 'No se pudieron cargar los integrantes.';
      });
    }
  }

  /// Permisos del grupo, best-effort: si falla (p. ej. Social sin actualizar)
  /// se asume que no hay nada habilitado para todos.
  Future<GroupMemberDefaults> _fetchGroupDefaults(String gid) async {
    try {
      return await _service.getGroupPermissions(gid);
    } catch (_) {
      return const GroupMemberDefaults();
    }
  }

  /// Carga el estado de follow de cada miembro que tenga código público.
  Future<void> _loadFollowStatuses() async {
    if (!mounted) return;
    final updates = <String, bool>{};
    for (final m in _members) {
      final pid = m.publicId?.trim();
      if (pid == null || pid.isEmpty) continue;
      try {
        final status = await _profileService.getFollowStatus(pid);
        if (!mounted) return;
        updates[pid] = status.following;
        // Mapear por userId para el UI
        _followingStatus[m.userId] = status.following;
      } catch (_) {
        // No bloquea la lista si falla el estado de follow de un miembro.
      }
    }
    if (!mounted) return;
    setState(() => _followingStatus = {..._followingStatus, ...updates});
  }

  /// Alterna seguir/dejar de seguir a un miembro del grupo.
  Future<void> _toggleFollowMember(GroupMember m) async {
    final pid = m.publicId?.trim();
    if (pid == null || pid.isEmpty) {
      _snack('Este integrante no tiene código de perfil disponible.');
      return;
    }
    final currentlyFollowing = _followingStatus[m.userId] ?? false;
    setState(() {
      _followingStatus = Map<String, bool>.from(_followingStatus)
        ..[m.userId] = !currentlyFollowing;
    });
    try {
      if (currentlyFollowing) {
        await _profileService.unfollowUser(pid);
      } else {
        await _profileService.followUser(pid);
      }
      if (!mounted) return;
      _snack(
        currentlyFollowing ? 'Has dejado de seguir a ${m.displayLabel}.'
            : 'Ahora sigues a ${m.displayLabel}.',
      );
    } catch (_) {
      if (!mounted) return;
      // Revertir el cambio optimista en caso de error.
      setState(() {
        _followingStatus = Map<String, bool>.from(_followingStatus)
          ..[m.userId] = currentlyFollowing;
      });
      _snack('No se pudo actualizar el seguimiento.', error: true);
    }
  }

  Future<void> _showInfo(GroupMember m) async {
    await showNeobrutalistDialog<void>(
      context: context,
      dialog: NeobrutalistDialog(
        title: 'Ficha de integrante',
        cancelLabel: 'Cerrar',
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _field('Nombre', m.displayLabel),
            _field(
              'Correo',
              m.email?.trim().isNotEmpty == true ? m.email! : '—',
            ),
            _field('Rol', m.role == 'admin' ? 'Admin' : 'Miembro'),
            _field(
              'Se unió',
              m.joinedAt != null
                  ? '${m.joinedAt!.toLocal()}'.split('.').first
                  : '—',
            ),
            // Código público del perfil (el mismo que se usa en «Buscar
            // perfil»); nunca el identificador interno.
            _field(
              'Código de perfil',
              m.publicId ?? 'No disponible',
              mono: m.publicId != null,
            ),
          ],
        ),
      ),
    );
  }

  Widget _field(String label, String value, {bool mono = false}) {
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label.toUpperCase(),
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.5,
              color: AppColors.muted,
            ),
          ),
          const SizedBox(height: AppDimens.spaceXs),
          SelectableText(
            value,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w700,
              color: AppColors.text,
              fontFamily: mono ? 'monospace' : null,
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _confirmModeration(GroupMember m, {required bool ban}) async {
    var cleanupSharedNotes =
        true; // Marcada por defecto; el usuario puede quitarla.
    final cleanup = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: NeobrutalistDialog(
        title: ban ? 'Banear integrante' : 'Expulsar integrante',
        cancelLabel: 'Cancelar',
        confirmLabel: 'Confirmar',
        confirmVariant: NeobrutalistButtonVariant.danger,
        closeOnConfirm: false,
        onConfirm: () => Navigator.of(context).pop(cleanupSharedNotes),
        content: StatefulBuilder(
          builder: (ctx, setDialogState) => Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                ban
                    ? '¿Banear a ${m.displayLabel}? No podrá reingresar al grupo ni con un enlace válido.'
                    : '¿Expulsar a ${m.displayLabel} del grupo? Podrá volver con una invitación.',
              ),
              const SizedBox(height: AppDimens.spaceMd),
              CleanupSharedNotesCheckbox(
                value: cleanupSharedNotes,
                onChanged: (v) => setDialogState(() => cleanupSharedNotes = v),
              ),
            ],
          ),
        ),
      ),
    );
    if (cleanup == null || !mounted) return;
    await _perform(
      m,
      ban ? 'Integrante baneado.' : 'Integrante expulsado.',
      () => ban
          ? _service.banMember(
              groupId: _groupId,
              userId: m.userId,
              cleanupSharedNotes: cleanup,
            )
          : _service.kickMember(
              groupId: _groupId,
              userId: m.userId,
              cleanupSharedNotes: cleanup,
            ),
    );
  }

  static const Widget _inheritedNote = Text(
    'Habilitado para todo el grupo (puedes quitarlo a este integrante)',
    style: TextStyle(
      fontSize: 11,
      fontWeight: FontWeight.w700,
      color: AppColors.mutedStrong,
    ),
  );

  // Diálogo de permisos delegados (PATCH permissions): el admin elige qué
  // puede hacer el miembro además de leer y chatear.
  Future<void> _editPermissions(GroupMember m) async {
    // El admin pudo cambiar un switch de grupo desde otra pestaña: refrescar.
    final inherited = await _fetchGroupDefaults(_groupId);
    if (!mounted) return;
    setState(() => _groupDefaults = inherited);
    // Permiso efectivo de partida: propio OR grupo, salvo retiro individual.
    final initialShare = m.sharesNotesWith(inherited);
    final initialMeetings = m.createsMeetingsWith(inherited);
    final initialKanban = m.editsKanbanWith(inherited);
    var share = initialShare;
    var meetings = initialMeetings;
    var kanban = initialKanban;
    final result = await showNeobrutalistDialog<(bool, bool, bool)>(
      context: context,
      dialog: NeobrutalistDialog(
        title: 'Permisos de ${m.displayLabel}',
        cancelLabel: 'Cancelar',
        confirmLabel: 'Guardar',
        confirmVariant: NeobrutalistButtonVariant.accent,
        closeOnConfirm: false,
        onConfirm: () => Navigator.of(context).pop((share, meetings, kanban)),
        content: StatefulBuilder(
          builder: (ctx, setDialogState) => Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text(
                'Los admins pueden todo. Aquí eliges qué más puede hacer este miembro.',
              ),
              const SizedBox(height: AppDimens.spaceSm),
              // Lo habilitado para todo el grupo aparece marcado, pero sigue
              // siendo editable: desmarcarlo lo retira solo a este miembro.
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                controlAffinity: ListTileControlAffinity.leading,
                title: const Text('Enviar notas al grupo'),
                subtitle: inherited.canShareNotes ? _inheritedNote : null,
                value: share,
                onChanged: (v) => setDialogState(() => share = v ?? false),
              ),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                controlAffinity: ListTileControlAffinity.leading,
                title: const Text('Crear reuniones'),
                subtitle: inherited.canCreateMeetings ? _inheritedNote : null,
                value: meetings,
                onChanged: (v) => setDialogState(() => meetings = v ?? false),
              ),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                controlAffinity: ListTileControlAffinity.leading,
                title: const Text('Modificar el kanban'),
                subtitle: inherited.canEditKanban ? _inheritedNote : null,
                value: kanban,
                onChanged: (v) => setDialogState(() => kanban = v ?? false),
              ),
            ],
          ),
        ),
      ),
    );
    if (result == null || !mounted) return;
    final (s, mt, k) = result;
    if (s == initialShare && mt == initialMeetings && k == initialKanban) {
      return;
    }
    // Solo se envían los permisos que cambiaron: el backend deduce si hay que
    // denegarlos a este miembro o basta con retirar el permiso propio.
    await _perform(
      m,
      'Permisos de ${m.displayLabel} actualizados.',
      () => _service.setMemberPermissions(
        groupId: _groupId,
        userId: m.userId,
        canShareNotes: s == initialShare ? null : s,
        canCreateMeetings: mt == initialMeetings ? null : mt,
        canEditKanban: k == initialKanban ? null : k,
      ),
    );
  }

  // Ascender, degradar o transferir la administración (PATCH role /
  // POST transfer-admin). Tras transferir, el usuario actual pasa a miembro.
  Future<void> _confirmRole(GroupMember m, {required String action}) async {
    final name = m.displayLabel;
    final (title, message, success, call) = switch (action) {
      'promote' => (
        'Hacer admin',
        '¿Dar permisos de administración a $name? Seguirás siendo admin.',
        '$name ahora es admin.',
        () => _service.changeMemberRole(
          groupId: _groupId,
          userId: m.userId,
          role: 'admin',
        ),
      ),
      'demote' => (
        'Quitar admin',
        '¿Quitar los permisos de administración a $name? Quedará como miembro.',
        '$name ahora es miembro.',
        () => _service.changeMemberRole(
          groupId: _groupId,
          userId: m.userId,
          role: 'member',
        ),
      ),
      _ => (
        'Transferir admin',
        '¿Transferir tu rol de admin a $name? Pasarás a ser miembro y perderás '
            'los permisos de gestión.',
        'Administración transferida a $name.',
        () =>
            _service.transferAdmin(groupId: _groupId, newAdminUserId: m.userId),
      ),
    };
    final ok = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: NeobrutalistDialog(
        title: title,
        cancelLabel: 'Cancelar',
        confirmLabel: 'Confirmar',
        confirmVariant: action == 'transfer'
            ? NeobrutalistButtonVariant.danger
            : NeobrutalistButtonVariant.accent,
        closeOnConfirm: false,
        onConfirm: () => Navigator.of(context).pop(true),
        content: Text(message),
      ),
    );
    if (ok != true || !mounted) return;
    await _perform(m, success, call);
  }

  Future<void> _perform(
    GroupMember m,
    String success,
    Future<void> Function() call,
  ) async {
    setState(() => _busyUserId = m.userId);
    try {
      await call();
      if (!mounted) return;
      await _load();
      _snack(success);
    } on SocialApiException catch (e) {
      _snack(e.message, error: true);
    } catch (_) {
      _snack('No se pudo completar la acción.', error: true);
    } finally {
      if (mounted) setState(() => _busyUserId = null);
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
      child: Row(
        children: [
          const Icon(Icons.group_rounded, size: 16, color: AppColors.text),
          const SizedBox(width: AppDimens.spaceSm),
          Expanded(
            child: Text(
              'MIEMBROS (${_members.length})',
              style: const TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 12,
                letterSpacing: 0.5,
              ),
            ),
          ),
          if (_isAdmin)
            const NeobrutalistBadge(
              label: 'Eres admin',
              tone: NeobrutalistTone.accent,
              icon: Icons.shield_rounded,
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
    if (_members.isEmpty) {
      return const Center(
        child: Text(
          'Este grupo no tiene integrantes.',
          style: TextStyle(fontWeight: FontWeight.w700),
        ),
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      itemCount: _members.length,
      itemBuilder: (_, i) => _memberCard(_members[i]),
    );
  }

  Widget _memberCard(GroupMember m) {
    final isAdmin = m.role == 'admin';
    final isMe = m.userId == _me;
    final busy = _busyUserId == m.userId;
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      child: NeobrutalistCard(
        padding: const EdgeInsets.all(AppDimens.spaceMd),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _avatar(m),
                const SizedBox(width: AppDimens.spaceMd),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        m.displayLabel,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 14,
                        ),
                      ),
                      if (m.email?.trim().isNotEmpty == true)
                        Text(
                          m.email!,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontSize: 12,
                            fontWeight: FontWeight.w600,
                            color: AppColors.muted,
                          ),
                        ),
                      const SizedBox(height: AppDimens.spaceSm),
                      Wrap(
                        spacing: AppDimens.spaceSm,
                        runSpacing: AppDimens.spaceXs,
                        children: [
                          NeobrutalistBadge(
                            label: isAdmin ? 'Admin' : 'Miembro',
                            tone: isAdmin
                                ? NeobrutalistTone.accent
                                : NeobrutalistTone.neutral,
                            icon: isAdmin ? Icons.shield_rounded : null,
                          ),
                          if (isMe)
                            const NeobrutalistBadge(
                              label: 'Tú',
                              tone: NeobrutalistTone.info,
                            ),
                          if (!isAdmin && m.sharesNotesWith(_groupDefaults))
                            const NeobrutalistBadge(
                              label: 'Notas',
                              tone: NeobrutalistTone.neutral,
                              icon: Icons.note_add_rounded,
                            ),
                          if (!isAdmin &&
                              m.createsMeetingsWith(_groupDefaults))
                            const NeobrutalistBadge(
                              label: 'Reuniones',
                              tone: NeobrutalistTone.neutral,
                              icon: Icons.event_rounded,
                            ),
                          if (!isAdmin && m.editsKanbanWith(_groupDefaults))
                            const NeobrutalistBadge(
                              label: 'Kanban',
                              tone: NeobrutalistTone.neutral,
                              icon: Icons.view_kanban_rounded,
                            ),
                        ],
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: AppDimens.spaceMd),
            Wrap(
              spacing: AppDimens.spaceSm,
              runSpacing: AppDimens.spaceSm,
              children: [
                _action(
                  'Ver',
                  Icons.badge_rounded,
                  NeobrutalistButtonVariant.secondary,
                  () => _showInfo(m),
                ),
                // Seguir / dejar de seguir (solo si no es uno mismo y tiene código de perfil)
                if (!isMe && m.publicId?.trim().isNotEmpty == true)
                  _action(
                    _followingStatus[m.userId] == true ? 'Siguiendo' : 'Seguir',
                    _followingStatus[m.userId] == true
                        ? Icons.person_remove_rounded
                        : Icons.person_add_rounded,
                    _followingStatus[m.userId] == true
                        ? NeobrutalistButtonVariant.secondary
                        : NeobrutalistButtonVariant.info,
                    _busyUserId == m.userId
                        ? null
                        : () => _toggleFollowMember(m),
                  ),
                // Social rechaza (400 target_is_admin) expulsar o banear a
                // otro admin: primero hay que quitarle el rol.
                if (_isAdmin && !isMe && isAdmin)
                  _action(
                    'Quitar admin',
                    Icons.remove_moderator_rounded,
                    NeobrutalistButtonVariant.secondary,
                    busy ? null : () => _confirmRole(m, action: 'demote'),
                  ),
                if (_isAdmin && !isMe && !isAdmin) ...[
                  _action(
                    'Permisos',
                    Icons.tune_rounded,
                    NeobrutalistButtonVariant.accent,
                    busy ? null : () => _editPermissions(m),
                  ),
                  _action(
                    'Hacer admin',
                    Icons.add_moderator_rounded,
                    NeobrutalistButtonVariant.info,
                    busy ? null : () => _confirmRole(m, action: 'promote'),
                  ),
                  _action(
                    'Transferir admin',
                    Icons.swap_horiz_rounded,
                    NeobrutalistButtonVariant.secondary,
                    busy ? null : () => _confirmRole(m, action: 'transfer'),
                  ),
                  _action(
                    'Expulsar',
                    Icons.person_remove_rounded,
                    NeobrutalistButtonVariant.secondary,
                    busy ? null : () => _confirmModeration(m, ban: false),
                  ),
                  _action(
                    'Banear',
                    Icons.block_rounded,
                    NeobrutalistButtonVariant.danger,
                    busy ? null : () => _confirmModeration(m, ban: true),
                  ),
                ],
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _action(
    String label,
    IconData icon,
    NeobrutalistButtonVariant variant,
    VoidCallback? onPressed,
  ) {
    return NeobrutalistButton(
      label: label,
      icon: icon,
      variant: variant,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceMd,
        vertical: AppDimens.spaceSm,
      ),
      onPressed: onPressed,
    );
  }

  Widget _avatar(GroupMember m) {
    final isAdmin = m.role == 'admin';
    return UserAvatar(
      name: m.displayLabel,
      photoUrl: m.photoThumbUrl,
      size: 44,
      backgroundColor: isAdmin
          ? AppColors.accentBlueDeep
          : AppColors.surfaceLow,
      foregroundColor: isAdmin ? AppColors.surface : AppColors.text,
      borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
      boxShadow: AppShadows.badge,
    );
  }
}
