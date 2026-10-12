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
}
