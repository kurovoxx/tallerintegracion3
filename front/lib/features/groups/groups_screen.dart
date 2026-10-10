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
import 'package:flutter/services.dart';

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

String _initialsFor(String display) {
  final t = display.trim();
  if (t.isEmpty) return '?';
  // "Agustín Vega" -> "AV". Si es email, usa las 2 primeras letras.
  // Nunca muestra UUID: el llamador ya resolvió displayLabel.
  final parts = t.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
  if (parts.length >= 2) {
    return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
  }
  final clean = parts.first.replaceAll(RegExp(r'[^A-Za-zÁÉÍÓÚÑáéíóúñ0-9]'), '');
  if (clean.length >= 2) return clean.substring(0, 2).toUpperCase();
  if (clean.isNotEmpty) return clean[0].toUpperCase();
  return t.substring(0, t.length >= 2 ? 2 : 1).toUpperCase();
}

class _GroupsScreenState extends State<GroupsScreen> {
  late final SocialService _service;
  bool _isLoading = true;
  bool _isCreating = false;
  String? _error;
  Overview? _overview;
  // Iniciales reales por grupo (display_name/email). Best-effort: si falla,
  // la card muestra solo el conteo "N integrantes" sin avatar falso.
  Map<String, List<String>> _initialsByGroup = const {};

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
      // Iniciales reales (best-effort, no bloquea la lista).
      _loadInitials(ov.groups);
    } on SocialApiException catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudieron cargar tus grupos.';
        _isLoading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudieron cargar tus grupos.';
        _isLoading = false;
      });
    }
  }

  Future<void> _loadInitials(List<GroupCard> groups) async {
    final out = <String, List<String>>{};
    for (final g in groups) {
      try {
        final members = await _service.listMembers(g.groupId);
        final initials = <String>[];
        for (final m in members.take(4)) {
          // displayLabel nunca es UUID desnudo (display_name/email o corto).
          initials.add(_initialsFor(m.displayLabel));
        }
        out[g.groupId] = initials;
      } catch (_) {
        // Sin miembros legibles: la card muestra solo el conteo.
      }
    }
    if (!mounted) return;
    setState(() => _initialsByGroup = out);
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
      // POST /groups real; el id lo usa la recarga, no se muestra.
      await _service.createGroup(
        name: draft.name,
        description: draft.description.isEmpty ? null : draft.description,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Grupo creado.'),
          backgroundColor: AppColors.border,
        ),
      );
      await _load();
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo crear el grupo.'),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _isCreating = false);
    }
  }

  Future<void> _showGroupInfo(GroupCard group) async {
    final left = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: _GroupInfoDialog(group: group, service: _service),
    );
    if (left == true && mounted) await _load();
  }

  Future<void> _showInvite(GroupCard group) async {
    await showNeobrutalistDialog<void>(
      context: context,
      dialog: _InviteDialog(group: group, service: _service),
    );
    if (mounted) await _load();
  }

  bool _isJoining = false;

  Future<void> _openJoinDialog() async {
    final idCtrl = TextEditingController();
    final codeCtrl = TextEditingController();
    final ok = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: NeobrutalistDialog(
        title: 'UNIRSE A GRUPO',
        cancelLabel: 'Cancelar',
        confirmLabel: 'Unirse',
        closeOnConfirm: false,
        onConfirm: () => Navigator.of(context).pop(true),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text(
              'Pide al administrador el identificador del grupo y el código de invitación.',
              style: TextStyle(fontSize: 12),
            ),
            const SizedBox(height: 12),
            const AppFieldLabel('IDENTIFICADOR DEL GRUPO'),
            const SizedBox(height: 4),
            TextField(
              controller: idCtrl,
              decoration: appInputDecoration('Identificador del grupo'),
            ),
            const SizedBox(height: 8),
            const AppFieldLabel('CÓDIGO DE INVITACIÓN'),
            const SizedBox(height: 4),
            TextField(
              controller: codeCtrl,
              decoration: appInputDecoration('Código de invitación'),
            ),
          ],
        ),
      ),
    );
    final groupId = idCtrl.text.trim();
    final code = codeCtrl.text.trim();
    // Sin dispose manual: el diálogo aún anima su salida y sus TextField
    // pueden reconstruirse un frame más (dispose rompería el pump).
    if (ok != true || !mounted) return;
    if (groupId.isEmpty || code.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Completa el identificador y el código.'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    setState(() => _isJoining = true);
    try {
      // POST /groups/:id/join real. Refresca lista + sidebar vía notifier.
      await _service.joinGroup(groupId: groupId, inviteToken: code);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Te uniste al grupo.'),
          backgroundColor: AppColors.border,
        ),
      );
      await _load();
    } on SocialApiException catch (e) {
      if (!mounted) return;
      final s = e.statusCode;
      final msg = s == 404
          ? 'No se encontró el grupo o el código es inválido.'
          : s == 403
          ? 'No tienes permiso para unirte a este grupo.'
          : s == 401
          ? 'Tu sesión venció. Vuelve a iniciar sesión.'
          : 'No se pudo unir al grupo. Revisa los datos.';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(msg), backgroundColor: AppColors.error),
      );
    } finally {
      if (mounted) setState(() => _isJoining = false);
    }
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
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.card,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.error_outline_rounded,
                size: 36,
                color: AppColors.text,
              ),
              const SizedBox(height: 12),
              const Text(
                'NO SE PUDO CARGAR TUS GRUPOS',
                style: TextStyle(
                  fontWeight: FontWeight.w900,
                  fontSize: 14,
                  color: AppColors.text,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                _error!,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 12,
                  color: AppColors.mutedStrong,
                ),
              ),
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
            const Icon(
              Icons.groups_rounded,
              size: 40,
              color: AppColors.mutedStrong,
            ),
            const SizedBox(height: 12),
            const Text(
              'NO TIENES GRUPOS TODAVÍA',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 14,
                color: AppColors.text,
              ),
            ),
            const SizedBox(height: 6),
            const Text(
              'Crea tu primer grupo y aparecerá aquí',
              style: TextStyle(
                fontWeight: FontWeight.w600,
                fontSize: 12,
                color: AppColors.mutedStrong,
              ),
            ),
            const SizedBox(height: 14),
            NeobrutalistButton(
              label: 'Nuevo grupo',
              icon: Icons.add_rounded,
              variant: NeobrutalistButtonVariant.accent,
              onPressed: _openCreateSheet,
            ),
            const SizedBox(height: 8),
            NeobrutalistButton(
              label: _isJoining ? 'Uniéndose...' : 'Unirse a grupo',
              icon: Icons.login_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: _isJoining ? null : _openJoinDialog,
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
                    initials: _initialsByGroup[group.groupId] ?? const [],
                    onInvite: () => _showInvite(group),
                    onInfo: () => _showGroupInfo(group),
                  ),
                ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildHeader(bool compact) {
    final o = _overview;
    final stats = o == null
        ? 'Tus grupos'
        : '${o.groupsCount} ${o.groupsCount == 1 ? 'grupo' : 'grupos'} · '
              '${o.adminGroupsCount} como admin';
    final titleColumn = Column(
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
    );
    // Compacto: botones debajo del título a ancho completo (sin overflow).
    if (compact) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          titleColumn,
          const SizedBox(height: AppDimens.spaceMd),
          NeobrutalistButton(
            label: _isJoining ? 'Uniéndose...' : 'Unirse a grupo',
            icon: Icons.login_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            expand: true,
            onPressed: _isJoining ? null : _openJoinDialog,
          ),
        ],
      );
    }
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(child: titleColumn),
        const SizedBox(width: AppDimens.spaceMd),
        Column(
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            NeobrutalistButton(
              label: _isCreating ? 'Creando...' : 'Nuevo grupo',
              icon: Icons.add_rounded,
              variant: NeobrutalistButtonVariant.accent,
              onPressed: _isCreating ? null : _openCreateSheet,
            ),
            const SizedBox(height: 8),
            NeobrutalistButton(
              label: _isJoining ? 'Uniéndose...' : 'Unirse a grupo',
              icon: Icons.login_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: _isJoining ? null : _openJoinDialog,
            ),
          ],
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Diálogo INFO: nombre, descripción, rol, integrantes y Abandonar.
// Sin código de invitación (vive en el diálogo INVITAR, solo admin).
// ---------------------------------------------------------------------------

