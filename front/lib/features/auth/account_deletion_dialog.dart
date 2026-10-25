import 'dart:async';

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/services/account_deletion_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Cómo terminó el diálogo; el llamador decide la navegación.
enum AccountDeletionExit {
  /// Nada cambió en este dispositivo.
  cancelled,

  /// Identity confirmó el borrado completo; sesión y datos locales limpios.
  deleted,

  /// Identity no tiene la cuenta (404); se limpió este dispositivo.
  accountNotFound,

  /// Borrado iniciado (202) y el usuario salió: la cuenta ya está bloqueada y
  /// el servidor termina la limpieza; se limpió este dispositivo.
  pendingSignedOut,

  /// El JWT ya no sirve; hay que iniciar sesión.
  sessionExpired,
}

/// Palabra que el usuario debe escribir para habilitar el borrado.
const String kAccountDeletionConfirmWord = 'ELIMINAR';

/// Muestra el flujo de borrado de cuenta. Nunca devuelve null.
Future<AccountDeletionExit> showAccountDeletionDialog(
  BuildContext context, {
  required AccountDeletionService service,
  String? email,
  Duration pollInterval = const Duration(seconds: 4),
  int maxPolls = 15,
}) async {
  final userId = SessionManager.currentUserId;
  if (userId == null) return AccountDeletionExit.sessionExpired;
  final exit = await showNeobrutalistDialog<AccountDeletionExit>(
    context: context,
    // Solo los botones cierran: un toque fuera no debe cancelar a medias.
    barrierDismissible: false,
    dialog: AccountDeletionDialog(
      service: service,
      userId: userId,
      email: email,
      pollInterval: pollInterval,
      maxPolls: maxPolls,
    ),
  );
  return exit ?? AccountDeletionExit.cancelled;
}

enum _Phase { confirm, processing, pending, retryable, unknown, unauthorized }

class AccountDeletionDialog extends StatefulWidget {
  const AccountDeletionDialog({
    super.key,
    required this.service,
    required this.userId,
    this.email,
    this.pollInterval = const Duration(seconds: 4),
    this.maxPolls = 15,
  });

  final AccountDeletionService service;

  /// Cuenta capturada al abrir: la limpieza local nunca usa la sesión del
  /// momento de la respuesta, que podría ser otra.
  final String userId;
  final String? email;
  final Duration pollInterval;
  final int maxPolls;

  @override
  State<AccountDeletionDialog> createState() => _AccountDeletionDialogState();
}

class _AccountDeletionDialogState extends State<AccountDeletionDialog> {
  final _confirmController = TextEditingController();
  _Phase _phase = _Phase.confirm;
  bool _busy = false;
  bool _finalizing = false;
  String? _detail;
  Timer? _pollTimer;
  int _polls = 0;
  late final int _revision;

  bool get _confirmed =>
      _confirmController.text.trim().toUpperCase() ==
      kAccountDeletionConfirmWord;

  @override
  void initState() {
    super.initState();
    _revision = SessionManager.revision.value;
    _confirmController.addListener(_onConfirmChanged);
    SessionManager.revision.addListener(_onSessionChanged);
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    SessionManager.revision.removeListener(_onSessionChanged);
    _confirmController
      ..removeListener(_onConfirmChanged)
      ..dispose();
    super.dispose();
  }

  void _onConfirmChanged() => setState(() {});

  void _onSessionChanged() {
    // La limpieza propia también cambia la revisión; esa no cancela.
    if (_finalizing || !mounted) return;
    if (SessionManager.revision.value == _revision) return;
    if (_phase == _Phase.pending && !SessionManager.isLoggedIn) {
      // Con el borrado ya iniciado Identity rechaza el refresh: al vencer el
      // access la sesión expira sola. La cuenta ya está bloqueada, así que se
      // purgan igualmente sus datos de este dispositivo.
      unawaited(_finish(AccountDeletionExit.pendingSignedOut));
      return;
    }
    if (!SessionManager.isLoggedIn) {
      // Expiró antes de iniciar el borrado: se explica en vez de cerrar. Si la
      // app tiene manejador global de expiración, él navega al login.
      _pollTimer?.cancel();
      setState(() {
        _busy = false;
        _phase = _Phase.unauthorized;
      });
      return;
    }
    // Otra cuenta tomó la sesión: este diálogo ya no la representa y no se
    // toca nada local.
    _close(AccountDeletionExit.cancelled);
  }

  void _close(AccountDeletionExit exit) {
    _pollTimer?.cancel();
    _popIfCurrent(exit);
  }

