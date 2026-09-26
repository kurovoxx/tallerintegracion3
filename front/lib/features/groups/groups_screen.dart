// Ruta: front/lib/features/groups/groups_screen.dart
//
// 'Credenciales / Carpetas de Proyecto': tarjetas de grupo con datos REALES
// de GET /me/overview (Benjamín). Ver:
// - back/social/internal/handler/http/view_handler.go:29 MyOverview
// - back/social/internal/model/views.go:36-42 MyOverview/GroupCard
// - back/social/cmd/server/main.go:186 RegisterRoutes
// No inventa subject/avatares/correos: solo group_id, name, description,
// role, member_count, joined_at + stats reales.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'group_detail_screen.dart';

class GroupsScreen extends StatefulWidget {
  const GroupsScreen({super.key, SocialService? service})
      : _serviceOverride = service;

  final SocialService? _serviceOverride;

  @visibleForTesting
  SocialService? get serviceOverride => _serviceOverride;

  @override
  State<GroupsScreen> createState() => _GroupsScreenState();
}

class _GroupsScreenState extends State<GroupsScreen> {
  late final SocialService _service;
  bool _isLoading = true;
  bool _isCreating = false;
  String? _error;
  Overview? _overview;

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? SocialService();
    _load();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (!mounted) return;
    setState(() {
      _isLoading = true;
      _error = null;
    });
    try {
      final ov = await _service.getOverview();
      if (!mounted) return;
      setState(() {
        _overview = ov;
        _isLoading = false;
      });
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _isLoading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudo cargar tus grupos: $e';
        _isLoading = false;
      });
    }
  }

  Future<void> _openCreateSheet() async {
    final draft = await showModalBottomSheet<_NewGroupDraft>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.scrim,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      builder: (_) => const _NewGroupSheet(),
    );
    if (draft == null || !mounted) return;
    setState(() => _isCreating = true);
    try {
      // Contrato real: POST /groups {name, description?} -> 201 {group_id}
      // Ver back/social/internal/handler/http/group_handler.go:83-105.
      final groupId = await _service.createGroup(
        name: draft.name,
        description:
            draft.description.isEmpty ? null : draft.description,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('Grupo creado: $groupId'),
          backgroundColor: AppColors.border,
        ),
      );
      await _load();
    } on SocialApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('No se pudo crear: $e'),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _isCreating = false);
    }
  }

  void _showGroupInfo(GroupCard group) {
    // No se finge envío de invitación: el invite_token solo lo ve admin vía
    // GET /groups/:id (group_handler.go:119). Aquí solo se informa rol real.
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          '${group.name} · rol ${group.role} · ${group.memberCount} integrantes. '
          'La invitación por token se gestiona en el detalle (solo admin).',
        ),
        backgroundColor: AppColors.border,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final breakpoint = context.breakpoint;
    final compact = breakpoint == AppBreakpoint.compact;

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: MaxWidthContainer(
          maxWidth: AppDimens.contentMaxWidth,
          padding: EdgeInsets.fromLTRB(
            compact ? AppDimens.spaceLg : AppDimens.spaceXl,
            AppDimens.spaceXl,
            compact ? AppDimens.spaceLg : AppDimens.spaceXl,
            AppDimens.spaceXxl,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _buildHeader(compact),
              const SizedBox(height: AppDimens.spaceXl),
              Expanded(child: _buildContent(breakpoint, compact)),
            ],
          ),
        ),
      ),
      floatingActionButton: compact
          ? NeobrutalistFab(
              icon: Icons.group_add_rounded,
              tooltip: 'Nuevo grupo',
              onPressed: _isCreating ? null : _openCreateSheet,
            )
          : null,
    );
  }

  Widget _buildContent(AppBreakpoint breakpoint, bool compact) {
    if (_isLoading) {
      return const Center(
        child: CircularProgressIndicator(color: AppColors.text),
      );
    }
    if (_error != null) {
      return Center(
        child: Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(
                color: AppColors.border, width: AppDimens.borderWidth),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.card,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded,
                  size: 36, color: AppColors.text),
              const SizedBox(height: 12),
              const Text('NO SE PUDO CARGAR TUS GRUPOS',
                  style: TextStyle(
                      fontWeight: FontWeight.w900,
                      fontSize: 14,
                      color: AppColors.text)),
              const SizedBox(height: 8),
              Text(_error!,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                      fontWeight: FontWeight.w600,
                      fontSize: 12,
                      color: AppColors.mutedStrong)),
              const SizedBox(height: 14),
              NeobrutalistButton(
                label: 'Reintentar',
                icon: Icons.refresh_rounded,
                variant: NeobrutalistButtonVariant.accent,
                onPressed: _load,
              ),
            ],
          ),
        ),
      );
    }
    final groups = _overview?.groups ?? const <GroupCard>[];
    if (groups.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.groups_rounded,
                size: 40, color: AppColors.mutedStrong),
            const SizedBox(height: 12),
            const Text('NO TIENES GRUPOS TODAVÍA',
                style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 14,
                    color: AppColors.text)),
            const SizedBox(height: 6),
            const Text('Crea tu primer grupo y aparecerá aquí',
                style: TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 12,
                    color: AppColors.mutedStrong)),
            const SizedBox(height: 14),
            NeobrutalistButton(
              label: 'Nuevo grupo',
              icon: Icons.add_rounded,
              variant: NeobrutalistButtonVariant.accent,
              onPressed: _openCreateSheet,
            ),
          ],
        ),
      );
    }
    return LayoutBuilder(
      builder: (context, constraints) {
        final columns = breakpoint == AppBreakpoint.expanded
            ? 3
            : (breakpoint == AppBreakpoint.medium ? 2 : 1);
        const spacing = AppDimens.spaceLg;
        final cardWidth =
            (constraints.maxWidth - spacing * (columns - 1)) / columns;
        return SingleChildScrollView(
          physics: const BouncingScrollPhysics(),
          child: Wrap(
            spacing: spacing,
            runSpacing: spacing,
            children: [
              for (final group in groups)
                SizedBox(
                  width: cardWidth,
                  child: _GroupCard(
                    group: group,
                    onInvite: () => _showGroupInfo(group),
                  ),
                ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildHeader(bool compact) {
    final stats = _overview == null
        ? 'Tus grupos reales desde /me/overview'
        : '${_overview!.groupsCount} grupos · ${_overview!.adminGroupsCount} como admin (real)';
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const _FolderStamp(
                label: 'Carpetas de proyecto',
                icon: Icons.folder_special_rounded,
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                'MIS GRUPOS',
                style: TextStyle(
                  fontSize: compact ? 22 : 26,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                  letterSpacing: -0.5,
                ),
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                stats,
                style: const TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  color: AppColors.mutedStrong,
                ),
              ),
            ],
          ),
        ),
        if (!compact) ...[
          const SizedBox(width: AppDimens.spaceMd),
          NeobrutalistButton(
            label: _isCreating ? 'Creando...' : 'Nuevo grupo',
            icon: Icons.add_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: _isCreating ? null : _openCreateSheet,
          ),
        ],
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Credencial de grupo (solo campos reales del backend)
// ---------------------------------------------------------------------------

class _GroupCard extends StatelessWidget {
  const _GroupCard({required this.group, required this.onInvite});

  final GroupCard group;
  final VoidCallback onInvite;

  Color _colorFor(String id) {
    if (id.isEmpty) return AppColors.accentYellow;
    final h = id.hashCode;
    const options = [
      AppColors.accentYellow,
      AppColors.subjectBlue,
      AppColors.subjectMint,
    ];
    return options[h.abs() % options.length];
  }

  @override
  Widget build(BuildContext context) {
    final color = _colorFor(group.groupId);
    final description = (group.description == null ||
            group.description!.trim().isEmpty)
        ? 'Sin descripción'
        : group.description!.trim();
    final members = group.memberCount;

    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.card,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(height: 8, color: color),
          Padding(
            padding: const EdgeInsets.fromLTRB(
              AppDimens.spaceLg,
              AppDimens.spaceMd,
              AppDimens.spaceLg,
              AppDimens.spaceMd,
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Container(
                  width: 52,
                  height: 52,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: color,
                    border: Border.all(
                      color: AppColors.border,
                      width: AppDimens.borderWidth,
                    ),
                    borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                    boxShadow: AppShadows.badge,
                  ),
                  child: const Icon(
                    Icons.groups_rounded,
                    size: 26,
                    color: AppColors.text,
                  ),
                ),
                const SizedBox(width: AppDimens.spaceMd),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        group.name,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 14.5,
                          height: 1.15,
                          color: AppColors.text,
                        ),
                      ),
                      const SizedBox(height: AppDimens.spaceSm),
                      _CodeChip(label: description),
                      const SizedBox(height: 4),
                      Text('Rol: ${group.role}',
                          style: const TextStyle(
                              fontSize: 11,
                              fontWeight: FontWeight.w800,
                              color: AppColors.mutedStrong)),
                    ],
                  ),
                ),
              ],
            ),
          ),
          const Divider(
            color: AppColors.border,
            thickness: AppDimens.borderWidth,
            height: AppDimens.borderWidth,
          ),
          Padding(
            padding: const EdgeInsets.symmetric(
              horizontal: AppDimens.spaceLg,
              vertical: AppDimens.spaceMd,
            ),
            child: Row(
              children: [
                _AvatarStack(initials: const [], extra: members),
                const SizedBox(width: AppDimens.spaceMd),
                Expanded(
                  child: Text(
                    '$members ${members == 1 ? 'integrante' : 'integrantes'} (real)',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 11.5,
                      fontWeight: FontWeight.w800,
                      color: AppColors.mutedStrong,
                    ),
                  ),
                ),
              ],
            ),
          ),
          const Divider(
            color: AppColors.border,
            thickness: AppDimens.borderWidth,
            height: AppDimens.borderWidth,
          ),
          Padding(
            padding: const EdgeInsets.all(AppDimens.spaceMd),
            child: Row(
              children: [
                Expanded(
                  child: NeobrutalistButton(
                    label: 'Abrir',
                    icon: Icons.folder_open_rounded,
                    variant: NeobrutalistButtonVariant.primary,
                    expand: true,
                    onPressed: () => Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) => GroupDetailScreen(
                          group: <String, dynamic>{
                            'id': group.groupId,
                            'name': group.name,
                            'subject': description,
                            'members': members,
                            'role': group.role,
                          },
                        ),
                      ),
                    ),
                  ),
                ),
                const SizedBox(width: AppDimens.spaceSm),
                NeobrutalistButton(
                  label: 'Info',
                  icon: Icons.info_outline_rounded,
                  variant: NeobrutalistButtonVariant.secondary,
                  onPressed: onInvite,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _CodeChip extends StatelessWidget {
  const _CodeChip({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
      decoration: BoxDecoration(
        color: AppColors.bg,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        label.toUpperCase(),
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(
          fontFamily: 'monospace',
          fontSize: 10,
          fontWeight: FontWeight.w900,
          letterSpacing: 0.4,
          color: AppColors.text,
        ),
      ),
    );
  }
}

class _AvatarStack extends StatelessWidget {
  const _AvatarStack({required this.initials, required this.extra});

  final List<String> initials;
  final int extra;

  static const double _size = 30;
  static const double _gap = AppDimens.spaceXs;

  @override
  Widget build(BuildContext context) {
    final hasExtra = extra > 0;
    if (initials.isEmpty && !hasExtra) return const SizedBox.shrink();

    return Wrap(
      spacing: _gap,
      runSpacing: _gap,
      children: [
        for (final initial in initials) _AvatarBox(label: initial),
        if (hasExtra) _AvatarBox(label: '+$extra', inverted: true),
      ],
    );
  }
}

class _AvatarBox extends StatelessWidget {
  const _AvatarBox({required this.label, this.inverted = false});

  final String label;
  final bool inverted;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: _AvatarStack._size,
      height: _AvatarStack._size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: inverted ? AppColors.border : AppColors.accentYellow,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w900,
          letterSpacing: 0.2,
          color: inverted ? AppColors.surface : AppColors.text,
        ),
      ),
    );
  }
}

