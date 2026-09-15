
import 'package:flutter/material.dart';
import '../../core/common_widgets.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../auth/login_screen.dart';

const String kSubjectApproved = 'approved';
const String kSubjectInProgress = 'in_progress';
const String kSubjectFailed = 'failed';
const String kSubjectPending = 'pending';

String formatGrade(num raw) {
  final value = raw >= 10 ? raw / 10 : raw.toDouble();
  return value.toStringAsFixed(1);
}

String formatRole(String rawRole) {
  switch (rawRole) {
    case 'teacher':
      return 'Docente';
    case 'student':
    default:
      return 'Estudiante';
  }
}

double? computeAttendance({required int? totalClasses, required int unjustifiedAbsences}) {
  if (totalClasses == null || totalClasses <= 0) return null;
  final ratio = 1 - (unjustifiedAbsences / totalClasses);
  return (ratio * 100).clamp(0, 100).toDouble();
}

class ProfileScreen extends StatefulWidget {
  final Map<String, dynamic>? userData;

  const ProfileScreen({super.key, this.userData});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  bool _isLoading = true;
  bool _isSaving = false;
  bool _hasError = false;

  late Map<String, dynamic> _profile;

  @override
  void initState() {
    super.initState();
    _loadProfile();
  }

  Future<void> _loadProfile() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
    });

    try {
      await Future.delayed(const Duration(milliseconds: 500));

      _profile = {
        'email': widget.userData?['email'] ?? 'miguel.fernandez@uniorg.cl',
        'role': widget.userData?['role'] ?? 'student',
        'display_name': widget.userData?['display_name'] ?? 'Miguel Fernández',
        'photo_url': widget.userData?['photo_url'] ?? '',
        'phone': widget.userData?['phone'] ?? '',
        'institution': widget.userData?['institution'] ?? 'Universidad Católica de Temuco',
        'description': widget.userData?['description'] ??
            'Enfocado en infraestructura, redes y desarrollo full-stack. '
                'Construyendo esta misma app como proyecto de capstone.',
        'visibility': widget.userData?['visibility'] ?? 'public',
        'followers_count': widget.userData?['followers_count'] ?? 128,
        'following_count': widget.userData?['following_count'] ?? 54,
        'average_raw': widget.userData?['average_raw'] ?? 58,
        'total_classes': widget.userData?['total_classes'] ?? 120,
        'unjustified_absences': widget.userData?['unjustified_absences'] ?? 10,
      };

      if (!mounted) return;
      setState(() => _isLoading = false);
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _hasError = true;
      });
    }
  }

  Future<void> _handleSave(Map<String, dynamic> payload) async {
    setState(() => _isSaving = true);
    try {
      await Future.delayed(const Duration(milliseconds: 600));

      if (!mounted) return;
      setState(() {
        _profile = {
          ..._profile,
          'display_name': payload['display_name'],
          'description': payload['description'],
          'institution': payload['institution'],
          'visibility': payload['visibility'],
        };
      });

      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Cambios guardados correctamente', style: TextStyle(fontWeight: FontWeight.w700)),
          backgroundColor: AppColors.border,
        ),
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  void _handleShare() {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Enlace de perfil copiado al portapapeles')),
    );
  }

  void _handleLogout() {
    SessionManager.clear();
    Navigator.of(context).pushAndRemoveUntil(
      MaterialPageRoute(builder: (_) => const LoginScreen()),
      (route) => false,
    );
  }

  Future<void> _openEditSheet() async {
    if (_isLoading) return;

    final payload = await showModalBottomSheet<Map<String, dynamic>>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => _EditProfileSheet(
        initialDisplayName: _profile['display_name'] as String,
        initialDescription: _profile['description'] as String,
        initialInstitution: _profile['institution'] as String,
        initialVisibility: _profile['visibility'] as String,
      ),
    );

    if (payload != null) {
      await _handleSave(payload);
    }
  }

  @override
  Widget build(BuildContext context) {
    final screenWidth = MediaQuery.of(context).size.width;
    final isDesktop = screenWidth > AppDimens.breakpointDesktop;

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: _hasError
            ? _buildErrorState()
            : SingleChildScrollView(
                padding: EdgeInsets.symmetric(horizontal: isDesktop ? 48 : 16, vertical: 20),
                child: Center(
                  child: ConstrainedBox(
                    constraints: BoxConstraints(maxWidth: isDesktop ? 1320 : 640),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _buildHeader(isDesktop),
                        const SizedBox(height: 18),
                        _isLoading ? _buildLoadingCard() : _buildIdentityCard(isDesktop),
                        const SizedBox(height: 16),
                        if (!_isLoading) _buildPersonalSummary(isDesktop),
                      ],
                    ),
                  ),
                ),
              ),
      ),
    );
  }

  // ---------------------------------------------------------------------
  // ENCABEZADO
  // ---------------------------------------------------------------------
  Widget _buildHeader(bool isDesktop) {
    final titleBlock = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('MI PERFIL', style: TextStyle(color: AppColors.text, fontWeight: FontWeight.w900, fontSize: isDesktop ? 28 : 24, letterSpacing: -0.6)),
        const SizedBox(height: 2),
        Text(
          'Tu identidad académica dentro de Sigma Academy',
          style: const TextStyle(color: AppColors.muted, fontWeight: FontWeight.w700, fontSize: 12.5),
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
      ],
    );

    final iconActions = Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        _SquareIconButton(icon: Icons.share_rounded, tooltip: 'Compartir perfil', onPressed: _handleShare),
        const SizedBox(width: 8),
        _SquareIconButton(icon: Icons.logout_rounded, tooltip: 'Cerrar sesión', onPressed: _handleLogout),
      ],
    );

    final saveButtonText = _isSaving ? 'GUARDANDO...' : 'GUARDAR CAMBIOS';

    if (isDesktop) {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(child: titleBlock),
          iconActions,
          const SizedBox(width: 12),
          SizedBox(width: 210, child: SubmitButton(text: saveButtonText, onPressed: _isLoading || _isSaving ? () {} : _openEditSheet)),
        ],
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(child: titleBlock),
            const SizedBox(width: 8),
            iconActions,
          ],
        ),
        const SizedBox(height: 12),
        SubmitButton(text: saveButtonText, onPressed: _isLoading || _isSaving ? () {} : _openEditSheet),
      ],
    );
  }

  // ---------------------------------------------------------------------
  // ACCESOS DIRECTOS — banner neo-brutalista (pmn.html)
  // 5 tarjetas con borde 4px, sombra dura Offset(4,4) y chevron
  // ---------------------------------------------------------------------
  Widget _buildPersonalSummary(bool isDesktop) {
    final institution = _profile['institution'] as String;
    final email = _profile['email'] as String;
    final roleLabel = formatRole(_profile['role'] as String);
    return Container(
      padding: EdgeInsets.all(isDesktop ? 20 : 16),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('FICHA PERSONAL', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.muted, letterSpacing: 0.6)),
          const SizedBox(height: 12),
          _SummaryRow(icon: Icons.school_rounded, label: 'CARRERA', value: roleLabel),
          const SizedBox(height: 10),
          _SummaryRow(icon: Icons.apartment_rounded, label: 'INSTITUCIÓN', value: institution),
          const SizedBox(height: 10),
          _SummaryRow(icon: Icons.email_rounded, label: 'CORREO INSTITUCIONAL', value: email),
          const SizedBox(height: 14),
          const Divider(color: AppColors.border, thickness: 2, height: 1),
          const SizedBox(height: 14),
          const Text('RESUMEN DE ACTIVIDAD', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.muted, letterSpacing: 0.5)),
          const SizedBox(height: 10),
          Row(
            children: [
              Expanded(child: _MiniStat(icon: Icons.description_rounded, label: 'Notas', value: '12')),
              const SizedBox(width: 10),
              Expanded(child: _MiniStat(icon: Icons.groups_rounded, label: 'Grupos', value: '3')),
              const SizedBox(width: 10),
              Expanded(child: _MiniStat(icon: Icons.task_alt_rounded, label: 'Tareas', value: '8/12')),
            ],
          ),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: SubmitButton(text: 'CERRAR SESIÓN', onPressed: _handleLogout),
          ),
        ],
      ),
    );
  }

  // ---------------------------------------------------------------------
  // TARJETA DE IDENTIDAD
  // ---------------------------------------------------------------------
  Widget _buildIdentityCard(bool isDesktop) {
    return Container(
      padding: EdgeInsets.all(isDesktop ? 28 : 18),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)],
      ),
      child: isDesktop
          ? IntrinsicHeight(
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Expanded(flex: 3, child: _buildIdentityInfo(isDesktop)),
                  const SizedBox(width: 28),
                  const VerticalDivider(color: AppColors.border, thickness: AppDimens.borderWidth, width: 1),
                  const SizedBox(width: 28),
                  Expanded(flex: 2, child: _buildStatsColumn()),
                ],
              ),
            )
          : Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _buildIdentityInfo(isDesktop),
                const SizedBox(height: 16),
                const Divider(color: AppColors.border, thickness: AppDimens.borderWidth, height: 1),
                const SizedBox(height: 14),
                _buildStatsGridMobile(),
              ],
            ),
    );
  }

  Widget _buildIdentityInfo(bool isDesktop) {
    final name = _profile['display_name'] as String;
    final role = _profile['role'] as String;
    final institution = _profile['institution'] as String;
    final email = _profile['email'] as String;
    final description = _profile['description'] as String;
    final visibility = _profile['visibility'] as String;
    final photoUrl = (_profile['photo_url'] as String?) ?? '';

    final avatar = _Avatar(name: name, photoUrl: photoUrl, size: isDesktop ? 100 : 80);
    final crossAlign = isDesktop ? CrossAxisAlignment.start : CrossAxisAlignment.center;
    final textAlign = isDesktop ? TextAlign.start : TextAlign.center;

    final nameAndRole = isDesktop
        ? Wrap(
            crossAxisAlignment: WrapCrossAlignment.center,
            spacing: 10,
            runSpacing: 6,
            children: [
              Text(name, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w900, color: AppColors.text)),
              _RoleBadge(label: formatRole(role).toUpperCase()),
            ],
          )
        : Column(
            children: [
              Text(
                name,
                textAlign: TextAlign.center,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w900, color: AppColors.text),
              ),
              const SizedBox(height: 6),
              _RoleBadge(label: formatRole(role).toUpperCase()),
            ],
          );

    final content = Column(
      crossAxisAlignment: crossAlign,
      children: [
        nameAndRole,
        const SizedBox(height: 8),
        Text(institution, textAlign: textAlign, style: TextStyle(fontSize: isDesktop ? 14 : 12.5, fontWeight: FontWeight.w700, color: AppColors.text)),
        const SizedBox(height: 2),
        Text(email, textAlign: textAlign, style: TextStyle(fontSize: isDesktop ? 13 : 11.5, fontWeight: FontWeight.w600, color: AppColors.muted)),
        const SizedBox(height: 12),
        Text(description, textAlign: textAlign, style: TextStyle(fontSize: isDesktop ? 14.5 : 12.5, fontWeight: FontWeight.w600, color: AppColors.text, height: 1.4)),
        const SizedBox(height: 12),
        _VisibilityChip(visibility: visibility),
      ],
    );

    if (isDesktop) {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [avatar, const SizedBox(width: 22), Expanded(child: content)],
      );
    }

    return Column(children: [avatar, const SizedBox(height: 12), content]);
  }

  Widget _buildStatsColumn() {
    final stats = _statBlocks();
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        for (int i = 0; i < stats.length; i++) ...[
          if (i != 0) const SizedBox(height: 18),
          stats[i],
        ],
      ],
    );
  }

  /// FIX: reemplaza el cálculo manual con MediaQuery (que se rompía en
  /// ventanas de ancho intermedio) por una grilla 2x2 basada en `Expanded`,
  /// que siempre se ajusta al espacio real disponible sin importar el
  /// tamaño de la ventana.
  Widget _buildStatsGridMobile() {
    final stats = _statBlocks();

    Widget divider() => Container(width: 1.5, color: AppColors.border.withOpacity(0.4));

    return Column(
      children: [
        IntrinsicHeight(
          child: Row(
            children: [
              Expanded(child: stats[0]),
              const SizedBox(width: 8),
              divider(),
              const SizedBox(width: 8),
              Expanded(child: stats[1]),
            ],
          ),
        ),
        const SizedBox(height: 16),
        IntrinsicHeight(
          child: Row(
            children: [
              Expanded(child: stats[2]),
              const SizedBox(width: 8),
              divider(),
              const SizedBox(width: 8),
              Expanded(child: stats[3]),
            ],
          ),
        ),
      ],
    );
  }

  List<Widget> _statBlocks() {
    final averageRaw = (_profile['average_raw'] as num?) ?? 0;
    final totalClasses = _profile['total_classes'] as int?;
    final unjustified = (_profile['unjustified_absences'] as int?) ?? 0;
    final followers = (_profile['followers_count'] as num?) ?? 0;
    final following = (_profile['following_count'] as num?) ?? 0;

    final attendancePct = computeAttendance(totalClasses: totalClasses, unjustifiedAbsences: unjustified);
    final attendanceLabel = attendancePct == null ? 'No config.' : '${attendancePct.round()}%';

    return [
      _StatBlock(value: formatGrade(averageRaw), label: 'PROMEDIO', color: AppColors.text),
      _StatBlock(
        value: attendanceLabel,
        label: 'ASISTENCIA',
        color: attendancePct == null ? AppColors.muted : AppColors.accentBlue,
        valueFontSize: attendancePct == null ? 13 : 24,
      ),
      _StatBlock(value: '$followers', label: 'SEGUIDORES', color: AppColors.text),
      _StatBlock(value: '$following', label: 'SIGUIENDO', color: AppColors.text),
    ];
  }

  // ---------------------------------------------------------------------
  // ESTADOS DE CARGA Y ERROR
  // ---------------------------------------------------------------------
  Widget _buildLoadingCard() {
    return Container(
      height: 220,
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)],
      ),
      child: const Center(child: CircularProgressIndicator(color: AppColors.text)),
    );
  }

  Widget _buildErrorState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded, size: 48, color: AppColors.text),
            const SizedBox(height: 12),
            const Text('NO SE PUDO CARGAR TU PERFIL', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
            const SizedBox(height: 16),
            SizedBox(width: 200, child: SubmitButton(text: 'REINTENTAR', onPressed: _loadProfile)),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------
// MODAL DE EDICIÓN (RF-05)
// ---------------------------------------------------------------------

class _EditProfileSheet extends StatefulWidget {
  final String initialDisplayName;
  final String initialDescription;
  final String initialInstitution;
  final String initialVisibility;

  const _EditProfileSheet({
    required this.initialDisplayName,
    required this.initialDescription,
    required this.initialInstitution,
    required this.initialVisibility,
  });

  @override
  State<_EditProfileSheet> createState() => _EditProfileSheetState();
}

class _EditProfileSheetState extends State<_EditProfileSheet> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _nameController;
  late final TextEditingController _descriptionController;
  late final TextEditingController _institutionController;
  late String _visibility;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: widget.initialDisplayName);
    _descriptionController = TextEditingController(text: widget.initialDescription);
    _institutionController = TextEditingController(text: widget.initialInstitution);
    _visibility = widget.initialVisibility;
  }

  @override
  void dispose() {
    _nameController.dispose();
    _descriptionController.dispose();
    _institutionController.dispose();
    super.dispose();
  }

  void _submit() {
    if (!_formKey.currentState!.validate()) return;

    Navigator.of(context).pop({
      'display_name': _nameController.text.trim(),
      'description': _descriptionController.text.trim(),
      'institution': _institutionController.text.trim(),
      'visibility': _visibility,
    });
  }

  @override
  Widget build(BuildContext context) {
    final bottomInset = MediaQuery.of(context).viewInsets.bottom;

    return Padding(
      padding: EdgeInsets.only(bottom: bottomInset),
      child: Container(
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: const Border(top: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
          borderRadius: BorderRadius.vertical(top: Radius.circular(AppDimens.radius * 2)),
        ),
        padding: const EdgeInsets.fromLTRB(20, 14, 20, 20),
        child: SingleChildScrollView(
          child: Form(
            key: _formKey,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Center(
                  child: Container(width: 44, height: 5, decoration: BoxDecoration(color: AppColors.muted, borderRadius: BorderRadius.circular(4))),
                ),
                const SizedBox(height: 16),
                const Text('EDITAR PERFIL', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 18, color: AppColors.text)),
                const SizedBox(height: 18),
                const AppFieldLabel('NOMBRE COMPLETO'),
                const SizedBox(height: 6),
                TextFormField(
                  controller: _nameController,
                  textInputAction: TextInputAction.next,
                  style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14, color: AppColors.text),
                  decoration: appInputDecoration('Ej. Miguel Fernández'),
                  validator: (v) {
                    if (v == null || v.trim().isEmpty) return 'El nombre no puede estar vacío';
                    return null;
                  },
                ),
                const SizedBox(height: 16),
                const AppFieldLabel('INSTITUCIÓN'),
                const SizedBox(height: 6),
                TextFormField(
                  controller: _institutionController,
                  textInputAction: TextInputAction.next,
                  style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14, color: AppColors.text),
                  decoration: appInputDecoration('Ej. Universidad Católica de Temuco'),
                ),
                const SizedBox(height: 16),
                const AppFieldLabel('DESCRIPCIÓN'),
                const SizedBox(height: 6),
                TextFormField(
                  controller: _descriptionController,
                  maxLines: 3,
                  maxLength: 500,
                  style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13, color: AppColors.text),
                  decoration: appInputDecoration('Cuéntanos sobre ti...'),
                ),
                const SizedBox(height: 8),
                const AppFieldLabel('VISIBILIDAD DEL PERFIL'),
                const SizedBox(height: 8),
                _VisibilitySelector(value: _visibility, onChanged: (v) => setState(() => _visibility = v)),
                const SizedBox(height: 24),
                SubmitButton(text: 'GUARDAR CAMBIOS', onPressed: _submit),
                const SizedBox(height: 6),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _VisibilitySelector extends StatelessWidget {
  final String value;
  final ValueChanged<String> onChanged;

  const _VisibilitySelector({required this.value, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(child: _VisibilityOption(label: 'PÚBLICO', icon: Icons.visibility_rounded, active: value == 'public', onTap: () => onChanged('public'))),
        const SizedBox(width: 10),
        Expanded(child: _VisibilityOption(label: 'PRIVADO', icon: Icons.visibility_off_rounded, active: value == 'private', onTap: () => onChanged('private'))),
      ],
    );
  }
}

