import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';
import 'package:taller_integracion_front/core/services/account_deletion_service.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

import 'support/service_uri.dart';

// Contrato de Identity: back/auth/internal/handler/http/account_deletion_handler.go
// y back/auth/cmd/server/main.go (DELETE /auth/account, GET /auth/account-deletion).

String jwtFor(String userId, {String tag = 'a'}) {
  String enc(Map<String, dynamic> m) =>
      base64Url.encode(utf8.encode(jsonEncode(m))).replaceAll('=', '');
  final exp =
      DateTime.now().add(const Duration(hours: 1)).millisecondsSinceEpoch ~/
      1000;
  return '${enc({'alg': 'HS256'})}.${enc({'user_id': userId, 'exp': exp, 'tag': tag})}.firma';
}

http.Response json(int status, Object body) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

class _Recorder {
  final requests = <http.Request>[];
  int get deletes => requests.where((r) => r.method == 'DELETE').length;
  int get statusChecks => requests.where((r) => r.method == 'GET').length;
}

/// Servidor falso con respuestas por método. [onDelete]/[onStatus] reciben el
/// número de llamada (desde 1).
MockClient _fakeIdentity(
  _Recorder recorder, {
  required FutureOr<http.Response> Function(int call, http.Request r) onDelete,
  FutureOr<http.Response> Function(int call, http.Request r)? onStatus,
}) {
  var deletes = 0;
  var statuses = 0;
  return MockClient((request) async {
    recorder.requests.add(request);
    if (request.method == 'DELETE' &&
        request.url.servicePath == '/auth/account') {
      return onDelete(++deletes, request);
    }
    if (request.method == 'GET' &&
        request.url.servicePath == '/auth/account-deletion') {
      final handler = onStatus;
      if (handler == null) throw StateError('status no esperado');
      return handler(++statuses, request);
    }
    return http.Response('ruta no contemplada', 599);
  });
}

