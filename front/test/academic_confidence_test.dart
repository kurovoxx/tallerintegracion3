import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/academic/academic_mappers.dart';

void main() {
  test('reporte tipo: Nota 2 vacía conf 3 y resto 1 -> Nota 2 > base', () {
    // target 4.0, Nota1 conocida 4.0 (w 0.4), Nota2 pendiente conf 3 (w 0.3),
    // Nota3 pendiente conf 1 (w 0.3).
    // known = 1.6, pending = 0.6, base = (4-1.6)/0.6 = 4.0,
    // confProm = (3*0.3+1*0.3)/0.6 = 2.0.
    // Tentativos: Nota2 = 11 -> 7.0, Nota3 = -3 -> 1.0, total 4.0.
    final res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 4.0, weight: 0.4, confidence: 1),
        ConfidenceEntry(score: null, weight: 0.3, confidence: 3),
        ConfidenceEntry(score: null, weight: 0.3, confidence: 1),
      ],
    );
    expect(res.reached, isTrue);
    expect(res.assigned[0], 4.0);
    expect(res.assigned[1], closeTo(7.0, 1e-9));
    expect(res.assigned[2], closeTo(1.0, 1e-9));
    // Coherencia: promedio con asignados alcanza la meta.
    final total = res.assigned[0]! * 0.4 +
        res.assigned[1]! * 0.3 +
        res.assigned[2]! * 0.3;
    expect(total, closeTo(4.0, 1e-9));
    // Nota 2 > base (4.0).
    expect(res.assigned[1]!, greaterThan(4.0));
  });

  test('confianzas iguales -> idéntico a mínimos iguales', () {
    // target 4.0, conocido 2.0 (4.0*0.5), pendientes 0.25+0.25 conf 2.
    // base = (4-2)/0.5 = 4.0, ambos 4.0 = computeNeeded.
    final dist = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 4.0, weight: 0.5, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.25, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.25, confidence: 2),
      ],
    );
    final need = computeNeeded(
      target: 4.0,
      weightedKnown: 2.0,
      pendingWeight: 0.5,
    );
    expect(need.status, NeededStatus.needed);
    expect(need.needed, closeTo(4.0, 1e-9));
    expect(dist.reached, isTrue);
    expect(dist.assigned[1], closeTo(need.needed!, 1e-9));
    expect(dist.assigned[2], closeTo(need.needed!, 1e-9));
  });

  test('sin pendientes o peso pendiente 0 -> sin reparto', () {
    var res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 5.0, weight: 0.6, confidence: 3),
        ConfidenceEntry(score: 4.0, weight: 0.4, confidence: 1),
      ],
    );
    expect(res.assigned, [5.0, 4.0]);
    expect(res.reached, isTrue);

    res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 3.0, weight: 1.0, confidence: 2),
      ],
    );
    expect(res.reached, isFalse);

    // Pendiente con peso 0: se ignora, queda null.
    res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 4.0, weight: 1.0, confidence: 2),
        ConfidenceEntry(score: null, weight: 0, confidence: 3),
      ],
    );
    expect(res.assigned[1], isNull);
    expect(res.reached, isTrue);
  });

  test('clamp alto 8.2+ -> 7.0 + recálculo del resto', () {
    // target 5.0, conocido 2.0 (4.0*0.5), pendientes A conf2 / B conf3 (0.25 c/u).
    // base libre = 6.0, confProm = 2.5, A = 2.5, B = 9.5 -> B a 7.0,
    // recálculo A: restante 1.25 / 0.25 = 5.0.
    final res = distributeNeeded(
      target: 5.0,
      entries: const [
        ConfidenceEntry(score: 4.0, weight: 0.5, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.25, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.25, confidence: 3),
      ],
    );
    expect(res.reached, isTrue);
    expect(res.assigned[2], closeTo(7.0, 1e-9));
    expect(res.assigned[1], closeTo(5.0, 1e-9));
    final total =
        2.0 + res.assigned[1]! * 0.25 + res.assigned[2]! * 0.25;
    expect(total, closeTo(5.0, 1e-9));
  });

  test('clamp bajo 0.4- -> 1.0 + recálculo del resto', () {
    // target 4.0, conocido 3.0 (6.0*0.5), pendientes A conf1 / B conf2.
    // base libre = 2.0, confProm = 1.5, A = -1.5 -> 1.0,
    // recálculo B: restante 0.75 / 0.25 = 3.0.
    final res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 6.0, weight: 0.5, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.25, confidence: 1),
        ConfidenceEntry(score: null, weight: 0.25, confidence: 2),
      ],
    );
    expect(res.reached, isTrue);
    expect(res.assigned[1], closeTo(1.0, 1e-9));
    expect(res.assigned[2], closeTo(3.0, 1e-9));
    final total =
        3.0 + res.assigned[1]! * 0.25 + res.assigned[2]! * 0.25;
    expect(total, closeTo(4.0, 1e-9));
  });

  test('meta ya alcanzada con pendientes -> 1.0 + reached', () {
    // Conocido 5.0*0.8 = 4.0 >= 4.0, pendiente 0.2 -> 1.0, total 4.2.
    final res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: 5.0, weight: 0.8, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.2, confidence: 3),
      ],
    );
    expect(res.reached, isTrue);
    expect(res.assigned[1], closeTo(1.0, 1e-9));
  });

  test('inalcanzable ni con 7.0 -> 7.0 + reached false', () {
    // target 6.5, conocido 1.0 (2.0*0.5), pendiente 0.5, base 11 -> 7.0.
    final res = distributeNeeded(
      target: 6.5,
      entries: const [
        ConfidenceEntry(score: 2.0, weight: 0.5, confidence: 2),
        ConfidenceEntry(score: null, weight: 0.5, confidence: 3),
      ],
    );
    expect(res.reached, isFalse);
    expect(res.assigned[1], closeTo(7.0, 1e-9));
  });

  test('examen/final con switch participa como fila top con su peso', () {
    // Final pendiente 0.7 conf 1, examen pendiente 0.3 conf 3, sin conocidos.
    // base = 4.0, confProm = 1.6. Tentativos -0.2 y 13.8: se fija el más
    // violador (examen a 7.0) y se recalcula final a ~2.7, total 4.0.
    final res = distributeNeeded(
      target: 4.0,
      entries: const [
        ConfidenceEntry(score: null, weight: 0.7, confidence: 1),
        ConfidenceEntry(score: null, weight: 0.3, confidence: 3),
      ],
    );
    expect(res.reached, isTrue);
    expect(res.assigned[1], closeTo(7.0, 1e-9));
    expect(res.assigned[0]!, lessThan(4.0));
    final total = res.assigned[0]! * 0.7 + res.assigned[1]! * 0.3;
    expect(total, closeTo(4.0, 1e-9));
  });

  test('validador de rango previo sigue bloqueando 70/50', () {
    // El clamp solo aplica a lo calculado; el input crudo 70/50 bloquea.
    expect(allScoresInRange([70.0, 50.0]), isFalse);
    expect(allScoresInRange([7.0, 1.0, null]), isTrue);
  });
}
