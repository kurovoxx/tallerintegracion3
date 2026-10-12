// Cliente HTTP para el backend Academic (back/infacadem).
// Patrón canónico del resto del front (ver core/services/social_service.dart):
// - `Authorization: Bearer` con el token de sesión; sin sesión lanza
//   AcademicApiException(no_session) antes de llamar.
// - Todo pasa por AuthedHttp.run: refresh único ante 401 y redirección a
//   login cuando la sesión muere (main.dart).
// - Base URL configurable con --dart-define=ACADEMIC_BASE_URL (por defecto
//   Pillán, igual que api-auth/api-notes/api-social).
import 'dart:convert';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:http/http.dart' as http;

import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';
import 'academic_mappers.dart';
import 'academic_models.dart';

const _pillanOrigin = 'https://ti3-brojas.dev.censei.cl';
const _academicBase = String.fromEnvironment(
  'ACADEMIC_BASE_URL',
  defaultValue: '$_pillanOrigin/api-academic',
);

String _resolveBase(String v) {
  if (!v.startsWith('/')) return v;
  if (kIsWeb) return '${Uri.base.origin}$v';
  return '$_pillanOrigin$v';
}

/// Resultado del fan-out de un bloque multi-día: un POST por día.
/// Si un POST falla, el error queda visible en [failed] (día + mensaje) y
/// lo ya creado en [created]: nunca estado a medias silencioso.
class FanOutResult {
  FanOutResult({
    required this.createdIds,
    required this.skippedDays,
    required this.failed,
  });

  final List<String> createdIds;
  final List<int> skippedDays;
  final Map<int, String> failed;

  bool get hasFailures => failed.isNotEmpty;
}

class AcademicApi {
  AcademicApi({http.Client? client, String? baseUrl})
    : _client = client ?? http.Client(),
      _baseUrl = baseUrl ?? _resolveBase(_academicBase);

  final http.Client _client;
  final String _baseUrl;

  static String get defaultBaseUrl => _resolveBase(_academicBase);

  void dispose() => _client.close();

  Map<String, String> _headers({bool json = false}) {
    final token = SessionManager.token;
    if (token == null || token.isEmpty) {
      throw AcademicApiException(
        'No hay sesión. Inicia sesión para ver tu malla.',
        code: 'no_session',
      );
    }
    return <String, String>{
      if (json) 'Content-Type': 'application/json',
      'Accept': 'application/json',
      'Authorization': 'Bearer $token',
    };
  }

  AcademicApiException _toError(http.Response res, String fallback) {
    String message = fallback;
    String? code;
    try {
      if (res.body.isNotEmpty) {
        final body = jsonDecode(utf8.decode(res.bodyBytes));
        if (body is Map) {
          final err = body['error'];
          if (err is Map) {
            code = err['code']?.toString();
            message = err['message']?.toString() ?? err.toString();
          } else if (err is String) {
            message = err;
          } else if (body['message'] is String) {
            message = body['message'] as String;
          }
        }
      }
    } catch (_) {}
    if (res.statusCode == 401) {
      message = 'No autorizado (401). Revisa tu sesión.';
      code ??= 'unauthorized';
    }
    if (res.statusCode == 403) {
      message = 'Acceso denegado (403).';
      code ??= 'forbidden';
    }
    if (res.statusCode == 404) {
      message = 'No encontrado (404).';
      code ??= 'not_found';
    }
    if (res.statusCode == 503) {
      message = 'Servicio no disponible (503). Intenta más tarde.';
      code ??= 'unavailable';
    }
    return AcademicApiException(
      message,
      statusCode: res.statusCode,
      code: code,
    );
  }

  List<Map<String, dynamic>> _items(http.Response res, String fallback) {
    if (res.statusCode != 200) throw _toError(res, fallback);
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    if (body is! Map || body['items'] is! List) {
      throw AcademicApiException('Respuesta inesperada del servidor.');
    }
    return (body['items'] as List)
        .whereType<Map>()
        .map((e) => Map<String, dynamic>.from(e))
        .toList();
  }

  Future<String> _createdId(http.Response res, String fallback) async {
    if (res.statusCode != 201) throw _toError(res, fallback);
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    final id = body is Map ? body['id']?.toString() : null;
    if (id == null || id.isEmpty) {
      throw AcademicApiException('Respuesta inesperada del servidor.');
    }
    return id;
  }