  /// Cierra el diálogo solo si sigue siendo la ruta superior. Si el manejador
  /// global de sesión expirada ya reemplazó la pila por el login, este diálogo
  /// sigue montado durante la transición pero ya fue retirado: un pop aquí
  /// quitaría el login y dejaría el navegador vacío (pantalla negra).
  void _popIfCurrent(AccountDeletionExit exit) {
    if (!mounted) return;
    if (ModalRoute.of(context)?.isCurrent != true) return;
    Navigator.of(context).pop(exit);
  }

  Future<void> _submit() async {
    if (_busy || !_confirmed) return;
    setState(() {
      _busy = true;
      _phase = _Phase.processing;
      _detail = null;
    });
    final result = await widget.service.requestDeletion();
    if (!mounted) return;
    await _apply(result);
  }

  Future<void> _checkStatus() async {
    if (_busy) return;
    setState(() => _busy = true);
    final result = await widget.service.checkStatus();
    if (!mounted) return;
    await _apply(result, fromStatus: true);
  }

  Future<void> _apply(
    AccountDeletionResult result, {
    bool fromStatus = false,
  }) async {
    switch (result.state) {
      case AccountDeletionState.complete:
        await _finish(AccountDeletionExit.deleted);
      case AccountDeletionState.accountNotFound:
        await _finish(AccountDeletionExit.accountNotFound);
      case AccountDeletionState.pending:
        setState(() {
          _busy = false;
          _phase = _Phase.pending;
          _detail = null;
        });
        _schedulePoll();
      case AccountDeletionState.notStarted:
        setState(() {
          _busy = false;
          _phase = _Phase.retryable;
          _detail = _failureDetail(result);
        });
      case AccountDeletionState.unauthorized:
        _pollTimer?.cancel();
        if (_phase == _Phase.pending) {
          await _finish(AccountDeletionExit.pendingSignedOut);
          return;
        }
        setState(() {
          _busy = false;
          _phase = _Phase.unauthorized;
        });
      case AccountDeletionState.sessionChanged:
        _close(AccountDeletionExit.cancelled);
      case AccountDeletionState.unknown:
        if (_phase == _Phase.pending && fromStatus) {
          // Ya se sabía que comenzó: una consulta fallida no lo cambia.
          setState(() {
            _busy = false;
            _detail = 'No se pudo consultar el estado ahora.';
          });
          _schedulePoll();
          return;
        }
        setState(() {
          _busy = false;
          _phase = _Phase.unknown;
          _detail = _failureDetail(result);
        });
    }
  }

  void _schedulePoll() {
    _pollTimer?.cancel();
    if (_polls >= widget.maxPolls) return;
    _pollTimer = Timer(widget.pollInterval, () {
      if (!mounted || _phase != _Phase.pending || _busy) return;
      _polls++;
      _checkStatus();
    });
  }

  Future<void> _finish(AccountDeletionExit exit) async {
    if (_finalizing) return;
    _pollTimer?.cancel();
    _finalizing = true;
    // La purga corre aunque otro flujo ya haya navegado al login.
    await widget.service.finalizeLocal(
      userId: widget.userId,
      email: widget.email,
    );
    _popIfCurrent(exit);
  }

  Future<void> _leaveWhilePending() async {
    if (_busy) return;
    setState(() => _busy = true);
    await _finish(AccountDeletionExit.pendingSignedOut);
  }

  String _failureDetail(AccountDeletionResult result) {
    final status = result.httpStatus;
    if (status == null) {
      return 'Sin respuesta del servidor (conexión o tiempo de espera).';
    }
    if (status == 503) {
      return 'El servicio de cuentas no está disponible (503).';
    }
    if (status >= 500) return 'Error del servidor ($status).';
    return 'Respuesta inesperada del servidor ($status).';
  }

  // ---------------------------------------------------------------------
  // UI
  // ---------------------------------------------------------------------
  @override
  Widget build(BuildContext context) {
    final canPop = _phase == _Phase.confirm || _phase == _Phase.retryable;
    // El botón atrás no cierra mientras procesa ni con el borrado iniciado:
    // esas salidas tienen botones explícitos.
    return PopScope(
      canPop: canPop && !_busy,
      child: NeobrutalistDialog(
        title: _title,
        content: _content(),
        actions: _actions(),
      ),
    );
  }

  String get _title {
    switch (_phase) {
      case _Phase.confirm:
      case _Phase.processing:
        return 'Eliminar cuenta';
      case _Phase.pending:
        return 'Eliminación en curso';
      case _Phase.retryable:
        return 'No se eliminó la cuenta';
      case _Phase.unknown:
        return 'Estado sin confirmar';
      case _Phase.unauthorized:
        return 'Sesión expirada';
    }
  }

