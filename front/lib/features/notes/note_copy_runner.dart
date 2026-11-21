import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import 'notes_api.dart';

/// Messenger raíz de la app (`MaterialApp.scaffoldMessengerKey`). Permite
/// informar el resultado de una copia aunque su pantalla ya se haya cerrado.
final GlobalKey<ScaffoldMessengerState> appScaffoldMessengerKey =
    GlobalKey<ScaffoldMessengerState>();

/// Copias de apuntes en curso, independientes de la pantalla que las inició.
///
/// Volver atrás ya no interrumpe una copia: la petición sigue viva (NotesApi
/// difiere su cierre) y el backend termina el trabajo aunque el cliente se
/// desconecte. Este coordinador:
/// - reutiliza la copia en curso de un apunte en vez de lanzar otra;
/// - avisa con [completed] cuando una copia termina bien, para que «Mis
///   apuntes» se refresque;
/// - si la pantalla que la inició ya no está ([detach]), informa el resultado
///   real con el messenger global: éxito solo cuando terminó bien.
class NoteCopyRunner {
  NoteCopyRunner._();

  static final Map<String, Future<NoteCopyResult>> _inFlight =
      <String, Future<NoteCopyResult>>{};
  static final Set<String> _detached = <String>{};

  /// Se incrementa cada vez que una copia termina correctamente.
  static final ValueNotifier<int> completed = ValueNotifier<int>(0);

  /// Copia en curso de [noteId], o null si no hay ninguna.
  static Future<NoteCopyResult>? pending(String noteId) => _inFlight[noteId];

  /// Inicia la copia de [noteId] o devuelve la que ya está en curso.
  static Future<NoteCopyResult> start(
    NotesApi api,
    String noteId, {
    required String idempotencyKey,
  }) {
    final running = _inFlight[noteId];
    if (running != null) return running;
    final future = api.copyNote(noteId, idempotencyKey: idempotencyKey);
    _inFlight[noteId] = future;
    future
        .then<void>(
          (_) {
            completed.value++;
            _report(noteId, null);
          },
          onError: (Object error) {
            _report(noteId, error);
          },
        )
        .whenComplete(() {
          _inFlight.remove(noteId);
          _detached.remove(noteId);
        });
    return future;
  }

  /// La pantalla que esperaba la copia se cerró: su resultado se informará
  /// con el messenger global.
  static void detach(String noteId) {
    if (_inFlight.containsKey(noteId)) _detached.add(noteId);
  }

  /// Una pantalla vuelve a seguir la copia: ella informará el resultado.
  static void attach(String noteId) => _detached.remove(noteId);

  static void _report(String noteId, Object? error) {
    if (!_detached.contains(noteId)) return;
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
          style: const TextStyle(
            color: AppColors.surface,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  @visibleForTesting
  static void resetForTesting() {
    _inFlight.clear();
    _detached.clear();
  }
}