class _VisibilityOption extends StatelessWidget {
  final String label;
  final IconData icon;
  final bool active;
  final VoidCallback onTap;

  const _VisibilityOption({required this.label, required this.icon, required this.active, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 12),
        decoration: BoxDecoration(
          color: active ? AppColors.accentBlue : AppColors.bg,
          border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: active ? const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)] : null,
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, size: 16, color: active ? Colors.white : AppColors.text),
            const SizedBox(width: 6),
            Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w900, color: active ? Colors.white : AppColors.text)),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------
// WIDGETS AUXILIARES PRIVADOS
// ---------------------------------------------------------------------

// ignore: unused_element - mantenido para referencia visual, navegación ahora en MainShell
class _QuickAccessCard extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  final VoidCallback onTap;

  const _QuickAccessCard({required this.icon, required this.title, required this.subtitle, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border, width: 4),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)],
        ),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: AppColors.accentYellow,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(AppDimens.radius),
              ),
              child: Icon(icon, size: 20, color: AppColors.text),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13.5, color: AppColors.text)),
                  const SizedBox(height: 2),
                  Text(subtitle, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 11.5, color: AppColors.muted)),
                ],
              ),
            ),
            const Icon(Icons.chevron_right_rounded, size: 20, color: AppColors.text),
          ],
        ),
      ),
    );
  }
}