  Widget _content() {
    switch (_phase) {
      case _Phase.confirm:
      case _Phase.processing:
        return _confirmContent();
      case _Phase.pending:
        return _pendingContent();
      case _Phase.retryable:
        return _messageContent(
          badge: const NeobrutalistBadge(
            label: 'No comenzó',
            tone: NeobrutalistTone.neutral,
            icon: Icons.info_outline_rounded,
          ),
          message:
              'El servidor confirmó que la eliminación no comenzó. Tu cuenta '
              'y tus datos siguen intactos; puedes reintentar.',
        );
      case _Phase.unknown:
        return _messageContent(
          badge: const NeobrutalistBadge(
            label: 'Sin confirmar',
            tone: NeobrutalistTone.accent,
            icon: Icons.help_outline_rounded,
          ),
          message:
              'No pudimos confirmar si la eliminación comenzó. Consulta el '
              'estado antes de volver a intentarlo; no se repetirá a ciegas.',
        );
      case _Phase.unauthorized:
        return _messageContent(
          badge: const NeobrutalistBadge(
            label: 'Sesión vencida',
            tone: NeobrutalistTone.pending,
            icon: Icons.lock_clock_rounded,
          ),
          message:
              'Tu sesión ya no es válida. Inicia sesión de nuevo para '
              'eliminar la cuenta o consultar el estado de la eliminación.',
        );
    }
  }