class _GroupInfoDialog extends StatefulWidget {
  const _GroupInfoDialog({required this.group, required this.service});

  final GroupCard group;
  final SocialService service;

  @override
  State<_GroupInfoDialog> createState() => _GroupInfoDialogState();
}

class _GroupInfoDialogState extends State<_GroupInfoDialog> {
  bool _loading = true;
  String? _error;
  GroupDetail? _detail;
  bool _leaving = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (!mounted) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final d = await widget.service.getGroup(widget.group.groupId);
      if (!mounted) return;
      setState(() {
        _detail = d;
        _loading = false;
      });
    } on SocialApiException catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudo cargar la información del grupo.';
        _loading = false;
      });
    }
  }

  Future<void> _leave() async {
    if (_leaving) return;
    final ok = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: NeobrutalistDialog(
        title: 'ABANDONAR GRUPO',
        cancelLabel: 'Cancelar',
        confirmLabel: 'Abandonar',
        confirmVariant: NeobrutalistButtonVariant.danger,
        closeOnConfirm: false,
        onConfirm: () => Navigator.of(context).pop(true),
        content: Text(
          '¿Abandonar "${widget.group.name}"? Si eres el único admin con más '
          'miembros, la administración pasará automáticamente al integrante más antiguo.',
        ),
      ),
    );
    if (ok != true || !mounted) return;
    setState(() => _leaving = true);
    try {
      // POST /groups/:id/leave real (204). Sucesión/admin según backend.
      await widget.service.leaveGroup(widget.group.groupId);
      if (!mounted) return;
      Navigator.of(context).pop(true);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Abandonaste el grupo.'),
          backgroundColor: AppColors.border,
        ),
      );
    } on SocialApiException catch (_) {
      if (!mounted) return;
      const msg = 'No se pudo abandonar el grupo. Inténtalo nuevamente.';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(msg), backgroundColor: AppColors.error),
      );
    } finally {
      if (mounted) setState(() => _leaving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final group = widget.group;
    final members = group.memberCount;
    return NeobrutalistDialog(
      title: group.name,
      cancelLabel: 'Cerrar',
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (group.description != null &&
              group.description!.trim().isNotEmpty)
            Text(group.description!.trim()),
          const SizedBox(height: 8),
          Text('Tu rol: ${group.role}'),
          Text('$members ${members == 1 ? 'integrante' : 'integrantes'}'),
          const SizedBox(height: 12),
          if (_loading)
            const Center(child: CircularProgressIndicator(strokeWidth: 2))
          else if (_error != null)
            Text(_error!)
          else if (_detail?.role == 'admin') ...[
            const Text(
              'Gestiona el código de invitación desde el botón Invitar.',
              style: TextStyle(fontSize: 12),
            ),
          ],
          const SizedBox(height: 12),
          const Divider(),
          TextButton.icon(
            onPressed: _leaving ? null : _leave,
            icon: const Icon(Icons.exit_to_app_rounded, size: 16),
            label: Text(_leaving ? 'Abandonando...' : 'Abandonar grupo'),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Diálogo INVITAR (solo admin): ver código, regenerar y compartir.
// El token solo lo entrega GET /groups/:id a admins.
// ---------------------------------------------------------------------------

class _InviteDialog extends StatefulWidget {
  const _InviteDialog({required this.group, required this.service});

  final GroupCard group;
  final SocialService service;

  @override
  State<_InviteDialog> createState() => _InviteDialogState();
}

class _InviteDialogState extends State<_InviteDialog> {
  bool _loading = true;
  String? _error;
  String? _inviteToken;
  bool _regenerating = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (!mounted) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final d = await widget.service.getGroup(widget.group.groupId);
      if (!mounted) return;
      setState(() {
        _inviteToken = d.inviteToken;
        _loading = false;
      });
    } on SocialApiException catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudo cargar la invitación.';
        _loading = false;
      });
    }
  }

  Future<void> _regenerate() async {
    if (_regenerating) return;
    setState(() => _regenerating = true);
    try {
      final token = await widget.service.regenerateInvite(widget.group.groupId);
      if (!mounted) return;
      setState(() => _inviteToken = token);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Invitación actualizada.'),
          backgroundColor: AppColors.border,
        ),
      );
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo actualizar la invitación.'),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _regenerating = false);
    }
  }

  Future<void> _copy(String label, String value) async {
    final v = value.trim();
    if (v.isEmpty) return;
    await Clipboard.setData(ClipboardData(text: v));
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('$label copiado.'),
        backgroundColor: AppColors.border,
      ),
    );
  }

  Future<void> _copyAll() {
    final id = widget.group.groupId;
    final token = _inviteToken?.trim() ?? '';
    return _copy('Datos de invitación', '$id $token');
  }

  Widget _copyableField({required String label, required String value}) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          label,
          style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 2),
        Row(
          children: [
            Expanded(child: SelectableText(value)),
            IconButton(
              icon: const Icon(Icons.copy_rounded, size: 18),
              tooltip: 'Copiar $label',
              onPressed: () => _copy(label, value),
            ),
          ],
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    return NeobrutalistDialog(
      title: 'Invitar a ${widget.group.name}',
      cancelLabel: 'Cerrar',
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            'Comparte estos datos con la persona que quieras invitar.',
            style: TextStyle(fontSize: 12),
          ),
          const SizedBox(height: 12),
          _copyableField(
            label: 'Identificador del grupo',
            value: widget.group.groupId,
          ),
          const SizedBox(height: 8),
          if (_loading)
            const Center(child: CircularProgressIndicator(strokeWidth: 2))
          else if (_error != null)
            Text(_error!)
          else ...[
            if (_inviteToken != null && _inviteToken!.isNotEmpty)
              _copyableField(
                label: 'Código de invitación',
                value: _inviteToken!,
              ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: _regenerating ? null : _regenerate,
              child: Text(
                _regenerating ? 'Actualizando...' : 'Generar nuevo código',
              ),
            ),
            const Text(
              'El código anterior dejará de funcionar.',
              style: TextStyle(fontSize: 11),
            ),
            const SizedBox(height: 4),
            OutlinedButton.icon(
              onPressed:
                  (_inviteToken == null || _inviteToken!.trim().isEmpty)
                  ? null
                  : _copyAll,
              icon: const Icon(Icons.copy_all_rounded, size: 16),
              label: const Text('Copiar datos de invitación'),
            ),
          ],
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Credencial de grupo (solo campos reales del backend)
// ---------------------------------------------------------------------------

class _GroupCard extends StatelessWidget {
  const _GroupCard({
    required this.group,
    required this.onInvite,
    required this.onInfo,
    this.initials = const [],
  });

  final GroupCard group;
  final VoidCallback onInvite;
  final VoidCallback onInfo;
  // Iniciales reales desde display_name/email (máx 3 visibles).
  final List<String> initials;

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
    final description =
        (group.description == null || group.description!.trim().isEmpty)
        ? 'Sin descripción'
        : group.description!.trim();
    final members = group.memberCount;

    final card = Container(
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
                      Text(
                        'Rol: ${group.role}',
                        style: const TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w800,
                          color: AppColors.mutedStrong,
                        ),
                      ),
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
                // Estilo vistas: iniciales reales apiladas. Sin miembros
                // legibles no se muestra "+N" como botón: solo el conteo.
                if (initials.isNotEmpty) ...[
                  _AvatarStack(
                    initials: initials.take(3).toList(),
                    extra: members - initials.take(3).length,
                  ),
                  const SizedBox(width: AppDimens.spaceMd),
                ] else ...[
                  const Icon(
                    Icons.groups_rounded,
                    size: 18,
                    color: AppColors.mutedStrong,
                  ),
                  const SizedBox(width: AppDimens.spaceMd),
                ],
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
                // Estilo vistas: ABRIR + INVITAR (solo admin). Miembro: INFO.
                // El admin accede a INFO con el icono superpuesto (sin
                // sobrecargar la fila en pantallas angostas).
                if (group.role == 'admin') ...[
                  const SizedBox(width: AppDimens.spaceSm),
                  NeobrutalistButton(
                    label: 'Invitar',
                    icon: Icons.person_add_alt_1_rounded,
                    variant: NeobrutalistButtonVariant.secondary,
                    onPressed: onInvite,
                  ),
                ] else ...[
                  const SizedBox(width: AppDimens.spaceSm),
                  NeobrutalistButton(
                    label: 'Info',
                    icon: Icons.info_outline_rounded,
                    variant: NeobrutalistButtonVariant.secondary,
                    onPressed: onInfo,
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
    if (group.role != 'admin') return card;
    return Stack(
      children: [
        card,
        Positioned(
          top: 12,
          right: 8,
          child: NeobrutalistIconButton(
            icon: Icons.info_outline_rounded,
            tooltip: 'Información del grupo',
            size: 28,
            onPressed: onInfo,
          ),
        ),
      ],
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
    Navigator.of(context).pop(_NewGroupDraft(name: name, description: desc));
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
                  'CREAR GRUPO',
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 16,
                    letterSpacing: 0.4,
                    color: AppColors.text,
                  ),
                ),
                const SizedBox(height: 4),
                const Text(
                  'Crea un espacio para organizar a tu equipo.',
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: AppColors.mutedStrong,
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
                  label: 'Crear',
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
