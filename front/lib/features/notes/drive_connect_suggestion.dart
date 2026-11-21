import 'package:flutter/material.dart';

import '../../core/services/profile_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Devuelve true si la cuenta tiene Google Drive conectado
/// (`GET /auth/google-drive/status` → `{connected}`).
typedef DriveStatusChecker = Future<bool> Function();

/// Aviso en Apuntes cuando la cuenta no tiene Google Drive conectado.
///
/// - Aparece al entrar a Apuntes en cuanto Auth confirma `connected=false`;
///   no hace falta intentar ninguna acción.
/// - Se consulta el estado real cada vez que se abre Apuntes: al conectar
///   Drive desaparece y, si se desconecta, vuelve a aparecer.
/// - «Ocultar por ahora» solo dura la sesión actual de esa cuenta (memoria):
///   no lo oculta para siempre ni para otra cuenta, y se olvida al cerrar
///   sesión o al confirmar que Drive quedó conectado.
/// - Si el estado no se puede consultar no se afirma nada (no se muestra); las
///   operaciones que requieren Drive siguen validándose en el backend.
/// - No bloquea la consulta de apuntes.
class DriveConnectSuggestion extends StatefulWidget {
  const DriveConnectSuggestion({
    super.key,
    this.checkStatus,
    this.onOpenProfile,
  });

  /// Consulta inyectable (pruebas). Por defecto ProfileService.getDriveStatus.
  final DriveStatusChecker? checkStatus;

  /// Lleva a la tarjeta «Google Drive» del perfil (botón «CONECTAR DRIVE»).
  final VoidCallback? onOpenProfile;

  /// Cuentas que ocultaron el aviso en esta sesión de la app.
  static final Set<String> _hiddenThisSession = <String>{};
  static bool _listeningSession = false;

  /// Olvida los «ocultar por ahora» al cerrar sesión, aunque Apuntes no esté
  /// abierto en ese momento.
  static void _ensureSessionListener() {
    if (_listeningSession) return;
    _listeningSession = true;
    SessionManager.revision.addListener(() {
      if (SessionManager.token == null) _hiddenThisSession.clear();
    });
  }

  @visibleForTesting
  static void resetSessionStateForTesting() => _hiddenThisSession.clear();

  @override
  State<DriveConnectSuggestion> createState() => _DriveConnectSuggestionState();
}

class _DriveConnectSuggestionState extends State<DriveConnectSuggestion> {
  bool _visible = false;
  String? _userId;

  @override
  void initState() {
    super.initState();
    DriveConnectSuggestion._ensureSessionListener();
    SessionManager.revision.addListener(_evaluate);
    _evaluate();
  }

  @override
  void dispose() {
    SessionManager.revision.removeListener(_evaluate);
    super.dispose();
  }

  static String? _currentUserId() {
    final id = SessionManager.user?['id'];
    return id is String && id.trim().isNotEmpty ? id.trim() : null;
  }

  Future<void> _evaluate() async {
    final revision = SessionManager.revision.value;
    final userId = _currentUserId();
    if (mounted && _visible) setState(() => _visible = false);
    if (userId == null || SessionManager.token == null) return;

    bool connected;
    try {
      connected = await _check();
    } catch (_) {
      // Estado desconocido: no se afirma que esté o no conectado.
      return;
    }
    if (!mounted ||
        revision != SessionManager.revision.value ||
        userId != _currentUserId()) {
      return;
    }
    if (connected) {
      // Conectado: se olvida el «ocultar» para que, si luego se desconecta,
      // el aviso vuelva a aparecer.
      DriveConnectSuggestion._hiddenThisSession.remove(userId);
    }
    setState(() {
      _userId = userId;
      _visible =
          !connected &&
          !DriveConnectSuggestion._hiddenThisSession.contains(userId);
    });
  }

  Future<bool> _check() async {
    final custom = widget.checkStatus;
    if (custom != null) return custom();
    final service = ProfileService();
    try {
      return await service.getDriveStatus();
    } finally {
      service.dispose();
    }
  }

  void _hide() {
    final userId = _userId;
    if (userId != null) DriveConnectSuggestion._hiddenThisSession.add(userId);
    setState(() => _visible = false);
  }

  @override
  Widget build(BuildContext context) {
    if (!_visible) return const SizedBox.shrink();
    final compact =
        MediaQuery.sizeOf(context).width < AppDimens.breakpointCompact;

    final icon = Container(
      width: 44,
      height: 44,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: const Icon(
        Icons.cloud_off_rounded,
        color: AppColors.text,
        size: 24,
      ),
    );

    final close = NeobrutalistIconButton(
      key: const Key('drive-suggestion-dismiss'),
      icon: Icons.close_rounded,
      tooltip: 'Ocultar por ahora',
      size: 36,
      onPressed: _hide,
    );

    final header = Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        icon,
        const SizedBox(width: AppDimens.spaceMd),
        const Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                'Google Drive no está conectado',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                ),
              ),
              SizedBox(height: AppDimens.spaceXs),
              Text(
                'Tu cuenta todavía no está vinculada a Google Drive. Sigma '
                'Academy guarda tus apuntes y sus archivos en tu Drive: '
                'conéctalo para guardarlos en la nube, adjuntar archivos y '
                'copiar apuntes de otros usuarios. Mientras tanto, puedes '
                'seguir consultando apuntes.',
                style: TextStyle(
                  fontSize: 13,
                  height: 1.4,
                  fontWeight: FontWeight.w600,
                  color: AppColors.text,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: AppDimens.spaceSm),
        close,
      ],
    );

    // Ruta exacta hasta la opción existente.
    Widget step(String label) => Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceSm,
        vertical: 2,
      ),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Text(
        label,
        style: const TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );
    const arrow = Icon(
      Icons.chevron_right_rounded,
      size: 18,
      color: AppColors.text,
    );
    final where = Wrap(
      key: const Key('drive-suggestion-where'),
      crossAxisAlignment: WrapCrossAlignment.center,
      spacing: AppDimens.spaceXs,
      runSpacing: AppDimens.spaceXs,
      children: <Widget>[
        const Text(
          '¿DÓNDE CONECTARLO?',
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w900,
            letterSpacing: 0.4,
            color: AppColors.text,
          ),
        ),
        step('Perfil'),
        arrow,
        step('Google Drive'),
        arrow,
        step('CONECTAR DRIVE'),
      ],
    );

    final goButton = NeobrutalistButton(
      key: const Key('drive-suggestion-open-profile'),
      label: 'Conectar Google Drive',
      icon: Icons.add_link_rounded,
      variant: NeobrutalistButtonVariant.secondary,
      expand: compact,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceMd,
        vertical: AppDimens.spaceSm,
      ),
      onPressed: widget.onOpenProfile,
    );

    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceLg),
      child: NeobrutalistCard(
        key: const Key('drive-connect-suggestion'),
        semanticLabel: 'Aviso: Google Drive no está conectado',
        backgroundColor: AppColors.accentYellow,
        padding: const EdgeInsets.all(AppDimens.spaceMd),
        shadow: AppShadows.hard2,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            header,
            const SizedBox(height: AppDimens.spaceMd),
            if (compact) ...<Widget>[
              where,
              const SizedBox(height: AppDimens.spaceMd),
              goButton,
            ] else
              Row(
                children: <Widget>[
                  Expanded(child: where),
                  const SizedBox(width: AppDimens.spaceMd),
                  goButton,
                ],
              ),
          ],
        ),
      ),
    );
  }
}