  // GET /curricula -> mallas propias.
  Future<List<AcademicCurriculum>> listCurricula() async {
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$_baseUrl/curricula'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudo cargar la malla.').map(AcademicCurriculum.fromJson).toList();
  }

  // POST /curricula {name} -> 201 {id}.
  Future<String> createCurriculum(String name) async {
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/curricula'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{'name': name.trim()}),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo crear la malla.');
  }

  /// Primera malla propia o la crea ([name]) si no hay ninguna.
  Future<AcademicCurriculum> ensureCurriculum([
    String name = 'Malla 2026',
  ]) async {
    final all = await listCurricula();
    if (all.isNotEmpty) return all.first;
    final id = await createCurriculum(name);
    return AcademicCurriculum(id: id, name: name);
  }

  // GET /curricula/:id/subjects -> materias de la malla.
  Future<List<AcademicSubject>> listSubjects(String curriculumId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/curricula/$curriculumId/subjects'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudieron cargar las materias.').map(AcademicSubject.fromJson).toList();
  }

  // POST /curricula/:id/subjects -> 201 {id}.
  Future<String> createSubject({
    required String curriculumId,
    required String name,
    String? code,
    String? professor,
    String? room,
    int? semester,
    int? credits,
    String status = 'in_progress',
  }) async {
    final payload = <String, dynamic>{
      'name': name.trim(),
      if (code != null && code.trim().isNotEmpty) 'code': code.trim(),
      if (professor != null && professor.trim().isNotEmpty)
        'professor': professor.trim(),
      if (room != null && room.trim().isNotEmpty) 'room': room.trim(),
      'status': status,
    };
    final sem = semester;
    if (sem != null) payload['semester'] = sem;
    final cred = credits;
    if (cred != null) payload['credits'] = cred;
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/curricula/$curriculumId/subjects'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo crear la materia.');
  }

  // PATCH /subjects/:id (parcial; "" limpia code/professor/room).
  Future<void> patchSubject(String subjectId, Map<String, dynamic> patch) async {
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse('$_baseUrl/subjects/$subjectId'),
            headers: _headers(json: true),
            body: jsonEncode(patch),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudo actualizar la materia.');
    }
  }

  // GET /subjects/:id/schedules -> horarios de la materia.
  Future<List<AcademicScheduleSlot>> listSchedules(String subjectId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/schedules'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudieron cargar los horarios.').map(AcademicScheduleSlot.fromJson).toList();
  }

  // POST /subjects/:id/schedules -> 201 {id}.
  Future<String> createSchedule({
    required String subjectId,
    required int dayOfWeek,
    required String startTime,
    required String endTime,
    String? modality,
  }) async {
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/subjects/$subjectId/schedules'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{
              'day_of_week': dayOfWeek,
              'start_time': startTime,
              'end_time': endTime,
              if (modality != null && modality.trim().isNotEmpty)
                'modality': modality.trim(),
            }),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo crear el horario.');
  }

  /// Normaliza "HH:MM[:SS]" a "HH:MM" para comparar horarios.
  String _normTime(String raw) {
    final m = hhMmToMinutes(raw);
    return m == null ? raw.trim() : minutesToHhMm(m);
  }

  /// Fan-out de un bloque multi-día: un POST por día (Lun->1 ... Dom->0).
  /// Con [skipExisting] (defecto true, para reintentos) primero hace
  /// GET y omite los días ya creados con igual inicio/fin, así reintentar
  /// no duplica lo logrado: verificar por GET antes de reintentar.
  Future<FanOutResult> createSchedulesFanOut({
    required String subjectId,
    required List<int> days,
    required String startTime,
    required String endTime,
    String? modality,
    bool skipExisting = true,
  }) async {
    final created = <String>[];
    final skipped = <int>[];
    final failed = <int, String>{};

    Set<String> existing = {};
    if (skipExisting) {
      try {
        final current = await listSchedules(subjectId);
        // La API responde "HH:MM:SS" y el draft usa "HH:MM": normalizar
        // ambos lados para que el reintento detecte lo ya creado.
        existing = current
            .map((s) => '${s.dayOfWeek}|${_normTime(s.startTime)}|${_normTime(s.endTime)}')
            .toSet();
      } catch (_) {
        // Si el GET falla, se intenta crear igual (los POST dirán).
      }
    }

    final wantStart = _normTime(startTime);
    final wantEnd = _normTime(endTime);
    for (final day in days) {
      if (existing.contains('$day|$wantStart|$wantEnd')) {
        skipped.add(day);
        continue;
      }
      try {
        created.add(
          await createSchedule(
            subjectId: subjectId,
            dayOfWeek: day,
            startTime: startTime,
            endTime: endTime,
            modality: modality,
          ),
        );
      } on AcademicApiException catch (e) {
        failed[day] = e.message;
      }
    }
    return FanOutResult(
      createdIds: created,
      skippedDays: skipped,
      failed: failed,
    );
  }
}
