import 'package:flutter/material.dart';

import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import 'notes_api.dart';

/// Messenger raíz de la app (`MaterialApp.scaffoldMessengerKey`). Permite
/// informar el resultado de una copia aunque su pantalla ya se haya cerrado.
final GlobalKey<ScaffoldMessengerState> appScaffoldMessengerKey =
    GlobalKey<ScaffoldMessengerState>();

/// Una copia en curso y la sesión que la inició.
class _CopyOp {
  _CopyOp(this.session, this.future);

  final String session;
  final Future<NoteCopyResult> future;

  /// La pantalla que la inició se cerró: su resultado lo informa el aviso
  /// global (solo si la sesión sigue siendo la misma).
  bool detached = false;
}

/// Copias de apuntes en curso, independientes de la pantalla que las inició.
///
/// Volver atrás no interrumpe una copia: la petición sigue viva (NotesApi
/// difiere su cierre) y el backend termina el trabajo aunque el cliente se
/// desconecte. Este coordinador:
/// - reutiliza la copia en curso de un apunte en vez de lanzar otra;
/// - avisa con [completed] cuando una copia termina bien, para que «Mis
///   apuntes» se refresque;
/// - si la pantalla que la inició ya no está ([detach]), informa el resultado
///   real con el messenger global: éxito solo cuando terminó bien.
///
/// Todo está aislado por sesión ([sessionKey] = usuario + revisión de
/// sesión): tras cerrar sesión o cambiar de cuenta, la copia anterior sigue en
/// segundo plano (no se cancela) pero ya no se muestra, no bloquea copias de
/// la nueva sesión y su resultado no se notifica a nadie.
class NoteCopyRunner {
  NoteCopyRunner._();

  /// Clave `sesión|noteId` → copia en curso.
  static final Map<String, _CopyOp> _ops = <String, _CopyOp>{};

  /// Se incrementa cada vez que una copia de la sesión actual termina bien.
  static final ValueNotifier<int> completed = ValueNotifier<int>(0);

  /// Identidad de la sesión actual, o null sin sesión. La revisión cambia al
  /// iniciar, restaurar o cerrar sesión (no al refrescar el token), así que
  /// volver a entrar con la misma cuenta también es una sesión nueva.
  static String? sessionKey() {
    final fromUser = SessionManager.user?['id'];
    final userId = fromUser is String && fromUser.trim().isNotEmpty
        ? fromUser.trim()
        : SessionManager.currentUserId;
    if (userId == null || userId.isEmpty) return null;
    return '$userId@${SessionManager.revision.value}';
  }

  /// true si [session] (capturada al iniciar) sigue siendo la sesión actual.
  static bool isCurrentSession(String? session) => session == sessionKey();

  static String _key(String session, String noteId) => '$session|$noteId';

  static _CopyOp? _currentOp(String noteId) {
    final session = sessionKey();
    return session == null ? null : _ops[_key(session, noteId)];
  }

  /// Copia en curso de [noteId] de la sesión actual, o null.
  static Future<NoteCopyResult>? pending(String noteId) =>
      _currentOp(noteId)?.future;

  /// Inicia la copia de [noteId] o devuelve la que la sesión actual ya tiene
  /// en curso. Las copias de otras sesiones no se reutilizan.
  static Future<NoteCopyResult> start(
    NotesApi api,
    String noteId, {
    required String idempotencyKey,
  }) {
    final session = sessionKey();
    if (session == null) {
      // Sin sesión no hay a quién atribuir ni informar la copia.
      return api.copyNote(noteId, idempotencyKey: idempotencyKey);
    }
    final key = _key(session, noteId);
    final running = _ops[key];
    if (running != null) return running.future;
    final future = api.copyNote(noteId, idempotencyKey: idempotencyKey);
    final op = _CopyOp(session, future);
    _ops[key] = op;
    future
        .then<void>(
          (_) {
            if (isCurrentSession(op.session)) completed.value++;
            _report(op, null);
          },
          onError: (Object error) {
            _report(op, error);
          },
        )
        .whenComplete(() {
          if (identical(_ops[key], op)) _ops.remove(key);
        });
    return future;
  }

  /// La pantalla que esperaba la copia se cerró: su resultado se informará
  /// con el messenger global (si la sesión no cambia antes).
  static void detach(String noteId) => _currentOp(noteId)?.detached = true;

  /// Una pantalla vuelve a seguir la copia: ella informará el resultado.
  static void attach(String noteId) => _currentOp(noteId)?.detached = false;

  static void _report(_CopyOp op, Object? error) {
    // Nunca se informa a otra sesión (otra cuenta o tras cerrar sesión).
    if (!op.detached || !isCurrentSession(op.session)) return;
    final messenger = appScaffoldMessengerKey.currentState;
    if (messenger == null) return;
    final String message;
    final bool ok;
    if (error == null) {
      message = 'Copia guardada en tus apuntes y en tu Drive.';
      ok = true;
    } else if (error is NotesApiException) {
      message = error.message;
      ok = false;
    } else {
      // Sin respuesta (red): el servidor pudo terminarla igualmente.
      message =
          'No pudimos confirmar la copia. Revisa «Mis apuntes» en unos '
          'segundos.';
      ok = false;
    }
    messenger.showSnackBar(
      SnackBar(
        backgroundColor: ok ? AppColors.border : AppColors.errorDeep,
        content: Text(
          message,
          style: TextStyle(
            color: AppColors.surface,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  @visibleForTesting
  static void resetForTesting() => _ops.clear();
}
