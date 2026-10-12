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
