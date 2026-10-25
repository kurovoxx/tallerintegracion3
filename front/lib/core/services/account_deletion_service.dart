import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import '../database/app_database.dart';
import '../database/app_database_manager.dart';
import '../database/local_notes_repository.dart';
import 'api_config.dart';
import 'authed_client.dart';
import 'session_manager.dart';

// Cliente del borrado de cuenta de Identity (Auth).
// Contrato verificado en back/auth/cmd/server/main.go (rutas) y
// back/auth/internal/handler/http/account_deletion_handler.go:
//
//   DELETE /auth/account          (JWT; el user_id sale solo del token, sin cuerpo)
//     200 {"status":"complete"}  Identity, Social y Notes confirmaron.
//     202 {"status":"pending"}   Iniciado y durable; un worker lo termina.
//     404 {"error":"account_not_found"}
//     503 {"error":"account_deletion_unavailable"}  (puede haber comenzado)
//     401 {"error":{"code":...}} JWT ausente, inválido o vencido.
//   GET /auth/account-deletion    (mismo JWT, aun con la cuenta bloqueada)
//     200 {"status":"pending"|"complete","updated_at":...}
//     404 {"status":"not_started"}
//     503 {"error":"account_status_unavailable"}
//
// Ambas rutas usan RequireIdentity: aceptan el JWT aunque la cuenta ya esté en
// borrado. El resto de rutas protegidas responde 423 desde que comienza, y el
// refresh y el login quedan rechazados para siempre.

/// Estado del borrado tal como lo confirma Identity.
enum AccountDeletionState {
  /// 200 complete: la cuenta ya no existe en ningún servicio.
  complete,

  /// Iniciado y bloqueado; Social/Notes aún no confirmaron la limpieza.
  pending,

  /// Identity confirma que nunca empezó: se puede reintentar sin riesgo.
  notStarted,

  /// DELETE 404: el token es válido pero Identity no tiene esa cuenta.
  accountNotFound,

  /// 401 tras el refresh de [AuthedHttp]: hay que iniciar sesión.
  unauthorized,

  /// Sin respuesta concluyente (red, timeout, 5xx y la consulta de estado
  /// también falló). No se debe repetir el DELETE a ciegas.
  unknown,

  /// La sesión cambió mientras viajaba la petición: el resultado es de otra.
  sessionChanged,
}

class AccountDeletionResult {
  const AccountDeletionResult(
    this.state, {
    this.httpStatus,
    this.errorCode,
    this.verifiedByStatus = false,
  });

  final AccountDeletionState state;

  /// Código HTTP de la petición que originó el resultado (null si no hubo).
  final int? httpStatus;
  final String? errorCode;

  /// true si el estado se obtuvo de GET /auth/account-deletion tras una
  /// respuesta ambigua del DELETE.
  final bool verifiedByStatus;

  bool get isTerminalSuccess =>
      state == AccountDeletionState.complete ||
      state == AccountDeletionState.accountNotFound;

  @override
  String toString() =>
      'AccountDeletionResult($state, http=$httpStatus, code=$errorCode, '
      'verified=$verifiedByStatus)';
}

/// Lo que quedó limpio en este dispositivo tras confirmar el borrado.
class LocalAccountCleanup {
  const LocalAccountCleanup({
    required this.sessionCleared,
    required this.localDataPurged,
    required this.rememberedEmailRemoved,
  });

  final bool sessionCleared;
  final bool localDataPurged;
  final bool rememberedEmailRemoved;
}

/// Borra del SQLite local solo las filas del [userId] indicado.
typedef AccountLocalDataPurger = Future<void> Function(String userId);

/// Purga por dueño en la base compartida: notas, adjuntos pendientes y
/// tombstones del usuario. Las filas de otras cuentas no se tocan.
Future<void> purgeLocalAccountData(String userId) async {
  final manager = AppDatabaseManager.instance;
  final db = await manager.acquire();
  try {
    await manager.guard(() => purgeLocalAccountDataIn(db, userId));
  } finally {
    manager.release();
  }
}

/// Purga en una sola transacción las filas de [userId] en [db].
Future<void> purgeLocalAccountDataIn(AppDatabase db, String userId) {
  final repo = LocalNotesRepository(db);
  return db.transaction(() async {
    await repo.clearOwner(userId);
    await repo.clearOwnerTombstones(userId);
  });
}

class AccountDeletionService {
  AccountDeletionService({
    http.Client? client,
    AccountLocalDataPurger? purgeLocalData,
    this.requestTimeout = const Duration(seconds: 45),
  }) : _client = client ?? http.Client(),
       _purgeLocalData = purgeLocalData ?? purgeLocalAccountData;