class _FolderStamp extends StatelessWidget {
  const _FolderStamp({required this.label, required this.icon});

  final String label;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: -0.02,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 4),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radiusChip),
          boxShadow: AppShadows.badge,
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 12, color: AppColors.text),
            const SizedBox(width: AppDimens.spaceXs),
            Flexible(
              child: Text(
                label.toUpperCase(),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 9.5,
                  fontWeight: FontWeight.w900,
                  letterSpacing: 0.8,
                  color: AppColors.text,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Modal de crear grupo (contrato real: name + description?)
// ---------------------------------------------------------------------------

class _NewGroupDraft {
  const _NewGroupDraft({required this.name, required this.description});

  final String name;
  final String description;
}

class _NewGroupSheet extends StatefulWidget {
  const _NewGroupSheet();

  @override
  State<_NewGroupSheet> createState() => _NewGroupSheetState();
}

class _NewGroupSheetState extends State<_NewGroupSheet> {
  final TextEditingController _nameController = TextEditingController();
  final TextEditingController _descController = TextEditingController();

  @override
  void dispose() {
    _nameController.dispose();
    _descController.dispose();
    super.dispose();
  }

  void _submit() {
    final name = _nameController.text.trim();
    if (name.isEmpty) return;
    final desc = _descController.text.trim();
    Navigator.of(context).pop(
      _NewGroupDraft(name: name, description: desc),
    );
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      top: false,
      child: Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: Container(
          decoration: const BoxDecoration(
            color: AppColors.surface,
            border: Border(
              top: BorderSide(
                color: AppColors.border,
                width: AppDimens.borderWidthThick,
              ),
            ),
          ),
          padding: const EdgeInsets.fromLTRB(
            AppDimens.spaceXl,
            AppDimens.spaceMd,
            AppDimens.spaceXl,
            AppDimens.spaceXl,
          ),
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Center(
                  child: Container(
                    width: 44,
                    height: 5,
                    decoration: BoxDecoration(
                      color: AppColors.border,
                      borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                    ),
                  ),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                Row(
                  children: [
                    const Expanded(
                      child: _FolderStamp(
                        label: 'Nueva carpeta de proyecto',
                        icon: Icons.create_new_folder_rounded,
                      ),
                    ),
                    NeobrutalistIconButton(
                      icon: Icons.close_rounded,
                      tooltip: 'Cerrar',
                      size: 32,
                      onPressed: () => Navigator.of(context).pop(),
                    ),
                  ],
                ),
                const SizedBox(height: AppDimens.spaceMd),
                const Text(
                  'CREAR GRUPO (REAL)',
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 16,
                    letterSpacing: 0.4,
                    color: AppColors.text,
                  ),
                ),
                const SizedBox(height: 4),
                const Text(
                  'POST /groups {name, description?} con tu sesión.',
                  style: TextStyle(
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                      color: AppColors.mutedStrong),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                const AppFieldLabel('NOMBRE DEL GRUPO'),
                const SizedBox(height: AppDimens.spaceSm),
                TextField(
                  controller: _nameController,
                  textInputAction: TextInputAction.next,
                  style: const TextStyle(
                    fontWeight: FontWeight.w800,
                    fontSize: 13.5,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration('Ej. Grupo Cálculo'),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                const AppFieldLabel('DESCRIPCIÓN (OPCIONAL)'),
                const SizedBox(height: AppDimens.spaceSm),
                TextField(
                  controller: _descController,
                  textInputAction: TextInputAction.done,
                  style: const TextStyle(
                    fontWeight: FontWeight.w800,
                    fontSize: 13,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration('Ej. Estudio para INF-360'),
                ),
                const SizedBox(height: AppDimens.spaceXl),
                NeobrutalistButton(
                  label: 'Crear grupo',
                  icon: Icons.add_rounded,
                  variant: NeobrutalistButtonVariant.accent,
                  expand: true,
                  onPressed: _submit,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
