import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/common_widgets.dart';
import '../../core/models/profile_models.dart';
import '../../core/services/api_config.dart';
import '../../core/services/profile_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../auth/google_drive_service.dart';
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

enum _DriveStatus { idle, connecting, connected, disconnecting, error }

class ProfileScreen extends StatefulWidget {
  final Map<String, dynamic>? userData;

  const ProfileScreen({super.key, this.userData, ProfileService? service})
      : _serviceOverride = service;

  final ProfileService? _serviceOverride;

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  static const String _googleDrivePrefKey = 'profile_google_drive_connected';

  bool _isLoading = true;
  bool _isSaving = false;
  bool _hasError = false;
  bool _isEditing = false;
  String? _errorMessage;

  UserProfile? _profile;
  late final ProfileService _service;

  final _formKey = GlobalKey<FormState>();
  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();
  final _institutionController = TextEditingController();
  final _descriptionController = TextEditingController();
  // Correo de Google declarado por el usuario para conectar Drive.
  // Viaja como login_hint en OAuth y como expected_email al backend,
  // que lo compara con el email real de userinfo (anti cuenta equivocada).
  final _googleEmailController = TextEditingController();
  String _draftVisibility = 'public';
  _DriveStatus _driveStatus = _DriveStatus.idle;

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? ProfileService();
    _restoreDriveStatus();
    _loadProfile();
  }

  Future<void> _restoreDriveStatus() async {
    final preferences = await SharedPreferences.getInstance();
    final wasConnected = preferences.getBool(_googleDrivePrefKey) ?? false;
    if (!mounted) return;
    setState(() {
      _driveStatus = wasConnected ? _DriveStatus.connected : _DriveStatus.idle;
    });
  }

  Future<void> _persistDriveStatus(bool connected) async {
    final preferences = await SharedPreferences.getInstance();
    await preferences.setBool(_googleDrivePrefKey, connected);
  }

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    _institutionController.dispose();
    _descriptionController.dispose();
    _googleEmailController.dispose();
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  // Contrato real: GET /profile/me (Auth).
  // Ver back/auth/cmd/server/main.go:107 y profile_handler.go:46.
  Future<void> _loadProfile() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
      _isEditing = false;
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

  // ---------------------------------------------------------------------
  // MÁQUINA DE ESTADOS: modo vista <-> modo edición in-place (_isEditing)
  // ---------------------------------------------------------------------

  bool get _canEdit => !_isLoading && !_isSaving && _profile != null;

  void _startEditing() {
    final p = _profile;
    if (p == null || !_canEdit) return;

    _nameController.text = p.displayName;
    _phoneController.text = p.phone ?? '';
    _institutionController.text = p.institution ?? '';
    _descriptionController.text = p.description ?? '';

    setState(() {
      _draftVisibility = p.visibility == 'private' ? 'private' : 'public';
      _isEditing = true;
    });
  }

  void _cancelEditing() {
    if (_isSaving) return;
    setState(() => _isEditing = false);
  }

  // Contrato real: PATCH /profile/me (parcial).
  // Campos omitidos no cambian; vacíos borran a NULL (salvo display_name).
  Future<void> _handleSave() async {
    if (_profile == null || _isSaving) return;
    if (!(_formKey.currentState?.validate() ?? false)) return;

    setState(() => _isSaving = true);
    try {
      final patch = <String, dynamic>{
        'display_name': _nameController.text.trim(),
        'phone': _phoneController.text.trim(),
        'institution': _institutionController.text.trim(),
        'description': _descriptionController.text.trim(),
        'visibility': _draftVisibility,
      };
      final updated = await _service.patchProfile(patch);
      if (!mounted) return;
      setState(() {
        _profile = updated;
        _isEditing = false;
      });

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
    } catch (e) {
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

  Future<void> _handleConnectDrive() async {
    if (_driveStatus == _DriveStatus.connecting ||
        _driveStatus == _DriveStatus.disconnecting) {
      return;
    }
    final messenger = ScaffoldMessenger.of(context);
    final declaredEmail = _googleEmailController.text.trim();
    if (declaredEmail.isEmpty) {
      messenger.showSnackBar(const SnackBar(content: Text('Escribe tu correo de Google para conectar Drive')));
      return;
    }
    setState(() => _driveStatus = _DriveStatus.connecting);

    try {
      // En web no hay localhost que capture el code: diálogo pegar-código.
      final String? code = kIsWeb
          ? await _askWebAuthCode(messenger, declaredEmail)
          : await GoogleDriveService().getServerAuthCode(loginHint: declaredEmail);
      if (code == null) {
        if (!mounted) return;
        setState(() => _driveStatus = _DriveStatus.idle);
        messenger.showSnackBar(const SnackBar(content: Text('Conexión con Drive cancelada')));
        return;
      }
      final token = SessionManager.token;
      if (token == null) {
        if (!mounted) return;
        setState(() => _driveStatus = _DriveStatus.idle);
        messenger.showSnackBar(const SnackBar(content: Text('No hay sesión')));
        return;
      }
      debugPrint('[FRONT DEBUG] Drive callback: endpoint auth=$authApiBaseUrl/auth/google-drive/connect');
      final ok = await GoogleDriveService().connectDrive(
        backendBaseUrl: authApiBaseUrl,
        appAccessToken: token,
        oauthCode: code,
        expectedEmail: declaredEmail,
      );
      await _persistDriveStatus(ok);
      if (!mounted) return;
      setState(() => _driveStatus = ok ? _DriveStatus.connected : _DriveStatus.error);
      messenger.showSnackBar(SnackBar(content: Text(ok ? 'Drive conectado (200)' : 'No se pudo conectar Drive')));
    } catch (e) {
      if (!mounted) return;
      setState(() => _driveStatus = _DriveStatus.error);
      messenger.showSnackBar(SnackBar(content: Text('Error al conectar Drive: $e')));
    }
  }

  /// Flujo web: abre Google en pestaña externa y pide pegar el ?code=
  /// de la URL de retorno (nada escucha localhost en el navegador).
  /// Retorna null si el usuario cancela.
  Future<String?> _askWebAuthCode(ScaffoldMessengerState messenger, String declaredEmail) async {
    final codeController = TextEditingController();
    try {
      await GoogleDriveService().openWebAuthUrl(loginHint: declaredEmail);
    } catch (_) {
      // Si ni siquiera abre el navegador, igual se ofrece el pegado manual.
    }
    if (!mounted) return null;
    final pasted = await showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: const Text('PEGA EL CÓDIGO DE GOOGLE'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text(
              'Autoriza en la pestaña de Google y copia el parámetro code= de la dirección a la que te redirige.',
              style: TextStyle(fontSize: 13),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: codeController,
              decoration: const InputDecoration(
                labelText: 'CÓDIGO (code=...)',
                hintText: '4/0A...',
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('CANCELAR'),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(codeController.text.trim()),
            child: const Text('CONECTAR'),
          ),
        ],
      ),
    );
    final code = (pasted ?? '').trim();
    return code.isEmpty ? null : code;
  }

  Future<void> _handleDisconnectDrive() async {
    if (_driveStatus == _DriveStatus.connecting ||
        _driveStatus == _DriveStatus.disconnecting) {
      return;
    }
    final messenger = ScaffoldMessenger.of(context);
    setState(() => _driveStatus = _DriveStatus.disconnecting);

    try {
      await GoogleDriveService().signOut();
    } catch (_) {}
    await _persistDriveStatus(false);
    if (!mounted) return;
    setState(() => _driveStatus = _DriveStatus.idle);
    messenger.showSnackBar(const SnackBar(content: Text('Google Drive desconectado')));
  }

  void _handleShare() {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Enlace de perfil copiado al portapapeles')),
    );
  }

  Future<void> _handleLogout() async {
    await SessionManager.clear();
    if (!mounted) return;
    Navigator.of(context).pushAndRemoveUntil(
      MaterialPageRoute(builder: (_) => const LoginScreen()),
      (route) => false,
    );
  }

  // ---------------------------------------------------------------------
  // BUILD
  // ---------------------------------------------------------------------
  @override
  Widget build(BuildContext context) {
    final isDesktop = context.isDesktop;

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: _hasError
            ? _buildErrorState()
            : SingleChildScrollView(
                physics: const BouncingScrollPhysics(),
                padding: EdgeInsets.symmetric(
                  horizontal: isDesktop ? AppDimens.spaceXl : AppDimens.spaceLg,
                  vertical: AppDimens.spaceXl,
                ),
                child: Center(
                  child: ConstrainedBox(
                    constraints: BoxConstraints(
                      maxWidth: isDesktop ? AppDimens.contentMaxWidth : 640,
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _buildHeader(isDesktop),
                        const SizedBox(height: AppDimens.spaceLg),
                        if (_isLoading)
                          _buildLoadingCard()
                        else if (_isEditing)
                          _buildEditMode(isDesktop)
                        else
                          ..._buildViewMode(isDesktop),
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
        Text(
          'MI PERFIL',
          style: TextStyle(
            color: AppColors.text,
            fontWeight: FontWeight.w900,
            fontSize: isDesktop ? 28 : 24,
            letterSpacing: -0.6,
          ),
        ),
        const SizedBox(height: 2),
        const Text(
          'Tu identidad académica dentro de Sigma Academy',
          style: TextStyle(color: AppColors.muted, fontWeight: FontWeight.w700, fontSize: 12.5),
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
      ],
    );

    if (_isEditing) {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(child: titleBlock),
          const SizedBox(width: AppDimens.spaceMd),
          const NeobrutalistBadge(
            label: 'Modo edición',
            tone: NeobrutalistTone.accent,
            icon: Icons.edit_rounded,
          ),
        ],
      );
    }

    final shareButton = NeobrutalistIconButton(
      icon: Icons.share_rounded,
      tooltip: 'Compartir perfil',
      onPressed: _handleShare,
    );

    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Expanded(child: titleBlock),
        const SizedBox(width: AppDimens.spaceSm),
        shareButton,
      ],
    );
  }

  // ---------------------------------------------------------------------
  // MODO VISTA: tarjetas neobrutalistas (avatar, datos, Google Drive)
  // ---------------------------------------------------------------------
  List<Widget> _buildViewMode(bool isDesktop) {
    if (_profile == null) return const [SizedBox.shrink()];
    return [
      _buildIdentityCard(isDesktop),
      const SizedBox(height: AppDimens.spaceLg),
      _buildPersonalDataCard(isDesktop),
      const SizedBox(height: AppDimens.spaceLg),
      _buildDriveCard(isDesktop),
      const SizedBox(height: AppDimens.spaceLg),
      _buildSessionActions(isDesktop),
    ];
  }

  Widget _buildSessionActions(bool isDesktop) {
    final logoutButton = NeobrutalistButton(
      label: 'CERRAR SESIÓN',
      icon: Icons.logout_rounded,
      variant: NeobrutalistButtonVariant.secondary,
      expand: !isDesktop,
      onPressed: _handleLogout,
    );

    if (!isDesktop) return logoutButton;
    return Align(
      alignment: Alignment.centerRight,
      child: SizedBox(width: 220, child: logoutButton),
    );
  }

  Widget _buildIdentityCard(bool isDesktop) {
    final p = _profile!;
    final name = p.displayName.trim().isEmpty ? 'Sin nombre' : p.displayName.trim();
    final isPublic = p.visibility == 'public';
    final description = (p.description == null || p.description!.trim().isEmpty)
        ? 'Sin descripción. Agrega una presentación desde Editar perfil.'
        : p.description!.trim();

    final identityText = Column(
      crossAxisAlignment: isDesktop ? CrossAxisAlignment.start : CrossAxisAlignment.center,
      children: [
        Text(
          name,
          textAlign: isDesktop ? TextAlign.start : TextAlign.center,
          style: TextStyle(
            fontSize: isDesktop ? 22 : 18,
            fontWeight: FontWeight.w900,
            color: AppColors.text,
            letterSpacing: -0.3,
          ),
        ),
        const SizedBox(height: AppDimens.spaceSm),
        NeobrutalistBadge(
          label: isPublic ? 'Perfil público' : 'Perfil privado',
          tone: isPublic ? NeobrutalistTone.info : NeobrutalistTone.pending,
          icon: isPublic ? Icons.visibility_rounded : Icons.visibility_off_rounded,
        ),
        const SizedBox(height: AppDimens.spaceMd),
        Text(
          description,
          textAlign: isDesktop ? TextAlign.start : TextAlign.center,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: AppColors.mutedStrong,
            height: 1.4,
          ),
        ),
      ],
    );

    final avatar = _Avatar(name: name, photoUrl: p.photoUrl ?? '', size: isDesktop ? 112 : 88);

    return _ProfileCard(
      title: 'Avatar e identidad',
      trailing: const NeobrutalistBadge(label: 'Datos reales', tone: NeobrutalistTone.neutral),
      child: isDesktop
          ? Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                avatar,
                const SizedBox(width: AppDimens.spaceXl),
                Expanded(child: identityText),
              ],
            )
          : Column(
              children: [
                avatar,
                const SizedBox(height: AppDimens.spaceLg),
                identityText,
              ],
            ),
    );
  }

  Widget _buildPersonalDataCard(bool isDesktop) {
    final p = _profile!;
    final institution = (p.institution == null || p.institution!.trim().isEmpty)
        ? 'No informada'
        : p.institution!.trim();
    final phone = (p.phone == null || p.phone!.trim().isEmpty)
        ? 'No informado'
        : p.phone!.trim();

    final editButton = NeobrutalistButton(
      label: 'EDITAR PERFIL',
      icon: Icons.edit_rounded,
      variant: NeobrutalistButtonVariant.accent,
      expand: !isDesktop,
      onPressed: _canEdit ? _startEditing : null,
    );

    return _ProfileCard(
      title: 'Datos personales',
      trailing: const _EndpointTag('GET /profile/me'),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _SummaryRow(icon: Icons.apartment_rounded, label: 'INSTITUCIÓN', value: institution),
          const SizedBox(height: AppDimens.spaceMd),
          _SummaryRow(icon: Icons.phone_rounded, label: 'TELÉFONO', value: phone),
          const SizedBox(height: AppDimens.spaceMd),
          const _SummaryRow(
            icon: Icons.email_rounded,
            label: 'CORREO INSTITUCIONAL',
            value: 'No entregado por /profile/me',
          ),
          const SizedBox(height: AppDimens.spaceLg),
          const Divider(color: AppColors.border, thickness: AppDimens.borderWidth, height: 1),
          const SizedBox(height: AppDimens.spaceMd),
          Container(
            padding: const EdgeInsets.all(AppDimens.spaceMd),
            decoration: BoxDecoration(
              color: AppColors.bg,
              border: Border.all(color: AppColors.border, width: 1.5),
              borderRadius: BorderRadius.circular(AppDimens.radius),
            ),
            child: const Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(Icons.info_outline_rounded, size: 16, color: AppColors.mutedStrong),
                SizedBox(width: AppDimens.spaceSm),
                Expanded(
                  child: Text(
                    'El backend solo entrega display_name, photo_url, phone, institution, description y visibility. '
                    'Notas, grupos, tareas, promedios, asistencia y seguidores no forman parte de GET /profile/me.',
                    style: TextStyle(
                      fontSize: 11.5,
                      fontWeight: FontWeight.w600,
                      color: AppColors.mutedStrong,
                      height: 1.35,
                    ),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: AppDimens.spaceLg),
          isDesktop
              ? Align(
                  alignment: Alignment.centerRight,
                  child: SizedBox(width: 220, child: editButton),
                )
              : editButton,
        ],
      ),
    );
  }

  Widget _buildDriveCard(bool isDesktop) {
    final connecting = _driveStatus == _DriveStatus.connecting;
    final connected = _driveStatus == _DriveStatus.connected;
    final disconnecting = _driveStatus == _DriveStatus.disconnecting;
    final busy = connecting || disconnecting;

    final connectButton = NeobrutalistButton(
      label: connecting ? 'CONECTANDO...' : 'CONECTAR DRIVE',
      icon: Icons.add_link_rounded,
      variant: NeobrutalistButtonVariant.info,
      expand: !isDesktop,
      onPressed: busy ? null : _handleConnectDrive,
    );
    final renewButton = NeobrutalistButton(
      label: disconnecting ? 'DESCONECTANDO...' : 'RENOVAR',
      icon: Icons.sync_rounded,
      variant: NeobrutalistButtonVariant.accent,
      expand: !isDesktop,
      onPressed: busy ? null : _handleConnectDrive,
    );
    final disconnectButton = NeobrutalistButton(
      label: 'DESCONECTAR',
      icon: Icons.link_off_rounded,
      variant: NeobrutalistButtonVariant.secondary,
      expand: !isDesktop,
      onPressed: busy ? null : _handleDisconnectDrive,
    );

    final Widget driveActions;
    if (!connected) {
      driveActions = isDesktop
          ? Align(
              alignment: Alignment.centerRight,
              child: SizedBox(width: 240, child: connectButton),
            )
          : connectButton;
    } else if (isDesktop) {
      driveActions = Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          SizedBox(width: 180, child: disconnectButton),
          const SizedBox(width: AppDimens.spaceMd),
          SizedBox(width: 180, child: renewButton),
        ],
      );
    } else {
      driveActions = Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          renewButton,
          const SizedBox(height: AppDimens.spaceSm),
          disconnectButton,
        ],
      );
    }

    return _ProfileCard(
      title: 'Google Drive',
      trailing: _buildDriveBadge(),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 44,
                height: 44,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: AppColors.accentYellow,
                  border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: AppShadows.badge,
                ),
                child: const Icon(Icons.cloud_rounded, size: 22, color: AppColors.text),
              ),
              const SizedBox(width: AppDimens.spaceMd),
              const Expanded(
                child: Text(
                  'Delega el respaldo de tus apuntes y archivos: la vinculación se autoriza por OAuth '
                  'y el backend guarda el acceso para sincronizar tu Drive.',
                  style: TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w600,
                    color: AppColors.mutedStrong,
                    height: 1.4,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceLg),
          if (!connected) ...[
            TextField(
              controller: _googleEmailController,
              keyboardType: TextInputType.emailAddress,
              enabled: !busy,
              decoration: const InputDecoration(
                labelText: 'TU CORREO DE GOOGLE',
                hintText: 'tucorreo@gmail.com',
                helperText: 'Se usa para abrir tu cuenta y verificar la conexión. Nunca se comparte.',
                helperMaxLines: 2,
              ),
            ),
            const SizedBox(height: AppDimens.spaceMd),
          ],
          driveActions,
        ],
      ),
    );
  }

  Widget _buildDriveBadge() {
    switch (_driveStatus) {
      case _DriveStatus.connecting:
        return const NeobrutalistBadge(
          label: 'Conectando',
          tone: NeobrutalistTone.accent,
          icon: Icons.sync_rounded,
        );
      case _DriveStatus.disconnecting:
        return const NeobrutalistBadge(
          label: 'Desconectando',
          tone: NeobrutalistTone.pending,
          icon: Icons.sync_disabled_rounded,
        );
      case _DriveStatus.connected:
        return const NeobrutalistBadge(
          label: 'Conectado',
          tone: NeobrutalistTone.success,
          icon: Icons.check_circle_rounded,
        );
      case _DriveStatus.error:
        return const NeobrutalistBadge(
          label: 'Error',
          tone: NeobrutalistTone.danger,
          icon: Icons.error_rounded,
        );
      case _DriveStatus.idle:
        return const NeobrutalistBadge(
          label: 'Sin conectar',
          tone: NeobrutalistTone.pending,
          icon: Icons.cloud_off_rounded,
        );
    }
  }

  // ---------------------------------------------------------------------
  // MODO EDICIÓN IN-PLACE: Form + barra GUARDAR CAMBIOS / CANCELAR
  // ---------------------------------------------------------------------
  Widget _buildEditMode(bool isDesktop) {
    final formCard = _ProfileCard(
      title: 'Editar perfil',
      trailing: const _EndpointTag('PATCH /profile/me'),
      child: Form(
        key: _formKey,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _ProfileTextField(
              label: 'NOMBRE COMPLETO',
              controller: _nameController,
              hint: 'Ej. Miguel Fernández',
              prefixIcon: Icons.person_rounded,
              textInputAction: TextInputAction.next,
              maxLength: 100,
              validator: (value) {
                final v = value?.trim() ?? '';
                if (v.isEmpty) return 'El nombre no puede estar vacío';
                if (v.length > 100) return 'Máximo 100 caracteres';
                return null;
              },
            ),
            const SizedBox(height: AppDimens.spaceLg),
            _ProfileTextField(
              label: 'INSTITUCIÓN',
              controller: _institutionController,
              hint: 'Ej. Universidad Católica de Temuco',
              prefixIcon: Icons.apartment_rounded,
              textInputAction: TextInputAction.next,
              maxLength: 200,
              validator: (value) {
                if ((value?.trim().length ?? 0) > 200) return 'Máximo 200 caracteres';
                return null;
              },
            ),
            const SizedBox(height: AppDimens.spaceLg),
            _ProfileTextField(
              label: 'TELÉFONO',
              controller: _phoneController,
              hint: 'Ej. +56 9 1234 5678',
              prefixIcon: Icons.phone_rounded,
              keyboardType: TextInputType.phone,
              textInputAction: TextInputAction.next,
              maxLength: 30,
              validator: (value) {
                if ((value?.trim().length ?? 0) > 30) return 'Máximo 30 caracteres';
                return null;
              },
            ),
            const SizedBox(height: AppDimens.spaceLg),
            _ProfileTextField(
              label: 'DESCRIPCIÓN',
              controller: _descriptionController,
              hint: 'Cuéntanos sobre ti...',
              maxLines: 3,
              maxLength: 500,
              textInputAction: TextInputAction.newline,
            ),
            const SizedBox(height: AppDimens.spaceLg),
            const AppFieldLabel('VISIBILIDAD DEL PERFIL'),
            const SizedBox(height: AppDimens.spaceSm),
            _VisibilitySelector(
              value: _draftVisibility,
              onChanged: _isSaving ? (_) {} : (v) => setState(() => _draftVisibility = v),
            ),
          ],
        ),
      ),
    );

    return CallbackShortcuts(
      bindings: <ShortcutActivator, VoidCallback>{
        const SingleActivator(LogicalKeyboardKey.keyS, control: true): () {
          if (!_isSaving) _handleSave();
        },
        const SingleActivator(LogicalKeyboardKey.escape): () {
          if (!_isSaving) _cancelEditing();
        },
      },
      child: Focus(
        autofocus: true,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            formCard,
            const SizedBox(height: AppDimens.spaceLg),
            _buildEditActionBar(isDesktop),
          ],
        ),
      ),
    );
  }

  Widget _buildEditActionBar(bool isDesktop) {
    final saveButton = NeobrutalistButton(
      label: _isSaving ? 'GUARDANDO...' : 'GUARDAR CAMBIOS',
      icon: Icons.save_rounded,
      variant: NeobrutalistButtonVariant.accent,
      expand: !isDesktop,
      onPressed: _isSaving ? null : _handleSave,
    );
    final cancelButton = NeobrutalistButton(
      label: 'CANCELAR',
      icon: Icons.close_rounded,
      variant: NeobrutalistButtonVariant.secondary,
      expand: !isDesktop,
      onPressed: _isSaving ? null : _cancelEditing,
    );

    return Container(
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.button,
      ),
      child: isDesktop
          ? Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                SizedBox(width: 180, child: cancelButton),
                const SizedBox(width: AppDimens.spaceMd),
                SizedBox(width: 220, child: saveButton),
              ],
            )
          : Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                saveButton,
                const SizedBox(height: AppDimens.spaceSm),
                cancelButton,
              ],
            ),
    );
  }

  // ---------------------------------------------------------------------
  // ESTADOS DE CARGA Y ERROR
  // ---------------------------------------------------------------------
  Widget _buildLoadingCard() {
    return Container(
      height: 220,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.card,
      ),
      child: const Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          CircularProgressIndicator(color: AppColors.text),
          SizedBox(height: AppDimens.spaceMd),
          Text(
            'CARGANDO PERFIL...',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
              color: AppColors.muted,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildErrorState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(AppDimens.spaceXl),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded, size: 48, color: AppColors.text),
            const SizedBox(height: AppDimens.spaceMd),
            const Text(
              'NO SE PUDO CARGAR TU PERFIL (REAL)',
              textAlign: TextAlign.center,
              style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text),
            ),
            if (_errorMessage != null) ...[
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                _errorMessage!,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 12,
                  color: AppColors.mutedStrong,
                ),
              ),
            ],
            const SizedBox(height: AppDimens.spaceLg),
            SizedBox(
              width: 220,
              child: NeobrutalistButton(
                label: 'REINTENTAR',
                icon: Icons.refresh_rounded,
                variant: NeobrutalistButtonVariant.primary,
                expand: true,
                onPressed: _loadProfile,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------
// WIDGETS AUXILIARES PRIVADOS (tarjetas, campos y filas neobrutalistas)
// ---------------------------------------------------------------------

class _ProfileCard extends StatelessWidget {
  const _ProfileCard({required this.title, required this.child, this.trailing});

  final String title;
  final Widget child;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.card,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(
              AppDimens.spaceLg,
              14,
              AppDimens.spaceLg,
              AppDimens.spaceMd,
            ),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    title.toUpperCase(),
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w900,
                      letterSpacing: 0.6,
                      color: AppColors.text,
                    ),
                  ),
                ),
                ?trailing,
              ],
            ),
          ),
          const Divider(color: AppColors.border, thickness: AppDimens.borderWidth, height: 1),
          Padding(
            padding: const EdgeInsets.all(AppDimens.spaceLg),
            child: child,
          ),
        ],
      ),
    );
  }
}