  final http.Client _client;
  final AccountLocalDataPurger _purgeLocalData;

  /// Identity hace hasta cuatro llamadas gRPC de 30 s cada una. Al vencer, el
  /// resultado se consulta con GET en vez de repetir el DELETE.
  final Duration requestTimeout;

  static String get baseUrl => authApiBaseUrl;
  static const String deletePath = '/auth/account';
  static const String statusPath = '/auth/account-deletion';

  // Un solo DELETE en vuelo por cuenta, aunque haya varias instancias o
  // pulsaciones repetidas.
  static Future<AccountDeletionResult>? _inFlight;
  static String? _inFlightUser;

  @visibleForTesting
  static void resetInFlightForTesting() {
    _inFlight = null;
    _inFlightUser = null;
  }

  /// Solicita el borrado de la cuenta de la sesión actual.
  ///
  /// Nunca lanza. Una respuesta ambigua (red, timeout, 5xx, cuerpo inesperado)
  /// se resuelve con GET /auth/account-deletion antes de devolver.
  Future<AccountDeletionResult> requestDeletion() {
    final userId = SessionManager.currentUserId;
    final existing = _inFlight;
    if (existing != null && _inFlightUser == userId) return existing;

    late final Future<AccountDeletionResult> operation;
    operation = _requestDeletion().whenComplete(() {
      if (identical(_inFlight, operation)) {
        _inFlight = null;
        _inFlightUser = null;
      }
    });
    _inFlight = operation;
    _inFlightUser = userId;
    return operation;
  }

  Future<AccountDeletionResult> _requestDeletion() async {
    if (!SessionManager.isLoggedIn) {
      return const AccountDeletionResult(AccountDeletionState.unauthorized);
    }
    final revision = SessionManager.revision.value;
    http.Response response;
    try {
      response = await AuthedHttp.run(
        () => _client
            .delete(Uri.parse('$baseUrl$deletePath'), headers: _headers())
            .timeout(requestTimeout),
      );
    } catch (_) {
      if (SessionManager.revision.value != revision) {
        return const AccountDeletionResult(AccountDeletionState.sessionChanged);
      }
      // Red o timeout: el servidor pudo haber comenzado.
      return _resolveWithStatus(null, null);
    }
    if (SessionManager.revision.value != revision) {
      // Un 401 cuyo refresh fue rechazado expira la sesión (cambia la
      // revisión): sigue siendo un rechazo de esta misma cuenta.
      return AccountDeletionResult(
        _expiredByThisRequest(response)
            ? AccountDeletionState.unauthorized
            : AccountDeletionState.sessionChanged,
        httpStatus: response.statusCode,
      );
    }

    final body = _decode(response);
    final status = body?['status'];
    final code = _errorCode(body);
    switch (response.statusCode) {
      case 200:
        if (status == 'complete') {
          return const AccountDeletionResult(
            AccountDeletionState.complete,
            httpStatus: 200,
          );
        }
      case 202:
        if (status == 'pending') {
          return const AccountDeletionResult(
            AccountDeletionState.pending,
            httpStatus: 202,
          );
        }
      case 401:
        return AccountDeletionResult(
          AccountDeletionState.unauthorized,
          httpStatus: 401,
          errorCode: code,
        );
      case 404:
        if (code == 'account_not_found') {
          return AccountDeletionResult(
            AccountDeletionState.accountNotFound,
            httpStatus: 404,
            errorCode: code,
          );
        }
    }
    // 503 (puede haber dejado el tombstone), 403/409/5xx no contemplados por el
    // contrato o cuerpo inesperado: se consulta el estado real.
    return _resolveWithStatus(response.statusCode, code);
  }

  Future<AccountDeletionResult> _resolveWithStatus(
    int? httpStatus,
    String? errorCode,
  ) async {
    final checked = await checkStatus();
    switch (checked.state) {
      case AccountDeletionState.complete:
      case AccountDeletionState.pending:
      case AccountDeletionState.notStarted:
      case AccountDeletionState.unauthorized:
      case AccountDeletionState.sessionChanged:
        return AccountDeletionResult(
          checked.state,
          httpStatus: httpStatus,
          errorCode: errorCode,
          verifiedByStatus: true,
        );
      case AccountDeletionState.accountNotFound:
      case AccountDeletionState.unknown:
        return AccountDeletionResult(
          AccountDeletionState.unknown,
          httpStatus: httpStatus,
          errorCode: errorCode,
        );
    }
  }

