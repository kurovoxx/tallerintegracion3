// Mapeos puros entre el front Academic y la API (back/infacadem).
// - Días: etiquetas Lun..Dom <-> 0..6 (Domingo = 0, igual que Postgres).
// - Horas: minutos-int <-> "HH:MM" (la API acepta "15:04"/"15:04:05").
// - Pesos: % 0-100 <-> fracción 0-1 (dividir por 100 al enviar).
// Sin dependencias de Flutter: cubierto por flutter test.

/// Etiqueta corta -> día Postgres (0 = Domingo .. 6 = Sábado).
/// Acepta abreviaturas Lun..Dom y nombres completos, con o sin tilde.
/// Retorna null si el día es inválido (error front antes de llamar).
int? dayLabelToNumber(String label) {
  final v = label.trim().toLowerCase().replaceAll('é', 'e').replaceAll('á', 'a');
  switch (v) {
    case 'dom':
    case 'domingo':
      return 0;
    case 'lun':
    case 'lunes':
      return 1;
    case 'mar':
    case 'martes':
      return 2;
    case 'mie':
    case 'miercoles':
      return 3;
    case 'jue':
    case 'jueves':
      return 4;
    case 'vie':
    case 'viernes':
      return 5;
    case 'sab':
    case 'sabado':
      return 6;
    default:
      return null;
  }
}

/// Día Postgres (0 = Domingo .. 6 = Sábado) -> etiqueta corta del front.
String dayNumberToLabel(int day) {
  switch (day) {
    case 0:
      return 'Dom';
    case 1:
      return 'Lun';
    case 2:
      return 'Mar';
    case 3:
      return 'Mié';
    case 4:
      return 'Jue';
    case 5:
      return 'Vie';
    case 6:
      return 'Sáb';
    default:
      return '—';
  }
}

/// Minutos desde medianoche -> "HH:MM".
String minutesToHhMm(int minutes) {
  final h = (minutes ~/ 60).toString().padLeft(2, '0');
  final m = (minutes % 60).toString().padLeft(2, '0');
  return '$h:$m';
}

/// "HH:MM" o "HH:MM:SS" (tal como responde la API) -> minutos.
/// Retorna null si el formato es inválido.
int? hhMmToMinutes(String raw) {
  final v = raw.trim();
  final m = RegExp(r'^(\d{1,2}):(\d{2})(?::(\d{2}))?$').firstMatch(v);
  if (m == null) return null;
  final h = int.tryParse(m.group(1)!);
  final min = int.tryParse(m.group(2)!);
  if (h == null || min == null || h > 23 || min > 59) return null;
  return h * 60 + min;
}

/// Bloque "10:00–11:30" (guion largo/corto) -> [inicio, fin] en "HH:MM".
/// Retorna null si el bloque es inválido o fin <= inicio.
List<String>? splitTimeBlock(String block) {
  final parts = block.split(RegExp(r'\s*[–—-]\s*'));
  if (parts.length != 2) return null;
  final start = parts[0].trim();
  final end = parts[1].trim();
  final sMin = hhMmToMinutes(start);
  final eMin = hhMmToMinutes(end);
  if (sMin == null || eMin == null || eMin <= sMin) return null;
  return <String>[minutesToHhMm(sMin), minutesToHhMm(eMin)];
}

/// Borrador de 1 POST /subjects/:id/schedules tras el fan-out.
class ScheduleDraft {
  ScheduleDraft({
    required this.dayOfWeek,
    required this.startTime,
    required this.endTime,
  });

  final int dayOfWeek;
  final String startTime;
  final String endTime;
}

/// Entrada del picker ("Lun / Mié · 10:00–11:30") -> N borradores
/// (un POST por día). Lanza [FormatException] con mensaje humano si la
/// entrada es inválida (día desconocido o bloque mal formado).
List<ScheduleDraft> expandScheduleEntry(String entry) {
  final sides = entry.split('·');
  if (sides.length != 2) {
    throw FormatException('Horario inválido: $entry');
  }
  final days = sides[0]
      .split('/')
      .map((d) => d.trim())
      .where((d) => d.isNotEmpty)
      .toList();
  if (days.isEmpty) throw FormatException('Sin días: $entry');
  final block = splitTimeBlock(sides[1].trim());
  if (block == null) throw FormatException('Bloque inválido: $entry');
  return days.map((d) {
    final n = dayLabelToNumber(d);
    if (n == null) throw FormatException('Día inválido: $d');
    return ScheduleDraft(dayOfWeek: n, startTime: block[0], endTime: block[1]);
  }).toList();
}

/// % front (0-100, acepta "25", "25%" y coma decimal "25,5") -> fracción
/// backend (dividir por 100 al enviar). null si inválido.
double? percentToFraction(dynamic raw) {
  if (raw == null) return null;
  if (raw is num) return raw.toDouble() / 100.0;
  var v = raw.toString().trim().replaceAll(',', '.');
  if (v.endsWith('%')) v = v.substring(0, v.length - 1).trim();
  if (v.isEmpty) return null;
  final n = double.tryParse(v);
  if (n == null) return null;
  return n / 100.0;
}

/// Fracción backend -> % front (para render).
double fractionToPercent(double fraction) => fraction * 100.0;

/// UX-01: el código de materia acepta cualquier texto no vacío (trim).
/// El backend lo guarda libre (sin formato ni unique).
bool isValidSubjectCode(String? raw) {
  return raw != null && raw.trim().isNotEmpty;
}