class _Avatar extends StatelessWidget {
  final String name;
  final String photoUrl;
  final double size;
  const _Avatar({required this.name, this.photoUrl = '', this.size = 84});

  @override
  Widget build(BuildContext context) {
    final initials = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).take(2).map((p) => p[0].toUpperCase()).join();

    final fallback = Text(initials.isEmpty ? 'U' : initials, style: TextStyle(fontSize: size * 0.32, fontWeight: FontWeight.w900, color: AppColors.text));

    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      clipBehavior: Clip.antiAlias,
      decoration: BoxDecoration(
        color: AppColors.accentYellow,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: photoUrl.isNotEmpty
          ? Image.network(photoUrl, fit: BoxFit.cover, width: size, height: size, errorBuilder: (_, __, ___) => fallback)
          : fallback,
    );
  }
}

class _RoleBadge extends StatelessWidget {
  final String label;
  const _RoleBadge({required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(color: AppColors.text, borderRadius: BorderRadius.circular(4)),
      child: Text(label, style: const TextStyle(color: Colors.white, fontSize: 10, fontWeight: FontWeight.w900, letterSpacing: 0.5)),
    );
  }
}

class _VisibilityChip extends StatelessWidget {
  final String visibility;
  const _VisibilityChip({required this.visibility});

  @override
  Widget build(BuildContext context) {
    final isPublic = visibility == 'public';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(color: AppColors.accentBlue, borderRadius: BorderRadius.circular(AppDimens.radius)),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(isPublic ? Icons.visibility_rounded : Icons.visibility_off_rounded, size: 14, color: Colors.white),
          const SizedBox(width: 6),
          Text(isPublic ? 'PERFIL PÚBLICO' : 'PERFIL PRIVADO', style: const TextStyle(color: Colors.white, fontSize: 11, fontWeight: FontWeight.w800, letterSpacing: 0.3)),
        ],
      ),
    );
  }
}

