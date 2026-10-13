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

/// Borrador de 1 POST de nota (top o hija): score en escala backend
/// (10-70, null = pendiente), peso como fracción 0-1.
class GradeDraft {
  GradeDraft({
    required this.name,
    this.score,
    required this.weight,
    this.status = 'pending',
    this.isFinalExam = false,
    this.children = const [],
  });

  final String name;
  final int? score;
  final double weight;
  final String status;
  final bool isFinalExam;
  final List<GradeDraft> children;
}

/// Resultado del guardado de un árbol de notas: recrea todo (borra lo
/// existente y crea tops + hijas en fan-out). Fallos visibles por nombre;
/// reintentar es idempotente (primero borra lo logrado).
class GradeTreeResult {
  GradeTreeResult({
    required this.createdTops,
    required this.failedTops,
    required this.failedChildren,
    this.deleteError,
  });

  final List<String> createdTops;
  final Map<String, String> failedTops;
  final Map<String, String> failedChildren;
  final String? deleteError;

  bool get hasFailures =>
      failedTops.isNotEmpty ||
      failedChildren.isNotEmpty ||
      deleteError != null;
}

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

  // GET /subjects/:id/grades -> [{grade, children}].
  Future<List<AcademicGradeNode>> listGrades(String subjectId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/grades'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudieron cargar las notas.')
        .map(AcademicGradeNode.fromJson)
        .toList();
  }

  // POST /subjects/:id/grades {name, score?, weight, status?, ...} -> {id}.
  Future<String> createGrade({
    required String subjectId,
    required String name,
    int? score,
    required double weight,
    String? status,
    bool isFinalExam = false,
  }) async {
    final payload = <String, dynamic>{
      'name': name.trim(),
      'weight': weight,
      'status': status ?? (score != null ? 'graded' : 'pending'),
      if (isFinalExam) 'is_final_exam': true,
    };
    final sc = score;
    if (sc != null) payload['score'] = sc;
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/subjects/$subjectId/grades'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo crear la nota.');
  }

  // POST /grades/:id/children -> {id} (fan-out de subdivisión).
  Future<String> createGradeChild({
    required String parentGradeId,
    required String name,
    int? score,
    required double weight,
    String? status,
  }) async {
    final payload = <String, dynamic>{
      'name': name.trim(),
      'weight': weight,
      'status': status ?? (score != null ? 'graded' : 'pending'),
    };
    final sc = score;
    if (sc != null) payload['score'] = sc;
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/grades/$parentGradeId/children'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo crear la subnota.');
  }

  // PATCH /grades/:id (parcial).
  Future<void> patchGrade(String gradeId, Map<String, dynamic> patch) async {
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse('$_baseUrl/grades/$gradeId'),
            headers: _headers(json: true),
            body: jsonEncode(patch),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudo actualizar la nota.');
    }
  }

  // DELETE /grades/:id (las hijas caen por cascada).
  Future<void> deleteGrade(String gradeId) async {
    final res = await AuthedHttp.run(
      () => _client
          .delete(
            Uri.parse('$_baseUrl/grades/$gradeId'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 204) {
      throw _toError(res, 'No se pudo eliminar la nota.');
    }
  }

  /// Guardado completo del árbol: borra las notas actuales y recrea tops
  /// + hijas en fan-out. Idempotente ante reintento (borra primero).
  Future<GradeTreeResult> saveGradeTree({
    required String subjectId,
    required List<GradeDraft> tops,
  }) async {
    final created = <String>[];
    final failedTops = <String, String>{};
    final failedChildren = <String, String>{};

    List<AcademicGradeNode> current = [];
    try {
      current = await listGrades(subjectId);
    } catch (_) {
      // Si el GET falla, se intenta crear igual (los POST dirán).
    }
    for (final node in current) {
      try {
        await deleteGrade(node.grade.id);
      } on AcademicApiException catch (e) {
        return GradeTreeResult(
          createdTops: created,
          failedTops: failedTops,
          failedChildren: failedChildren,
          deleteError: e.message,
        );
      }
    }

    for (final top in tops) {
      String topId;
      try {
        topId = await createGrade(
          subjectId: subjectId,
          name: top.name,
          score: top.score,
          weight: top.weight,
          status: top.status,
          isFinalExam: top.isFinalExam,
        );
        created.add(topId);
      } on AcademicApiException catch (e) {
        failedTops[top.name] = e.message;
        continue;
      }
      for (var i = 0; i < top.children.length; i++) {
        final ch = top.children[i];
        try {
          await createGradeChild(
            parentGradeId: topId,
            name: ch.name,
            score: ch.score,
            weight: ch.weight,
            status: ch.status,
          );
        } on AcademicApiException catch (e) {
          failedChildren['${top.name} · hija ${i + 1}'] = e.message;
        }
      }
    }
    return GradeTreeResult(
      createdTops: created,
      failedTops: failedTops,
      failedChildren: failedChildren,
    );
  }

  // GET /subjects/:id/average -> {average, status}.
  Future<AcademicAverage> getAverage(String subjectId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/average'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudo calcular el promedio.');
    }
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    if (body is! Map) {
      throw AcademicApiException('Respuesta inesperada del servidor.');
    }
    return AcademicAverage.fromJson(Map<String, dynamic>.from(body));
  }

  // GET /subjects/:id/requirements (lectura).
  Future<List<AcademicRequirement>> listRequirements(String subjectId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/requirements'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudieron cargar los requisitos.')
        .map(AcademicRequirement.fromJson)
        .toList();
  }

  // GET /subjects/:id/absences -> faltas ordenadas por fecha.
  Future<List<AcademicAbsence>> listAbsences(String subjectId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/absences'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudieron cargar las inasistencias.')
        .map(AcademicAbsence.fromJson)
        .toList();
  }

  // POST /subjects/:id/absences {date, justified?, source?, type?} -> {id}.
  Future<String> createAbsence({
    required String subjectId,
    required String date,
    bool justified = false,
    String? source,
    String? type,
  }) async {
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/subjects/$subjectId/absences'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{
              'date': date,
              'justified': justified,
              if (source != null && source.trim().isNotEmpty)
                'source': source.trim(),
              if (type != null && type.trim().isNotEmpty) 'type': type.trim(),
            }),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo registrar la inasistencia.');
  }

  // PATCH /absences/:id (justified y/o source/type; "" limpia).
  Future<void> patchAbsence(String absenceId, Map<String, dynamic> patch) async {
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse('$_baseUrl/absences/$absenceId'),
            headers: _headers(json: true),
            body: jsonEncode(patch),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudo actualizar la inasistencia.');
    }
  }

  // DELETE /absences/:id.
  Future<void> deleteAbsence(String absenceId) async {
    final res = await AuthedHttp.run(
      () => _client
          .delete(
            Uri.parse('$_baseUrl/absences/$absenceId'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 204) {
      throw _toError(res, 'No se pudo eliminar la inasistencia.');
    }
  }

  // GET /subjects/:id/attendance -> resumen con la fórmula oficial.
  Future<AcademicAttendance> getAttendance(String subjectId) async {    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/attendance'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudo cargar la asistencia.');
    }
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    if (body is! Map) {
      throw AcademicApiException('Respuesta inesperada del servidor.');
    }
    return AcademicAttendance.fromJson(Map<String, dynamic>.from(body));
  }

  // GET /subjects/:id/contents -> unidades ordenadas por posición.
  Future<List<AcademicContent>> listContents(String subjectId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/subjects/$subjectId/contents'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _items(res, 'No se pudieron cargar los contenidos.')
        .map(AcademicContent.fromJson)
        .toList();
  }

  // POST /subjects/:id/contents {title, body?, position?} -> {id}.
  Future<String> createContent({
    required String subjectId,
    required String title,
    String? body,
    int position = 0,
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      'position': position,
    };
    final b = body;
    if (b != null && b.trim().isNotEmpty) payload['body'] = b.trim();
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/subjects/$subjectId/contents'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    return _createdId(res, 'No se pudo crear el contenido.');
  }

  // PATCH /contents/:id (title/body/position; body "" limpia).
  Future<void> patchContent(String contentId, Map<String, dynamic> patch) async {
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse('$_baseUrl/contents/$contentId'),
            headers: _headers(json: true),
            body: jsonEncode(patch),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudo actualizar el contenido.');
    }
  }

  // DELETE /contents/:id.
  Future<void> deleteContent(String contentId) async {
    final res = await AuthedHttp.run(
      () => _client
          .delete(
            Uri.parse('$_baseUrl/contents/$contentId'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 204) {
      throw _toError(res, 'No se pudo eliminar el contenido.');
    }
  }

  /// Fija pinned en una nota (el backend despinnea la anterior de la
  /// materia en la misma sentencia: un solo pin por materia).
  Future<void> pinGrade(String gradeId, bool pinned) {
    return patchGrade(gradeId, {'pinned': pinned});
  }

  /// Renombra una nota (PATCH existente).
  Future<void> renameGrade(String gradeId, String name) {
    return patchGrade(gradeId, {'name': name.trim()});
  }
}