class _ProfileTextField extends StatelessWidget {
  const _ProfileTextField({
    required this.label,
    required this.controller,
    this.hint,
    this.prefixIcon,
    this.keyboardType,
    this.textInputAction,
    this.maxLines = 1,
    this.maxLength,
    this.validator,
  });

  final String label;
  final TextEditingController controller;
  final String? hint;
  final IconData? prefixIcon;
  final TextInputType? keyboardType;
  final TextInputAction? textInputAction;
  final int maxLines;
  final int? maxLength;
  final String? Function(String?)? validator;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        AppFieldLabel(label),
        const SizedBox(height: AppDimens.spaceSm),
        Container(
          decoration: const BoxDecoration(boxShadow: AppShadows.badge),
          child: TextFormField(
            controller: controller,
            keyboardType: keyboardType,
            textInputAction: textInputAction,
            maxLines: maxLines,
            maxLength: maxLength,
            validator: validator,
            cursorColor: AppColors.text,
            style: const TextStyle(
              color: AppColors.text,
              fontWeight: FontWeight.w700,
              fontSize: 14,
            ),
            decoration: appInputDecoration(hint ?? '').copyWith(
              prefixIcon: prefixIcon == null
                  ? null
                  : Icon(prefixIcon, size: 18, color: AppColors.text),
              counterStyle: const TextStyle(
                color: AppColors.muted,
                fontWeight: FontWeight.w700,
                fontSize: 11,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _EndpointTag extends StatelessWidget {
  const _EndpointTag(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w800,
        color: AppColors.muted,
        fontFamily: 'monospace',
        letterSpacing: 0.3,
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
    final initials = name
        .trim()
        .split(RegExp(r'\s+'))
        .where((p) => p.isNotEmpty)
        .take(2)
        .map((p) => p[0].toUpperCase())
        .join();

    final fallback = Text(
      initials.isEmpty ? 'U' : initials,
      style: TextStyle(fontSize: size * 0.32, fontWeight: FontWeight.w900, color: AppColors.text),
    );

    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      clipBehavior: Clip.antiAlias,
      decoration: BoxDecoration(
        color: AppColors.accentYellow,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
        boxShadow: AppShadows.badge,
      ),
      child: photoUrl.isNotEmpty
          ? Image.network(
              photoUrl,
              fit: BoxFit.cover,
              width: size,
              height: size,
              errorBuilder: (_, _, _) => fallback,
            )
          : fallback,
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
        Expanded(
          child: _VisibilityOption(
            label: 'PÚBLICO',
            icon: Icons.visibility_rounded,
            active: value == 'public',
            onTap: () => onChanged('public'),
          ),
        ),
        const SizedBox(width: AppDimens.spaceSm),
        Expanded(
          child: _VisibilityOption(
            label: 'PRIVADO',
            icon: Icons.visibility_off_rounded,
            active: value == 'private',
            onTap: () => onChanged('private'),
          ),
        ),
      ],
    );
  }
}

class _VisibilityOption extends StatelessWidget {
  final String label;
  final IconData icon;
  final bool active;
  final VoidCallback onTap;

  const _VisibilityOption({
    required this.label,
    required this.icon,
    required this.active,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      selected: active,
      label: label,
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: onTap,
          child: AnimatedContainer(
            duration: AppMotion.fast,
            curve: AppMotion.standard,
            padding: const EdgeInsets.symmetric(vertical: AppDimens.spaceMd),
            decoration: BoxDecoration(
              color: active ? AppColors.accentYellow : AppColors.bg,
              border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: active ? AppShadows.badge : null,
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(icon, size: 16, color: AppColors.text),
                const SizedBox(width: AppDimens.spaceXs),
                Text(
                  label,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                    letterSpacing: 0.4,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
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
          width: 32,
          height: 32,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: AppColors.bg,
            border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
            borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
          ),
          child: Icon(icon, size: 16, color: AppColors.text),
        ),
        const SizedBox(width: AppDimens.spaceSm),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w800,
                  color: AppColors.muted,
                  letterSpacing: 0.5,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                value,
                style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: AppColors.text),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
