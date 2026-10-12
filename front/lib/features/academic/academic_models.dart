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
    this.totalClasses,
    this.minAttendancePct,
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
  final int? totalClasses;
  final double? minAttendancePct;

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
      totalClasses: asInt(j['total_classes']),
      minAttendancePct: asDouble(j['min_attendance_pct']),
    );
  }

  static double? asDouble(dynamic v) {
    if (v == null) return null;
    if (v is num) return v.toDouble();
    return double.tryParse(v.toString().replaceAll(',', '.'));
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

// Nota con score en escala backend (10-70, null = pendiente) y peso como
// fracción 0-1. Ver academic_mappers para conversiones UI (1-7, %).
class AcademicGrade {
  AcademicGrade({
    required this.id,
    required this.subjectId,
    this.parentGradeId,
    this.isFinalExam = false,
    this.status = 'pending',
    required this.name,
    this.score,
    required this.weight,
    this.scheduledDate,
    this.topics,
  });

  final String id;
  final String subjectId;
  final String? parentGradeId;
  final bool isFinalExam;
  final String status;
  final String name;
  final int? score;
  final double weight;
  final String? scheduledDate;
  final String? topics;

  static int? _asScore(dynamic v) {
    if (v == null) return null;
    if (v is int) return v;
    return int.tryParse(v.toString());
  }

  static double _asWeight(dynamic v) {
    if (v == null) return 0;
    if (v is num) return v.toDouble();
    return double.tryParse(v.toString().replaceAll(',', '.')) ?? 0;
  }

  factory AcademicGrade.fromJson(Map<String, dynamic> j) {
    return AcademicGrade(
      id: (j['id'] as String?) ?? '',
      subjectId: (j['subject_id'] as String?) ?? '',
      parentGradeId: j['parent_grade_id'] as String?,
      isFinalExam: (j['is_final_exam'] as bool?) ?? false,
      status: (j['status'] as String?) ?? 'pending',
      name: (j['name'] as String?) ?? '',
      score: _asScore(j['score']),
      weight: _asWeight(j['weight']),
      scheduledDate: j['scheduled_date'] as String?,
      topics: j['topics'] as String?,
    );
  }
}

/// Nodo de GET /subjects/:id/grades: nota tope + hijas (un nivel).
class AcademicGradeNode {
  AcademicGradeNode({required this.grade, required this.children});

  final AcademicGrade grade;
  final List<AcademicGrade> children;

  factory AcademicGradeNode.fromJson(Map<String, dynamic> j) {
    final g = j['grade'];
    final ch = j['children'];
    return AcademicGradeNode(
      grade: AcademicGrade.fromJson(
        g is Map ? Map<String, dynamic>.from(g) : <String, dynamic>{},
      ),
      children: ch is List
          ? ch
                .whereType<Map>()
                .map((e) => AcademicGrade.fromJson(Map<String, dynamic>.from(e)))
                .toList()
          : const [],
    );
  }
}

/// GET /subjects/:id/average: promedio en escala 1-7 (o null sin notas) +
/// estado derivado (approved/failed/exam_required/pending).
class AcademicAverage {
  AcademicAverage({this.average, required this.status});

  final double? average;
  final String status;

  factory AcademicAverage.fromJson(Map<String, dynamic> j) {
    double? avg;
    final raw = j['average'];
    if (raw is num) {
      avg = raw.toDouble();
    } else if (raw is String) {
      avg = double.tryParse(raw.replaceAll(',', '.'));
    }
    return AcademicAverage(
      average: avg,
      status: (j['status'] as String?) ?? 'pending',
    );
  }
}

class AcademicRequirement {
  AcademicRequirement({
    required this.id,
    required this.subjectId,
    this.sourceGradeId,
    required this.checksFinalAverage,
    required this.comparator,
    required this.thresholdValue,
    required this.action,
  });

  final String id;
  final String subjectId;
  final String? sourceGradeId;
  final bool checksFinalAverage;
  final String comparator;
  final double thresholdValue;
  final String action;

  factory AcademicRequirement.fromJson(Map<String, dynamic> j) {
    double th = 0;
    final raw = j['threshold_value'];
    if (raw is num) {
      th = raw.toDouble();
    } else if (raw is String) {
      th = double.tryParse(raw.replaceAll(',', '.')) ?? 0;
    }
    return AcademicRequirement(
      id: (j['id'] as String?) ?? '',
      subjectId: (j['subject_id'] as String?) ?? '',
      sourceGradeId: j['source_grade_id'] as String?,
      checksFinalAverage: (j['checks_final_average'] as bool?) ?? false,
      comparator: (j['comparator'] as String?) ?? '>=',
      thresholdValue: th,
      action: (j['action'] as String?) ?? 'force_exam',
    );
  }
}

/// Inasistencia: fecha "YYYY-MM-DD", source (self/teacher) y type
/// (Teoría/Lab/...) nulables.
class AcademicAbsence {
  AcademicAbsence({
    required this.id,
    required this.subjectId,
    required this.date,
    required this.justified,
    this.source,
    this.type,
  });

  final String id;
  final String subjectId;
  final String date;
  final bool justified;
  final String? source;
  final String? type;

  static String? _opt(Map<String, dynamic> j, String key) {
    final v = j[key] as String?;
    if (v == null || v.trim().isEmpty) return null;
    return v;
  }

  factory AcademicAbsence.fromJson(Map<String, dynamic> j) {
    return AcademicAbsence(
      id: (j['id'] as String?) ?? '',
      subjectId: (j['subject_id'] as String?) ?? '',
      date: (j['date'] as String?) ?? '',
      justified: (j['justified'] as bool?) ?? false,
      source: _opt(j, 'source'),
      type: _opt(j, 'type'),
    );
  }

  /// "YYYY-MM-DD" -> DateTime (null si inválida).
  DateTime? get dateTime {
    try {
      final parts = date.split('-');
      if (parts.length != 3) return null;
      return DateTime(
        int.parse(parts[0]),
        int.parse(parts[1]),
        int.parse(parts[2]),
      );
    } catch (_) {
      return null;
    }
  }
}

/// GET /subjects/:id/attendance: fórmula oficial
/// % = 1 - injustificadas/total (null sin total).
class AcademicAttendance {
  AcademicAttendance({
    this.totalClasses,
    required this.unjustifiedCount,
    required this.justifiedCount,
    this.percentage,
    this.minAttendancePct,
    this.meetsMinimum,
    this.remaining,
  });

  final int? totalClasses;
  final int unjustifiedCount;
  final int justifiedCount;
  final double? percentage;
  final double? minAttendancePct;
  final bool? meetsMinimum;
  final int? remaining;

  static int? _asInt(dynamic v) {
    if (v == null) return null;
    if (v is int) return v;
    return int.tryParse(v.toString());
  }

  static double? _asDouble(dynamic v) {
    if (v == null) return null;
    if (v is num) return v.toDouble();
    return double.tryParse(v.toString().replaceAll(',', '.'));
  }

  factory AcademicAttendance.fromJson(Map<String, dynamic> j) {
    int asCount(dynamic v) {
      if (v is int) return v;
      return int.tryParse(v?.toString() ?? '') ?? 0;
    }

    return AcademicAttendance(
      totalClasses: _asInt(j['total_classes']),
      unjustifiedCount: asCount(j['unjustified_count']),
      justifiedCount: asCount(j['justified_count']),
      percentage: _asDouble(j['percentage']),
      minAttendancePct: _asDouble(j['min_attendance_pct']),
      meetsMinimum: j['meets_minimum'] as bool?,
      remaining: _asInt(j['remaining']),
    );
  }
}