  /// GET /auth/account-deletion. Nunca lanza.
  Future<AccountDeletionResult> checkStatus() async {
    if (!SessionManager.isLoggedIn) {
      return const AccountDeletionResult(AccountDeletionState.unauthorized);
    }
    final revision = SessionManager.revision.value;
    http.Response response;
    try {
      response = await AuthedHttp.run(
        () => _client
            .get(Uri.parse('$baseUrl$statusPath'), headers: _headers())
            .timeout(const Duration(seconds: 15)),
      );
    } catch (_) {
      if (SessionManager.revision.value != revision) {
        return const AccountDeletionResult(AccountDeletionState.sessionChanged);
      }
      return const AccountDeletionResult(AccountDeletionState.unknown);
    }
    if (SessionManager.revision.value != revision) {
      return AccountDeletionResult(
        _expiredByThisRequest(response)
            ? AccountDeletionState.unauthorized
            : AccountDeletionState.sessionChanged,
        httpStatus: response.statusCode,
      );
    }
    final body = _decode(response);
    final status = body?['status'];
    if (response.statusCode == 200 && status == 'complete') {
      return const AccountDeletionResult(
        AccountDeletionState.complete,
        httpStatus: 200,
      );
    }
    if (response.statusCode == 200 && status == 'pending') {
      return const AccountDeletionResult(
        AccountDeletionState.pending,
        httpStatus: 200,
      );
    }
    if (response.statusCode == 404 && status == 'not_started') {
      return const AccountDeletionResult(
        AccountDeletionState.notStarted,
        httpStatus: 404,
      );
    }
    if (response.statusCode == 401) {
      return AccountDeletionResult(
        AccountDeletionState.unauthorized,
        httpStatus: 401,
        errorCode: _errorCode(body),
      );
    }
    return AccountDeletionResult(
      AccountDeletionState.unknown,
      httpStatus: response.statusCode,
      errorCode: _errorCode(body),
    );
  }

  /// Limpieza local tras confirmar el borrado (o tras un 202 si el usuario
  /// decide salir: Identity ya bloqueó la cuenta de forma irreversible).
  ///
  /// - Cierra la sesión solo si sigue siendo la de [userId].
  /// - Purga del SQLite únicamente las filas de [userId].
  /// - Olvida el correo de Recordarme solo si es el de la cuenta borrada.
  /// No llama a /auth/logout: Identity ya borró o bloqueó los refresh tokens.
  Future<LocalAccountCleanup> finalizeLocal({
    required String userId,
    String? email,
  }) async {
    var sessionCleared = false;
    if (SessionManager.currentUserId == userId) {
      await SessionManager.clear();
      sessionCleared = true;
    }

    var purged = false;
    try {
      await _purgeLocalData(userId);
      purged = true;
    } catch (e) {
      debugPrint('[AccountDeletion] purga local incompleta: $e');
    }

    var emailRemoved = false;
    final normalized = email?.trim().toLowerCase();
    if (normalized != null && normalized.isNotEmpty) {
      try {
        final preferences = await SharedPreferences.getInstance();
        final remembered = preferences
            .getString(SessionManager.rememberedEmailKey)
            ?.trim()
            .toLowerCase();
        if (remembered == normalized) {
          emailRemoved = await preferences.remove(
            SessionManager.rememberedEmailKey,
          );
        }
      } catch (e) {
        debugPrint('[AccountDeletion] no se pudo olvidar el correo: $e');
      }
    }
    return LocalAccountCleanup(
      sessionCleared: sessionCleared,
      localDataPurged: purged,
      rememberedEmailRemoved: emailRemoved,
    );
  }

  static bool _expiredByThisRequest(http.Response response) =>
      response.statusCode == 401 && !SessionManager.isLoggedIn;

  Map<String, String> _headers() {
    final token = SessionManager.token;
    return {
      'Accept': 'application/json',
      if (token != null && token.isNotEmpty) 'Authorization': 'Bearer $token',
    };
  }

  static Map<String, dynamic>? _decode(http.Response response) {
    if (response.body.isEmpty) return null;
    try {
      final decoded = jsonDecode(utf8.decode(response.bodyBytes));
      return decoded is Map<String, dynamic> ? decoded : null;
    } catch (_) {
      return null;
    }
  }

  static String? _errorCode(Map<String, dynamic>? body) {
    final error = body?['error'];
    if (error is String) return error;
    if (error is Map) {
      final code = error['code'];
      if (code is String) return code;
    }
    return null;
  }

  void dispose() => _client.close();
}