void main() {
  late List<String> purged;
  late String tokenA;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    AccountDeletionService.resetInFlightForTesting();
    AuthedHttp.refreshOverride = null;
    purged = [];
    tokenA = jwtFor('user-a');
    await SessionManager.saveSession(
      tokenA,
      {'id': 'user-a', 'email': 'a@example.test'},
      rememberMe: true,
      refreshToken: 'refresh-a',
    );
  });

  tearDown(() async {
    AuthedHttp.refreshOverride = null;
    await SessionManager.clear();
  });

  AccountDeletionService service(
    MockClient client, {
    Duration timeout = const Duration(seconds: 45),
  }) => AccountDeletionService(
    client: client,
    requestTimeout: timeout,
    purgeLocalData: (userId) async => purged.add(userId),
  );

  group('DELETE /auth/account', () {
    test(
      'envía DELETE autenticado, sin cuerpo, y 200 complete es definitivo',
      () async {
        final recorder = _Recorder();
        final svc = service(
          _fakeIdentity(
            recorder,
            onDelete: (_, r) => json(200, {'status': 'complete'}),
          ),
        );
        final result = await svc.requestDeletion();

        expect(result.state, AccountDeletionState.complete);
        expect(result.httpStatus, 200);
        expect(result.verifiedByStatus, isFalse);
        final request = recorder.requests.single;
        expect(request.method, 'DELETE');
        expect(request.url.servicePath, '/auth/account');
        expect(request.headers['Authorization'], 'Bearer $tokenA');
        // El user_id sale solo del JWT: no viaja ningún identificador.
        expect(request.body, isEmpty);
        expect(request.url.query, isEmpty);
        // Solicitar no limpia nada: eso ocurre en finalizeLocal.
        expect(SessionManager.isLoggedIn, isTrue);
        expect(purged, isEmpty);
      },
    );

    test('202 pending se informa como pendiente, no como éxito', () async {
      final recorder = _Recorder();
      final svc = service(
        _fakeIdentity(
          recorder,
          onDelete: (_, _) => json(202, {'status': 'pending'}),
        ),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.pending);
      expect(result.httpStatus, 202);
      expect(result.isTerminalSuccess, isFalse);
      expect(SessionManager.isLoggedIn, isTrue);
      expect(recorder.deletes, 1);
    });

    test(
      '401 con refresh rechazado: sesión expirada y nada borrado local',
      () async {
        AuthedHttp.refreshOverride = (_) async => RefreshResult.failure(
          'rechazado',
          kind: RefreshFailureKind.rejected,
        );
        final recorder = _Recorder();
        final svc = service(
          _fakeIdentity(
            recorder,
            onDelete: (_, _) => json(401, {
              'error': {'code': 'token_expired', 'message': 'expirado'},
            }),
          ),
        );
        final result = await svc.requestDeletion();
        expect(result.state, AccountDeletionState.unauthorized);
        expect(recorder.deletes, 1);
        expect(SessionManager.isLoggedIn, isFalse);
        expect(purged, isEmpty);
      },
    );

    test(
      '401 con refresh válido reintenta una vez con el token rotado',
      () async {
        final rotated = jwtFor('user-a', tag: 'rotated');
        AuthedHttp.refreshOverride = (_) async =>
            RefreshResult.success(accessToken: rotated, refreshToken: 'r2');
        final recorder = _Recorder();
        final svc = service(
          _fakeIdentity(
            recorder,
            onDelete: (call, r) => call == 1
                ? json(401, {
                    'error': {'code': 'token_expired', 'message': 'expirado'},
                  })
                : json(200, {'status': 'complete'}),
          ),
        );
        final result = await svc.requestDeletion();
        expect(result.state, AccountDeletionState.complete);
        expect(recorder.deletes, 2);
        expect(
          recorder.requests.last.headers['Authorization'],
          'Bearer $rotated',
        );
      },
    );

    test('404 account_not_found es terminal y no consulta estado', () async {
      final recorder = _Recorder();
      final svc = service(
        _fakeIdentity(
          recorder,
          onDelete: (_, _) => json(404, {'error': 'account_not_found'}),
        ),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.accountNotFound);
      expect(result.errorCode, 'account_not_found');
      expect(recorder.statusChecks, 0);
    });

    test('503 ambiguo se resuelve con el estado real (pending)', () async {
      final recorder = _Recorder();
      final svc = service(
        _fakeIdentity(
          recorder,
          onDelete: (_, _) =>
              json(503, {'error': 'account_deletion_unavailable'}),
          onStatus: (_, r) {
            expect(r.headers['Authorization'], 'Bearer $tokenA');
            return json(200, {'status': 'pending', 'updated_at': 'x'});
          },
        ),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.pending);
      expect(result.verifiedByStatus, isTrue);
      expect(result.httpStatus, 503);
      expect(recorder.deletes, 1);
      expect(recorder.statusChecks, 1);
    });

    test(
      '503 sin inicio confirmado permite reintentar (not_started)',
      () async {
        final recorder = _Recorder();
        final svc = service(
          _fakeIdentity(
            recorder,
            onDelete: (_, _) =>
                json(503, {'error': 'account_deletion_unavailable'}),
            onStatus: (_, _) => json(404, {'status': 'not_started'}),
          ),
        );
        final result = await svc.requestDeletion();
        expect(result.state, AccountDeletionState.notStarted);
        expect(result.verifiedByStatus, isTrue);
      },
    );

    for (final status in [403, 409, 500]) {
      test(
        '$status fuera de contrato no se toma como éxito ni se repite',
        () async {
          final recorder = _Recorder();
          final svc = service(
            _fakeIdentity(
              recorder,
              onDelete: (_, _) => json(status, {'error': 'x'}),
              onStatus: (_, _) => json(404, {'status': 'not_started'}),
            ),
          );
          final result = await svc.requestDeletion();
          expect(result.state, AccountDeletionState.notStarted);
          expect(result.httpStatus, status);
          expect(recorder.deletes, 1);
          expect(SessionManager.isLoggedIn, isTrue);
        },
      );
    }

    test(
      '500 y estado inconsultable quedan sin confirmar (sin reintento ciego)',
      () async {
        final recorder = _Recorder();
        final svc = service(
          _fakeIdentity(
            recorder,
            onDelete: (_, _) => json(500, {'error': 'boom'}),
            onStatus: (_, _) =>
                json(503, {'error': 'account_status_unavailable'}),
          ),
        );
        final result = await svc.requestDeletion();
        expect(result.state, AccountDeletionState.unknown);
        expect(result.httpStatus, 500);
        expect(recorder.deletes, 1);
      },
    );

    test('200 con cuerpo inesperado no se presenta como borrado', () async {
      final recorder = _Recorder();
      final svc = service(
        _fakeIdentity(
          recorder,
          onDelete: (_, _) => json(200, {'status': 'pending'}),
          onStatus: (_, _) => json(200, {'status': 'pending'}),
        ),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.pending);
      expect(result.verifiedByStatus, isTrue);
    });

    test('timeout consulta el estado en vez de repetir el DELETE', () async {
      final recorder = _Recorder();
      final never = Completer<http.Response>();
      final svc = service(
        _fakeIdentity(
          recorder,
          onDelete: (_, _) => never.future,
          onStatus: (_, _) => json(200, {'status': 'complete'}),
        ),
        timeout: const Duration(milliseconds: 50),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.complete);
      expect(result.verifiedByStatus, isTrue);
      expect(result.httpStatus, isNull);
      expect(recorder.deletes, 1);
    });

    test('error de red y estado también caído: unknown', () async {
      final svc = AccountDeletionService(
        client: MockClient((_) async => throw http.ClientException('offline')),
        purgeLocalData: (u) async => purged.add(u),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.unknown);
      expect(purged, isEmpty);
      expect(SessionManager.isLoggedIn, isTrue);
    });

    test('pulsaciones repetidas comparten un único DELETE en vuelo', () async {
      final recorder = _Recorder();
      final gate = Completer<http.Response>();
      final client = _fakeIdentity(recorder, onDelete: (_, _) => gate.future);
      final first = service(client).requestDeletion();
      final second = service(client).requestDeletion();
      final third = service(client).requestDeletion();
      expect(identical(first, second), isTrue);
      expect(identical(first, third), isTrue);
      await Future<void>.delayed(Duration.zero);
      gate.complete(json(200, {'status': 'complete'}));
      final results = await Future.wait([first, second, third]);
      expect(results.map((r) => r.state).toSet(), {
        AccountDeletionState.complete,
      });
      expect(recorder.deletes, 1);
    });

    test('sin sesión no se envía nada', () async {
      await SessionManager.clear();
      final recorder = _Recorder();
      final svc = service(
        _fakeIdentity(recorder, onDelete: (_, _) => json(200, {})),
      );
      final result = await svc.requestDeletion();
      expect(result.state, AccountDeletionState.unauthorized);
      expect(recorder.requests, isEmpty);
    });

    test(
      'si la sesión cambia en vuelo, el resultado no se atribuye a la nueva',
      () async {
        final recorder = _Recorder();
        final gate = Completer<http.Response>();
        final svc = service(
          _fakeIdentity(recorder, onDelete: (_, _) => gate.future),
        );
        final pending = svc.requestDeletion();
        await Future<void>.delayed(Duration.zero);
        await SessionManager.clear();
        await SessionManager.saveSession(jwtFor('user-b'), {'id': 'user-b'});
        gate.complete(json(200, {'status': 'complete'}));
        final result = await pending;
        expect(result.state, AccountDeletionState.sessionChanged);
        expect(SessionManager.currentUserId, 'user-b');
      },
    );
  });

  group('GET /auth/account-deletion', () {
    Future<AccountDeletionResult> check(http.Response response) {
      final recorder = _Recorder();
      return service(
        _fakeIdentity(
          recorder,
          onDelete: (_, _) => throw StateError('no DELETE'),
          onStatus: (_, _) => response,
        ),
      ).checkStatus();
    }

    test('mapea complete, pending, not_started y fallos', () async {
      expect(
        (await check(json(200, {'status': 'complete'}))).state,
        AccountDeletionState.complete,
      );
      expect(
        (await check(json(200, {'status': 'pending'}))).state,
        AccountDeletionState.pending,
      );
      expect(
        (await check(json(404, {'status': 'not_started'}))).state,
        AccountDeletionState.notStarted,
      );
      expect(
        (await check(json(503, {'error': 'account_status_unavailable'}))).state,
        AccountDeletionState.unknown,
      );
      // Un 404 genérico (p. ej. ruta inexistente en un Identity antiguo) no
      // prueba que el borrado no haya empezado.
      expect(
        (await check(http.Response('404 page not found', 404))).state,
        AccountDeletionState.unknown,
      );
    });
  });

  group('limpieza local tras confirmar', () {
    test(
      'cierra solo la sesión borrada y olvida su correo recordado',
      () async {
        SharedPreferences.setMockInitialValues({
          SessionManager.rememberedEmailKey: 'A@example.test',
        });
        await SessionManager.saveSession(
          jwtFor('user-a'),
          {'id': 'user-a'},
          rememberMe: true,
          refreshToken: 'refresh-a',
        );
        final svc = service(MockClient((_) async => http.Response('', 599)));
        final report = await svc.finalizeLocal(
          userId: 'user-a',
          email: 'a@example.test',
        );
        expect(report.sessionCleared, isTrue);
        expect(report.localDataPurged, isTrue);
        expect(report.rememberedEmailRemoved, isTrue);
        expect(purged, ['user-a']);
        expect(SessionManager.isLoggedIn, isFalse);
        expect(SessionManager.refreshToken, isNull);
        final prefs = await SharedPreferences.getInstance();
        expect(prefs.getString('auth_tokens_v1'), isNull);
        expect(prefs.getString('auth_token'), isNull);
        expect(prefs.getString('refresh_token'), isNull);
        expect(prefs.getString('auth_user'), isNull);
        expect(prefs.getBool('auth_remember_me'), isNull);
        expect(prefs.getString(SessionManager.rememberedEmailKey), isNull);
        // Recordarme ya no restaura nada.
        expect(await SessionManager.hasPersistedSession(), isFalse);
      },
    );

    test('no toca la sesión ni el correo recordado de otra cuenta', () async {
      SharedPreferences.setMockInitialValues({
        SessionManager.rememberedEmailKey: 'b@example.test',
      });
      await SessionManager.saveSession(
        jwtFor('user-b'),
        {'id': 'user-b'},
        rememberMe: true,
        refreshToken: 'refresh-b',
      );
      final svc = service(MockClient((_) async => http.Response('', 599)));
      final report = await svc.finalizeLocal(
        userId: 'user-a',
        email: 'a@example.test',
      );
      expect(report.sessionCleared, isFalse);
      expect(report.rememberedEmailRemoved, isFalse);
      expect(SessionManager.currentUserId, 'user-b');
      final prefs = await SharedPreferences.getInstance();
      expect(
        prefs.getString(SessionManager.rememberedEmailKey),
        'b@example.test',
      );
      expect(purged, ['user-a']);
    });

    test('un fallo de la purga no deja la sesión borrada abierta', () async {
      final svc = AccountDeletionService(
        client: MockClient((_) async => http.Response('', 599)),
        purgeLocalData: (_) async => throw StateError('disco lleno'),
      );
      final report = await svc.finalizeLocal(userId: 'user-a');
      expect(report.sessionCleared, isTrue);
      expect(report.localDataPurged, isFalse);
      expect(SessionManager.isLoggedIn, isFalse);
    });

    test(
      'la purga SQLite borra solo notas, adjuntos y tombstones del dueño',
      () async {
        final db = AppDatabase.forTesting(NativeDatabase.memory());
        addTearDown(db.close);
        final repo = LocalNotesRepository(db);
        LocalNote note(String id, String? owner) => LocalNote(
          needsSync: owner == 'user-a',
          id: id,
          title: 'Nota $id',
          content: 'contenido $id',
          visibility: 'private',
          updatedAt: DateTime.now(),
          ownerUserId: owner,
        );
        await repo.upsertNote(note('a1', 'user-a'));
        await repo.upsertNote(note('a2', 'user-a'));
        await repo.upsertNote(note('b1', 'user-b'));
        await repo.upsertNote(note('legacy', null));
        for (final (placeholder, noteId, owner) in [
          ('pa', 'a1', 'user-a'),
          ('pb', 'b1', 'user-b'),
        ]) {
          await repo.savePendingAttachment(
            placeholder: placeholder,
            noteId: noteId,
            ownerId: owner,
            fileName: '$placeholder.png',
            mime: 'image/png',
            bytes: Uint8List.fromList([1, 2, 3]),
          );
        }
        await repo.addTombstone(id: 'ta', ownerUserId: 'user-a');
        await repo.addTombstone(id: 'tb', ownerUserId: 'user-b');

        await purgeLocalAccountDataIn(db, 'user-a');

        expect(await repo.getAllNotes(ownerId: 'user-a'), isEmpty);
        expect((await repo.getAllNotes(ownerId: 'user-b')).map((n) => n.id), [
          'b1',
        ]);
        expect((await repo.getAllNotes()).map((n) => n.id), ['legacy']);
        expect(
          await repo.searchNotesFts('contenido', ownerId: 'user-a'),
          isEmpty,
        );
        expect(
          (await repo.searchNotesFts(
            'contenido',
            ownerId: 'user-b',
          )).map((n) => n.id),
          ['b1'],
        );
        expect(await repo.pendingAttachmentsForNote('a1'), isEmpty);
        expect(
          (await repo.pendingAttachmentsForNote(
            'b1',
          )).map((p) => p.placeholder),
          ['pb'],
        );
        expect(await repo.getTombstones(ownerId: 'user-a'), isEmpty);
        expect((await repo.getTombstones(ownerId: 'user-b')).map((t) => t.id), [
          'tb',
        ]);
      },
    );
  });
}
