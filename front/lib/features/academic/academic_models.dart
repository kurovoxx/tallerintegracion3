// Modelos para el backend Academic (back/infacadem).
// Contratos verificados en:
// - back/infacadem/cmd/server/main.go (rutas /curricula, /subjects, /schedules)
// - back/infacadem/internal/model/{curriculum,subject,schedule}.go (tags json)
// - back/infacadem/internal/handler/http/present.go (horas "15:04:05")
class AcademicCurriculum {
  AcademicCurriculum({required this.id, required this.name});

  final String id;
  final String name;

  factory AcademicCurriculum.fromJson(Map<String, dynamic> j) {
    return AcademicCurriculum(
      id: (j['id'] as String?) ?? '',
      name: (j['name'] as String?) ?? '',
    );
  }
}

class AcademicSubject {
  AcademicSubject({
    required this.id,
    required this.curriculumId,
    required this.name,
    this.code,
    this.professor,
    this.room,
    this.semester,
    this.credits,
    this.status = 'in_progress',
  });

  final String id;
  final String curriculumId;
  final String name;
  final String? code;
  final String? professor;
  final String? room;
  final int? semester;
  final int? credits;
  final String status;

  static String? _opt(Map<String, dynamic> j, String key) {
    final v = j[key] as String?;
    if (v == null || v.trim().isEmpty) return null;
    return v;
  }

  factory AcademicSubject.fromJson(Map<String, dynamic> j) {
    int? asInt(dynamic v) {
      if (v == null) return null;
      if (v is int) return v;
      return int.tryParse(v.toString());
    }

    return AcademicSubject(
      id: (j['id'] as String?) ?? '',
      curriculumId: (j['curriculum_id'] as String?) ?? '',
      name: (j['name'] as String?) ?? '',
      code: _opt(j, 'code'),
      professor: _opt(j, 'professor'),
      room: _opt(j, 'room'),
      semester: asInt(j['semester']),
      credits: asInt(j['credits']),
      status: (j['status'] as String?) ?? 'in_progress',
    );
  }

  /// JSON para POST /curricula/:id/subjects y PATCH /subjects/:id.
  /// null = omitido (el backend distingue omitido vs "" para limpiar).
  Map<String, dynamic> toCreateJson() => <String, dynamic>{
    'name': name,
    if (code != null) 'code': code,
    if (professor != null) 'professor': professor,
    if (room != null) 'room': room,
    if (semester != null) 'semester': semester,
    if (credits != null) 'credits': credits,
    'status': status,
  };
}

class AcademicScheduleSlot {
  AcademicScheduleSlot({
    required this.id,
    required this.subjectId,
    required this.dayOfWeek,
    required this.startTime,
    required this.endTime,
    this.modality,
  });

  final String id;
  final String subjectId;

  /// 0 = Domingo .. 6 = Sábado (igual que Postgres y el front).
  final int dayOfWeek;

  /// Horas tal como las entrega la API ("15:04:05" o "15:04").
  final String startTime;
  final String endTime;
  final String? modality;

  factory AcademicScheduleSlot.fromJson(Map<String, dynamic> j) {
    int day = 0;
    final raw = j['day_of_week'];
    if (raw is int) {
      day = raw;
    } else {
      day = int.tryParse(raw?.toString() ?? '') ?? 0;
    }
    return AcademicScheduleSlot(
      id: (j['id'] as String?) ?? '',
      subjectId: (j['subject_id'] as String?) ?? '',
      dayOfWeek: day,
      startTime: (j['start_time'] as String?) ?? '',
      endTime: (j['end_time'] as String?) ?? '',
      modality: _opt(j, 'modality'),
    );
  }

  static String? _opt(Map<String, dynamic> j, String key) {
    final v = j[key] as String?;
    if (v == null || v.trim().isEmpty) return null;
    return v;
  }
}

class AcademicApiException implements Exception {
  AcademicApiException(this.message, {this.statusCode, this.code});

  final String message;
  final int? statusCode;
  final String? code;

  @override
  String toString() {
    final c = code ?? 'error';
    final s = statusCode?.toString() ?? '?';
    return '[$c:$s] $message';
  }
}
