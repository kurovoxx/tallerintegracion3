// Ruta: front/lib/features/groups/groups_screen.dart
//
// 'Credenciales / Carpetas de Proyecto': tarjetas de grupo con caja de color
// de asignatura, chip de ramo, integrantes apilados y acciones balanceadas.
// Consume tokens de core/theme/app_theme.dart y core/widgets/neobrutalism.dart.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'group_detail_screen.dart';

class GroupsScreen extends StatefulWidget {
  const GroupsScreen({super.key});

  @override
  State<GroupsScreen> createState() => _GroupsScreenState();
}

class _GroupsScreenState extends State<GroupsScreen> {
  final List<Map<String, dynamic>> _groups = [
    {
      'id': 'g1',
      'name': 'Cálculo II - Grupo Alpha',
      'subject': 'MAT1002',
      'members': 5,
      'color': AppColors.accentYellow,
      'icon': Icons.calculate_rounded,
      'avatars': ['AB', 'CD', 'EF'],
    },
    {
      'id': 'g2',
      'name': 'Bases de Datos - Proyecto',
      'subject': 'INF220',
      'members': 4,
      'color': AppColors.subjectBlue,
      'icon': Icons.storage_rounded,
      'avatars': ['GH', 'IJ'],
    },
    {
      'id': 'g3',
      'name': 'Taller Integración III',
      'subject': 'INF-360',
      'members': 6,
      'color': AppColors.subjectMint,
      'icon': Icons.handyman_rounded,
      'avatars': ['KL', 'MN', 'OP', 'QR'],
    },
  ];

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
    setState(() {
      _groups.add({
        'id': 'g${_groups.length + 1}',
        'name': draft.name,
        'subject': draft.subject,
        'members': 1,
        'color': AppColors.accentYellow,
        'icon': Icons.groups_rounded,
        'avatars': ['YO'],
      });
    });
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Grupo creado'),
        backgroundColor: AppColors.border,
      ),
    );
  }

  void _invite(Map<String, dynamic> group) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('Invitación enviada a ${group['name']}'),
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
              Expanded(
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final columns = breakpoint == AppBreakpoint.expanded
                        ? 3
                        : (breakpoint == AppBreakpoint.medium ? 2 : 1);
                    const spacing = AppDimens.spaceLg;
                    final cardWidth =
                        (constraints.maxWidth - spacing * (columns - 1)) /
                        columns;
                    return SingleChildScrollView(
                      physics: const BouncingScrollPhysics(),
                      child: Wrap(
                        spacing: spacing,
                        runSpacing: spacing,
                        children: [
                          for (final group in _groups)
                            SizedBox(
                              width: cardWidth,
                              child: _GroupCard(
                                group: group,
                                onInvite: () => _invite(group),
                              ),
                            ),
                        ],
                      ),
                    );
                  },
                ),
              ),
            ],
          ),
        ),
      ),
      floatingActionButton: compact
          ? NeobrutalistFab(
              icon: Icons.group_add_rounded,
              tooltip: 'Nuevo grupo',
              onPressed: _openCreateSheet,
            )
          : null,
    );
  }

  Widget _buildHeader(bool compact) {
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
              const Text(
                'Colabora y comparte apuntes con tu comunidad',
                style: TextStyle(
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
            label: 'Nuevo grupo',
            icon: Icons.add_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: _openCreateSheet,
          ),
        ],
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Credencial de grupo
// ---------------------------------------------------------------------------

class _GroupCard extends StatelessWidget {
  const _GroupCard({required this.group, required this.onInvite});

  final Map<String, dynamic> group;
  final VoidCallback onInvite;

  @override
  Widget build(BuildContext context) {
    final color = group['color'] as Color;
    final subject = group['subject'] as String;
    final members = group['members'] as int;
    final avatars = (group['avatars'] as List).cast<String>();
    final visibleAvatars = avatars.take(3).toList();
    final extraMembers = members - visibleAvatars.length;

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
                  child: Icon(
                    group['icon'] as IconData,
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
                        group['name'] as String,
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
                      _CodeChip(label: subject),
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
                _AvatarStack(initials: visibleAvatars, extra: extraMembers),
                const SizedBox(width: AppDimens.spaceMd),
                Expanded(
                  child: Text(
                    '$members ${members == 1 ? 'integrante' : 'integrantes'}',
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
                        builder: (_) => GroupDetailScreen(group: group),
                      ),
                    ),
                  ),
                ),
                const SizedBox(width: AppDimens.spaceSm),
                NeobrutalistButton(
                  label: 'Invitar',
                  icon: Icons.person_add_alt_1_rounded,
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
// Modal de crear grupo
// ---------------------------------------------------------------------------

class _NewGroupDraft {
  const _NewGroupDraft({required this.name, required this.subject});

  final String name;
  final String subject;
}

class _NewGroupSheet extends StatefulWidget {
  const _NewGroupSheet();

  @override
  State<_NewGroupSheet> createState() => _NewGroupSheetState();
}

class _NewGroupSheetState extends State<_NewGroupSheet> {
  final TextEditingController _nameController = TextEditingController();
  final TextEditingController _subjectController = TextEditingController();

  @override
  void dispose() {
    _nameController.dispose();
    _subjectController.dispose();
    super.dispose();
  }

  void _submit() {
    final name = _nameController.text.trim();
    if (name.isEmpty) return;
    final subject = _subjectController.text.trim();
    Navigator.of(context).pop(
      _NewGroupDraft(
        name: name,
        subject: subject.isEmpty ? 'General' : subject,
      ),
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
                  'CREAR / UNIRSE A GRUPO',
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 16,
                    letterSpacing: 0.4,
                    color: AppColors.text,
                  ),
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
                const AppFieldLabel('ASIGNATURA ASOCIADA'),
                const SizedBox(height: AppDimens.spaceSm),
                TextField(
                  controller: _subjectController,
                  textInputAction: TextInputAction.done,
                  style: const TextStyle(
                    fontFamily: 'monospace',
                    fontWeight: FontWeight.w800,
                    fontSize: 13,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration('Ej. MAT1002'),
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