/// UX-03: etiqueta del combobox "CODE · Nombre" (CODE → '—' si vacío).
String subjectOptionLabel(String? code, String name) {
  final c = (code ?? '').trim();
  return '${c.isEmpty ? '—' : c} · $name';
}

/// UX-04: payload PATCH de config de asistencia. Omite nulos (el backend
/// no limpia total_classes/min_attendance_pct por PATCH, solo los setea).
Map<String, dynamic> attendanceConfigPatch({
  int? totalClasses,
  double? minAttendancePct,
}) {
  final out = <String, dynamic>{};
  final total = totalClasses;
  if (total != null) out['total_classes'] = total;
  final minPct = minAttendancePct;
  if (minPct != null) out['min_attendance_pct'] = minPct;
  return out;
}

double _roundHalfUp(double value, int decimals) {
  var factor = 1.0;
  for (var i = 0; i < decimals; i++) {
    factor *= 10;
  }
  return ((value * factor + 0.5).floorToDouble()) / factor;
}

/// Nota UI (1-7) -> score backend (10-70, half-up, clamp). null si inválida.
int? gradeUiToRaw(dynamic raw) {
  double? v;
  if (raw == null) return null;
  if (raw is num) {
    v = raw.toDouble();
  } else {
    v = double.tryParse(raw.toString().trim().replaceAll(',', '.'));
  }
  if (v == null) return null;
  return _roundHalfUp(v * 10, 0).round().clamp(10, 70);
}

/// Score backend (10-70) -> nota UI (1-7). null si inválido.
double? gradeRawToUi(dynamic raw) {
  int? n;
  if (raw == null) return null;
  if (raw is int) {
    n = raw;
  } else if (raw is num) {
    n = raw.round();
  } else {
    n = int.tryParse(raw.toString().trim());
  }
  if (n == null) return null;
  return n / 10.0;
}

/// Nota máxima válida en la UI (escala 1-7).
const double maxValidGrade = 7.0;

/// Nota mínima válida en la UI (escala 1-7).
const double minValidGrade = 1.0;

/// Una nota conocida es válida solo en [1.0, 7.0]; null/vacía = pendiente
/// (siempre OK). La escala 10-70 es solo del backend.
bool isValidGradeScore(double? score) {
  if (score == null) return true;
  return score >= minValidGrade && score <= maxValidGrade;
}

/// Todas las notas conocidas (tops, hijas, examen/final) en rango.
/// Una sola fuera de rango bloquea el cálculo.
bool allScoresInRange(Iterable<double?> scores) {
  return scores.every(isValidGradeScore);
}

/// Resultado sellado de [computeNeeded].
enum NeededStatus {
  /// La meta ya está alcanzada: nunca se muestra un needed <= 0 como nota.
  reached,

  /// Falta nota por obtener (1.0 <= needed <= 7.0).
  needed,

  /// Ni con 7.0 en todo lo pendiente se alcanza.
  unreachable,
}

class NeededResult {
  const NeededResult(this.status, [this.needed]);

  final NeededStatus status;

  /// Nota a obtener redondeada fuera (solo con status == needed).
  final double? needed;
}

/// Nota necesaria para alcanzar la meta con pendientes por rendir.
///
/// [weightedKnown] = Σ(nota × peso fracción) conocido; [pendingWeight] =
/// Σ de pesos fracción pendientes. Casos:
/// - `needed = (target - weightedKnown) / pendingWeight <= 0` → reached
///   (el promedio conocido ya cubre la meta; ej. conocidos 5.5, pendiente
///   1%, meta 4.0 → -150, que jamás debe mostrarse como nota).
/// - `needed == 0` → reached; `needed == 1.0` → needed (nota mínima válida,
///   se muestra por el flujo normal).
/// - `needed <= 7.0` → needed; `> 7.0` → unreachable.
/// - Sin pendientes (`pendingWeight <= 0`): reached si el promedio conocido
///   ya cubre la meta, si no unreachable (el llamador conserva su tarjeta
///   de promedio calculado).
NeededResult computeNeeded({
  required double target,
  required double weightedKnown,
  required double pendingWeight,
}) {
  if (pendingWeight <= 0) {
    return NeededResult(
      weightedKnown >= target ? NeededStatus.reached : NeededStatus.unreachable,
    );
  }
  final needed = (target - weightedKnown) / pendingWeight;
  if (needed <= 0) return const NeededResult(NeededStatus.reached);
  if (needed <= maxValidGrade) return NeededResult(NeededStatus.needed, needed);
  return const NeededResult(NeededStatus.unreachable);
}

/// UX-05b: un apunte pertenece a la materia si su título empieza con
/// "[CODE] " (insensible a caso). Convención de lado cliente: el modelo
/// de notas no tiene campo materia y los corchetes romperían la sintaxis
/// FTS si se filtrara por búsqueda.
bool noteMatchesSubjectCode(String title, String code) {
  final c = code.trim();
  if (c.isEmpty) return false;
  return title.trim().toUpperCase().startsWith('[${c.toUpperCase()}]');
}

/// Prefijo "[CODE] " para crear apunte con materia preseleccionada.
String subjectNotePrefix(String code) => '[${code.trim().toUpperCase()}] ';

/// Label de fila de calculadora: nombre real del backend si existe,
/// fallback "Nota N"/"Subnota N" para filas nuevas sin guardar.
String gradeRowLabel(String? backendName, String fallback) {
  final name = backendName?.trim();
  if (name == null || name.isEmpty) return fallback;
  return name;
}
