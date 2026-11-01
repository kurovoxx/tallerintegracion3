import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/academic/academic_mappers.dart';

void main() {
  test('días Lun..Dom <-> 0..6 (Domingo = 0)', () {
    expect(dayLabelToNumber('Lun'), 1);
    expect(dayLabelToNumber('Mar'), 2);
    expect(dayLabelToNumber('Mié'), 3);
    expect(dayLabelToNumber('Mie'), 3);
    expect(dayLabelToNumber('Jue'), 4);
    expect(dayLabelToNumber('Vie'), 5);
    expect(dayLabelToNumber('Sáb'), 6);
    expect(dayLabelToNumber('Dom'), 0);
    expect(dayLabelToNumber('Domingo'), 0);
    expect(dayLabelToNumber('Lunes'), 1);
    expect(dayLabelToNumber('Xyz'), isNull);
    expect(dayLabelToNumber(''), isNull);
    for (var d = 0; d <= 6; d++) {
      expect(dayLabelToNumber(dayNumberToLabel(d)), d);
    }
  });

  test('minutos-int <-> "HH:MM" (incluye "HH:MM:SS" de la API)', () {
    expect(minutesToHhMm(600), '10:00');
    expect(minutesToHhMm(690), '11:30');
    expect(hhMmToMinutes('10:00'), 600);
    expect(hhMmToMinutes('10:00:00'), 600);
    expect(hhMmToMinutes('08:15'), 495);
    expect(hhMmToMinutes('ayer'), isNull);
    expect(hhMmToMinutes('25:00'), isNull);
  });

  test('split "HH:MM–HH:MM" con guion largo/corto', () {
    expect(splitTimeBlock('10:00–11:30'), ['10:00', '11:30']);
    expect(splitTimeBlock('10:00-11:30'), ['10:00', '11:30']);
    expect(splitTimeBlock('10:00 — 11:30'), ['10:00', '11:30']);
    expect(splitTimeBlock('11:30–10:00'), isNull);
    expect(splitTimeBlock('10:00–10:00'), isNull);
    expect(splitTimeBlock('10:00'), isNull);
    expect(splitTimeBlock('x–y'), isNull);
  });

  test('bloque multi-día -> N borradores (fan-out)', () {
    final drafts = expandScheduleEntry('Lun / Mié · 10:00–11:30');
    expect(drafts, hasLength(2));
    expect(drafts[0].dayOfWeek, 1);
    expect(drafts[1].dayOfWeek, 3);
    expect(drafts[0].startTime, '10:00');
    expect(drafts[0].endTime, '11:30');

    final single = expandScheduleEntry('Vie · 14:00–17:00');
    expect(single, hasLength(1));
    expect(single.single.dayOfWeek, 5);

    expect(() => expandScheduleEntry('Xyz · 10:00–11:30'), throwsFormatException);
    expect(() => expandScheduleEntry('Lun · 11:30–10:00'), throwsFormatException);
    expect(() => expandScheduleEntry('sin separador'), throwsFormatException);
  });

  test('%/100 en ambas direcciones (coma decimal incluida)', () {
    expect(percentToFraction(25), 0.25);
    expect(percentToFraction(100), 1.0);
    expect(percentToFraction('25'), 0.25);
    expect(percentToFraction('25%'), 0.25);
    expect(percentToFraction('25,5%'), closeTo(0.255, 1e-9));
    expect(percentToFraction('0,5'), closeTo(0.005, 1e-9));
    expect(percentToFraction('x'), isNull);
    expect(percentToFraction(null), isNull);
    expect(fractionToPercent(0.25), 25.0);
  });

  test('nota UI (1-7) <-> score backend (10-70)', () {
    expect(gradeUiToRaw(6.2), 62);
    expect(gradeUiToRaw('4,0'), 40);
    expect(gradeUiToRaw(7), 70);
    expect(gradeUiToRaw(1), 10);
    expect(gradeUiToRaw('x'), isNull);
    expect(gradeUiToRaw(null), isNull);
    expect(gradeRawToUi(62), 6.2);
    expect(gradeRawToUi('40'), 4.0);
    expect(gradeRawToUi(null), isNull);
    expect(gradeRawToUi('x'), isNull);
  });

  test('computeNeeded: reporte -150 -> ya alcanzada, nunca negativo', () {
    // Reporte: conocidos 5.5, pendiente 1%, meta 4.0 -> (4-5.5)/0.01 = -150.
    final res = computeNeeded(
      target: 4.0,
      weightedKnown: 5.5,
      pendingWeight: 0.01,
    );
    expect(res.status, NeededStatus.reached);
    expect(res.needed, isNull);
  });

  test('computeNeeded: bordes 0/1.0/7.0/7.1 y sin pendientes', () {
    // needed exactamente 0 -> ya alcanzada.
    var res = computeNeeded(
      target: 4.0,
      weightedKnown: 4.0,
      pendingWeight: 0.5,
    );
    expect(res.status, NeededStatus.reached);

    // needed 1.0 -> flujo normal (nota mínima válida, se muestra).
    res = computeNeeded(
      target: 4.0,
      weightedKnown: 3.5,
      pendingWeight: 0.5,
    );
    expect(res.status, NeededStatus.needed);
    expect(res.needed, closeTo(1.0, 1e-9));

    // needed 4.5 normal.
    res = computeNeeded(
      target: 4.0,
      weightedKnown: 1.75,
      pendingWeight: 0.5,
    );
    expect(res.status, NeededStatus.needed);
    expect(res.needed, closeTo(4.5, 1e-9));

    // needed 7.0 -> alcanzable; 7.1 -> no alcanzable.
    res = computeNeeded(
      target: 4.0,
      weightedKnown: 1.2,
      pendingWeight: 0.4,
    );
    expect(res.status, NeededStatus.needed);
    expect(res.needed, closeTo(7.0, 1e-9));
    res = computeNeeded(
      target: 4.0,
      weightedKnown: 1.16,
      pendingWeight: 0.4,
    );
    expect(res.status, NeededStatus.unreachable);

    // needed 7.5 inalcanzable.
    res = computeNeeded(target: 4.0, weightedKnown: 1.0, pendingWeight: 0.4);
    expect(res.status, NeededStatus.unreachable);
    expect(res.needed, isNull);

    // Sin pendientes: intacto según promedio conocido.
    res = computeNeeded(target: 4.0, weightedKnown: 5.0, pendingWeight: 0);
    expect(res.status, NeededStatus.reached);
    res = computeNeeded(target: 4.0, weightedKnown: 3.0, pendingWeight: 0);
    expect(res.status, NeededStatus.unreachable);
  });

  test('validador de rango 1.0-7.0: reporte 70/50 y bordes', () {
    // Reporte: 70 y 50 (escala backend) bloquean el cálculo.
    expect(allScoresInRange([70.0, 50.0]), isFalse);
    expect(isValidGradeScore(70.0), isFalse);
    expect(isValidGradeScore(50.0), isFalse);

    // Bordes: 0.9 y 7.1 fuera; 1.0 y 7.0 dentro.
    expect(allScoresInRange([0.9]), isFalse);
    expect(allScoresInRange([7.1]), isFalse);
    expect(allScoresInRange([1.0, 7.0]), isTrue);

    // Vacía (pendiente) no dispara el error, ni mezclada con válidas.
    expect(allScoresInRange([null]), isTrue);
    expect(allScoresInRange([4.5, null, 6.0]), isTrue);

    // Hija inválida con resto válido bloquea igual.
    expect(allScoresInRange([5.0, 4.0, 80.0]), isFalse);
    expect(allScoresInRange([]), isTrue);
  });

  test('UX-01: código acepta cualquier texto no vacío', () {
    expect(isValidSubjectCode('INF-301'), isTrue);
    expect(isValidSubjectCode('inf 101'), isTrue);
    expect(isValidSubjectCode('MAT101'), isTrue);
    expect(isValidSubjectCode('un código muy largo con espacios'), isTrue);
    expect(isValidSubjectCode('  '), isFalse);
    expect(isValidSubjectCode(''), isFalse);
    expect(isValidSubjectCode(null), isFalse);
  });

  test('UX-02: bloque manual válido/inválido', () {
    // 10:20/11:50 válido crea el bloque.
    expect(splitTimeBlock('10:20–11:50'), ['10:20', '11:50']);
    expect(splitTimeBlock('10:20-11:50'), ['10:20', '11:50']);
    // Hora mal formada o fuera de rango.
    expect(hhMmToMinutes('9:5'), isNull);
    expect(hhMmToMinutes('24:00'), isNull);
    expect(splitTimeBlock('25:00–26:00'), isNull);
    // Fin <= inicio.
    expect(splitTimeBlock('11:50–11:50'), isNull);
    expect(splitTimeBlock('12:00–11:50'), isNull);
  });

  test('UX-03: etiqueta de opción CODE · Nombre', () {
    expect(subjectOptionLabel('INF-301', 'Redes'), 'INF-301 · Redes');
    expect(subjectOptionLabel(null, 'Redes'), '— · Redes');
    expect(subjectOptionLabel('  ', 'Redes'), '— · Redes');
  });

  test('UX-04: payload PATCH de config omite nulos', () {
    expect(
      attendanceConfigPatch(totalClasses: 32, minAttendancePct: 75.0),
      {'total_classes': 32, 'min_attendance_pct': 75.0},
    );
    expect(attendanceConfigPatch(), isEmpty);
    expect(
      attendanceConfigPatch(minAttendancePct: 70.5),
      {'min_attendance_pct': 70.5},
    );
  });

  test('UX-05b: matcher de apunte por materia + prefijo + done', () {
    expect(noteMatchesSubjectCode('[INF-301] Repaso', 'INF-301'), isTrue);
    expect(noteMatchesSubjectCode('[inf-301] repaso', 'INF-301'), isTrue);
    expect(noteMatchesSubjectCode('Repaso general', 'INF-301'), isFalse);
    expect(noteMatchesSubjectCode('[INF-302] Otro', 'INF-301'), isFalse);
    expect(noteMatchesSubjectCode('[INF-301] X', ''), isFalse);
    expect(subjectNotePrefix('inf-301'), '[INF-301] ');
    expect(subjectNotePrefix('  INF-301  '), '[INF-301] ');
  });

  test('labels de calculadora: nombre backend o fallback Nota N', () {
    expect(gradeRowLabel('Parcial', 'NOTA 1'), 'Parcial');
    expect(gradeRowLabel('  Certamen 2  ', 'NOTA 2'), 'Certamen 2');
    expect(gradeRowLabel(null, 'NOTA 1'), 'NOTA 1');
    expect(gradeRowLabel('', 'SUBNOTA 1'), 'SUBNOTA 1');
    expect(gradeRowLabel('   ', 'SUBNOTA 2'), 'SUBNOTA 2');
  });
}