  Widget _confirmContent() {
    final processing = _phase == _Phase.processing;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Align(
          alignment: Alignment.centerLeft,
          child: NeobrutalistBadge(
            label: 'Acción permanente',
            tone: NeobrutalistTone.danger,
            icon: Icons.warning_amber_rounded,
          ),
        ),
        const SizedBox(height: AppDimens.spaceMd),
        const Text(
          'Tu cuenta se eliminará de forma definitiva y no podrás recuperarla '
          'ni volver a iniciar sesión con ella.',
          style: _bodyStyle,
        ),
        const SizedBox(height: AppDimens.spaceMd),
        const _ConsequenceRow(
          icon: Icons.badge_outlined,
          text: 'Perfil, sesiones y conexiones con Google Drive y Calendar.',
        ),
        const _ConsequenceRow(
          icon: Icons.groups_outlined,
          text:
              'Tu participación en grupos y chats. Si eras el último miembro, '
              'el grupo se elimina; si administrabas, otro miembro hereda el rol.',
        ),
        const _ConsequenceRow(
          icon: Icons.share_outlined,
          text:
              'Los apuntes que compartiste dejan de estar disponibles para '
              'otros. Los archivos en tu Google Drive personal no se borran.',
        ),
        const SizedBox(height: AppDimens.spaceLg),
        const AppFieldLabel(
          'ESCRIBE $kAccountDeletionConfirmWord PARA CONFIRMAR',
        ),
        const SizedBox(height: AppDimens.spaceSm),
        Semantics(
          textField: true,
          label: 'Confirmación: escribe $kAccountDeletionConfirmWord',
          child: TextField(
            key: const ValueKey('account-deletion-confirm-field'),
            controller: _confirmController,
            enabled: !processing,
            autocorrect: false,
            enableSuggestions: false,
            textCapitalization: TextCapitalization.characters,
            textInputAction: TextInputAction.done,
            onSubmitted: (_) => _submit(),
            style: const TextStyle(
              fontWeight: FontWeight.w900,
              letterSpacing: 0.5,
              color: AppColors.text,
            ),
            decoration: appInputDecoration(kAccountDeletionConfirmWord),
          ),
        ),
        if (processing) ...[
          const SizedBox(height: AppDimens.spaceMd),
          const _StatusLine(
            icon: Icons.hourglass_top_rounded,
            text: 'Eliminando tu cuenta en Identity, Social y Notes…',
          ),
        ],
      ],
    );
  }

  Widget _pendingContent() {
    final exhausted = _polls >= widget.maxPolls;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Align(
          alignment: Alignment.centerLeft,
          child: NeobrutalistBadge(
            label: 'Pendiente',
            tone: NeobrutalistTone.pending,
            icon: Icons.hourglass_bottom_rounded,
          ),
        ),
        const SizedBox(height: AppDimens.spaceMd),
        const Text(
          'La eliminación comenzó y tu cuenta ya está bloqueada: no se puede '
          'usar ni volver a iniciar sesión. Aún falta que Social y Notes '
          'confirmen la limpieza; el servidor la reintenta automáticamente.',
          style: _bodyStyle,
        ),
        const SizedBox(height: AppDimens.spaceMd),
        _StatusLine(
          icon: _busy ? Icons.sync_rounded : Icons.schedule_rounded,
          text: _busy
              ? 'Consultando estado…'
              : exhausted
              ? 'Sigue pendiente. Puedes consultar de nuevo más tarde.'
              : 'Consultaremos el estado automáticamente.',
        ),
        if (_detail != null) ...[
          const SizedBox(height: AppDimens.spaceSm),
          Text(_detail!, style: _secondaryStyle),
        ],
      ],
    );
  }

  Widget _messageContent({required Widget badge, required String message}) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Align(alignment: Alignment.centerLeft, child: badge),
        const SizedBox(height: AppDimens.spaceMd),
        Text(message, style: _bodyStyle),
        if (_detail != null) ...[
          const SizedBox(height: AppDimens.spaceSm),
          Text(_detail!, style: _secondaryStyle),
        ],
      ],
    );
  }

  List<Widget> _actions() {
    switch (_phase) {
      case _Phase.confirm:
      case _Phase.processing:
        final processing = _phase == _Phase.processing;
        return [
          NeobrutalistButton(
            label: 'Cancelar',
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: processing
                ? null
                : () => _close(AccountDeletionExit.cancelled),
          ),
          NeobrutalistButton(
            key: const ValueKey('account-deletion-submit'),
            label: processing ? 'Eliminando…' : 'Eliminar cuenta',
            icon: Icons.delete_forever_rounded,
            variant: NeobrutalistButtonVariant.danger,
            borderWidth: AppDimens.borderWidthThick,
            onPressed: processing || !_confirmed ? null : _submit,
          ),
        ];
      case _Phase.pending:
        return [
          NeobrutalistButton(
            label: 'Consultar estado',
            icon: Icons.refresh_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: _busy ? null : _checkStatus,
          ),
          NeobrutalistButton(
            label: 'Salir de este dispositivo',
            icon: Icons.logout_rounded,
            variant: NeobrutalistButtonVariant.primary,
            onPressed: _busy ? null : _leaveWhilePending,
          ),
        ];
      case _Phase.retryable:
        return [
          NeobrutalistButton(
            label: 'Cancelar',
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: () => _close(AccountDeletionExit.cancelled),
          ),
          NeobrutalistButton(
            key: const ValueKey('account-deletion-retry'),
            label: 'Reintentar',
            icon: Icons.delete_forever_rounded,
            variant: NeobrutalistButtonVariant.danger,
            borderWidth: AppDimens.borderWidthThick,
            onPressed: _busy ? null : _submit,
          ),
        ];
      case _Phase.unknown:
        return [
          NeobrutalistButton(
            label: 'Cerrar',
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: _busy
                ? null
                : () => _close(AccountDeletionExit.cancelled),
          ),
          NeobrutalistButton(
            label: _busy ? 'Consultando…' : 'Consultar estado',
            icon: Icons.refresh_rounded,
            variant: NeobrutalistButtonVariant.primary,
            onPressed: _busy ? null : _checkStatus,
          ),
        ];
      case _Phase.unauthorized:
        return [
          NeobrutalistButton(
            label: 'Ir a iniciar sesión',
            icon: Icons.login_rounded,
            variant: NeobrutalistButtonVariant.primary,
            onPressed: () => _close(AccountDeletionExit.sessionExpired),
          ),
        ];
    }
  }
}

const TextStyle _bodyStyle = TextStyle(
  fontSize: 14,
  fontWeight: FontWeight.w700,
  color: AppColors.text,
  height: 1.4,
);

const TextStyle _secondaryStyle = TextStyle(
  fontSize: 13,
  fontWeight: FontWeight.w700,
  color: AppColors.mutedStrong,
  height: 1.35,
);

class _ConsequenceRow extends StatelessWidget {
  const _ConsequenceRow({required this.icon, required this.text});

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ExcludeSemantics(child: Icon(icon, size: 18, color: AppColors.text)),
          const SizedBox(width: AppDimens.spaceSm),
          Expanded(child: Text(text, style: _secondaryStyle)),
        ],
      ),
    );
  }
}

/// Estado textual sin animación indefinida (respeta Reduce Motion).
class _StatusLine extends StatelessWidget {
  const _StatusLine({required this.icon, required this.text});

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      liveRegion: true,
      child: Container(
        padding: const EdgeInsets.all(AppDimens.spaceMd),
        decoration: BoxDecoration(
          color: AppColors.surfaceLow,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
        ),
        child: Row(
          children: [
            ExcludeSemantics(
              child: Icon(icon, size: 18, color: AppColors.text),
            ),
            const SizedBox(width: AppDimens.spaceSm),
            Expanded(
              child: Text(
                text,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w800,
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