class _StatBlock extends StatelessWidget {
  final String value;
  final String label;
  final Color color;
  final double valueFontSize;
  const _StatBlock({required this.value, required this.label, required this.color, this.valueFontSize = 22});

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        FittedBox(
          fit: BoxFit.scaleDown,
          child: Text(value, textAlign: TextAlign.center, style: TextStyle(fontSize: valueFontSize, fontWeight: FontWeight.w900, color: color)),
        ),
        const SizedBox(height: 2),
        Text(
          label,
          textAlign: TextAlign.center,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w800, color: AppColors.muted, letterSpacing: 0.3),
        ),
      ],
    );
  }
}

class _SquareIconButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onPressed;
  final String? tooltip;

  const _SquareIconButton({required this.icon, required this.onPressed, this.tooltip});

  @override
  Widget build(BuildContext context) {
    final button = InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onPressed,
      child: Container(
        width: 42,
        height: 42,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)],
        ),
        child: Icon(icon, size: 18, color: AppColors.text),
      ),
    );
    return tooltip != null ? Tooltip(message: tooltip!, child: button) : button;
  }
}

class _SummaryRow extends StatelessWidget {
  final IconData icon;
  final String label;
  final String value;
  const _SummaryRow({required this.icon, required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 30,
          height: 30,
          decoration: BoxDecoration(
            color: AppColors.bg,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Icon(icon, size: 16, color: AppColors.text),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(label, style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w800, color: AppColors.muted, letterSpacing: 0.5)),
              const SizedBox(height: 2),
              Text(value, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: AppColors.text)),
            ],
          ),
        ),
      ],
    );
  }
}

class _MiniStat extends StatelessWidget {
  final IconData icon;
  final String label;
  final String value;
  const _MiniStat({required this.icon, required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 8),
      decoration: BoxDecoration(
        color: AppColors.bg,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        children: [
          Icon(icon, size: 18, color: AppColors.text),
          const SizedBox(height: 6),
          Text(value, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
          const SizedBox(height: 2),
          Text(label, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 10, color: AppColors.muted, letterSpacing: 0.4)),
        ],
      ),
    );
  }
}


