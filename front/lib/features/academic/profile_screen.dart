
import 'package:flutter/material.dart';
import '../../core/common_widgets.dart';
import '../../core/models/profile_models.dart';
import '../../core/services/api_config.dart';
import '../../core/services/profile_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../auth/login_screen.dart';
import '../auth/google_drive_service.dart';

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

  const ProfileScreen({super.key, this.userData, ProfileService? service})
      : _serviceOverride = service;

  final ProfileService? _serviceOverride;

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  bool _isLoading = true;
  bool _isSaving = false;
  bool _hasError = false;
  String? _errorMessage;

  UserProfile? _profile;
  late final ProfileService _service;

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? ProfileService();
    _loadProfile();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  // Contrato real: GET /profile/me (Auth).
  // Ver back/auth/cmd/server/main.go:107 y profile_handler.go:46.
  Future<void> _loadProfile() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
      _errorMessage = null;
    });

    try {
      final p = await _service.getProfile();
      if (!mounted) return;
      setState(() {
        _profile = p;
        _isLoading = false;
      });
    } on ProfileApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _hasError = true;
        _errorMessage = e.toString();
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _hasError = true;
        _errorMessage = 'No se pudo cargar tu perfil: $e';
      });
    }
  }

  // Contrato real: PATCH /profile/me (parcial).
  Future<void> _handleSave(Map<String, dynamic> payload) async {
    if (_profile == null) return;
    setState(() => _isSaving = true);
    try {
      final patch = <String, dynamic>{
        'display_name': payload['display_name'],
        'description': payload['description'],
        'institution': payload['institution'],
        'visibility': payload['visibility'],
      };
      final updated = await _service.patchProfile(patch);
      if (!mounted) return;
      setState(() => _profile = updated);

      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Cambios guardados en backend (real)',
              style: TextStyle(fontWeight: FontWeight.w700)),
          backgroundColor: AppColors.border,
        ),
      );
    } on ProfileApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('No se pudo guardar: $e'),
          backgroundColor: AppColors.error,
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
    if (_isLoading || _profile == null) return;

    final payload = await showModalBottomSheet<Map<String, dynamic>>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => _EditProfileSheet(
        initialDisplayName: _profile!.displayName,
        initialDescription: _profile!.description ?? '',
        initialInstitution: _profile!.institution ?? '',
        initialVisibility: _profile!.visibility,
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
        _SquareIconButton(
          icon: Icons.cloud_done_rounded,
          tooltip: 'Conectar Drive (prueba real)',
          onPressed: () async {
            final messenger = ScaffoldMessenger.of(context);
            try {
              final code = await GoogleDriveService().getServerAuthCode();
              if (code == null) {
                messenger.showSnackBar(const SnackBar(content: Text('Cancelado')));
                return;
              }
              final token = SessionManager.token;
              if (token == null) {
                messenger.showSnackBar(const SnackBar(content: Text('No hay sesión')));
                return;
              }
              final driveBackendBaseUrl = authApiBaseUrl;
              debugPrint('[FRONT DEBUG] Drive callback: endpoint auth=$driveBackendBaseUrl/auth/google-drive/connect');
              final ok = await GoogleDriveService().connectDrive(
                backendBaseUrl: driveBackendBaseUrl,
                appAccessToken: token,
                oauthCode: code,
              );
              messenger.showSnackBar(SnackBar(content: Text(ok ? 'Drive conectado (200)' : 'Falló')));
            } catch (e) {
              messenger.showSnackBar(SnackBar(content: Text('Error: $e')));
            }
          },
        ),
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
    final p = _profile;
    if (p == null) return const SizedBox.shrink();
    final institution = (p.institution == null || p.institution!.trim().isEmpty)
        ? 'No informada (real)'
        : p.institution!.trim();
    final phone = (p.phone == null || p.phone!.trim().isEmpty)
        ? 'No informado (real)'
        : p.phone!.trim();
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
          const Text('FICHA PERSONAL (REAL: GET /profile/me)',
              style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.muted, letterSpacing: 0.6)),
          const SizedBox(height: 12),
          _SummaryRow(icon: Icons.apartment_rounded, label: 'INSTITUCIÓN', value: institution),
          const SizedBox(height: 10),
          _SummaryRow(icon: Icons.phone_rounded, label: 'TELÉFONO', value: phone),
          const SizedBox(height: 10),
          const _SummaryRow(
              icon: Icons.email_rounded,
              label: 'CORREO INSTITUCIONAL',
              value: 'No entregado por /profile/me'),
          const SizedBox(height: 14),
          const Divider(color: AppColors.border, thickness: 2, height: 1),
          const SizedBox(height: 14),
          const Text('RESUMEN DE ACTIVIDAD',
              style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.muted, letterSpacing: 0.5)),
          const SizedBox(height: 10),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: AppColors.bg,
              border: Border.all(color: AppColors.border, width: 1.5),
              borderRadius: BorderRadius.circular(8),
            ),
            child: const Text(
              'Notas, grupos, tareas, promedios, asistencia y seguidores no disponibles en GET /profile/me. '
              'El backend solo entrega display_name, photo_url, phone, institution, description y visibility.',
              style: TextStyle(fontSize: 11.5, fontWeight: FontWeight.w600, color: AppColors.mutedStrong, height: 1.35),
            ),
          ),
          const SizedBox(height: 16),
          NeobrutalistButton(
            label: 'Cerrar sesión',
            icon: Icons.logout_rounded,
            variant: NeobrutalistButtonVariant.danger,
            expand: true,
            onPressed: _handleLogout,
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
    final p = _profile;
    if (p == null) return const SizedBox.shrink();
    final name =
        p.displayName.trim().isEmpty ? 'Sin nombre (real)' : p.displayName.trim();
    final institution = (p.institution == null || p.institution!.trim().isEmpty)
        ? 'Institución no informada (real)'
        : p.institution!.trim();
    final description = (p.description == null || p.description!.trim().isEmpty)
        ? 'Sin descripción (real)'
        : p.description!.trim();
    final visibility = p.visibility;
    final photoUrl = p.photoUrl ?? '';

    final avatar = _Avatar(name: name, photoUrl: photoUrl, size: isDesktop ? 100 : 80);
    final crossAlign = isDesktop ? CrossAxisAlignment.start : CrossAxisAlignment.center;
    final textAlign = isDesktop ? TextAlign.start : TextAlign.center;

    Widget nameRow(String label) => isDesktop
        ? Wrap(
            crossAxisAlignment: WrapCrossAlignment.center,
            spacing: 10,
            runSpacing: 6,
            children: [
              Text(name, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w900, color: AppColors.text)),
              _RoleBadge(label: label),
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
              _RoleBadge(label: label),
            ],
          );

    final nameAndRole = nameRow('PERFIL REAL');

    final content = Column(
      crossAxisAlignment: crossAlign,
      children: [
        nameAndRole,
        const SizedBox(height: 8),
        Text(institution, textAlign: textAlign, style: TextStyle(fontSize: isDesktop ? 14 : 12.5, fontWeight: FontWeight.w700, color: AppColors.text)),
        const SizedBox(height: 2),
        const Text('Correo y rol no entregados por /profile/me',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 11.5, fontWeight: FontWeight.w600, color: AppColors.muted)),
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

    Widget divider() => Container(width: 1.5, color: AppColors.border.withValues(alpha: 0.4));

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
    // El contrato GET /profile/me no entrega promedios, asistencia ni
    // seguidores. No se muestran valores demo como reales.
    return const [
      _StatBlock(value: '—', label: 'PROMEDIO (no disponible)', color: AppColors.muted),
      _StatBlock(value: '—', label: 'ASISTENCIA (no disponible)', color: AppColors.muted),
      _StatBlock(value: '—', label: 'SEGUIDORES (no disponible)', color: AppColors.muted),
      _StatBlock(value: '—', label: 'SIGUIENDO (no disponible)', color: AppColors.muted),
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
            const Text('NO SE PUDO CARGAR TU PERFIL (REAL)',
                textAlign: TextAlign.center,
                style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
            if (_errorMessage != null) ...[
              const SizedBox(height: 8),
              Text(_errorMessage!,
                  textAlign: TextAlign.center,
                  style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 12, color: AppColors.mutedStrong)),
            ],
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
          ? Image.network(photoUrl, fit: BoxFit.cover, width: size, height: size, errorBuilder: (_, _, _) => fallback)
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


