import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'academic_api.dart';
import 'academic_mappers.dart';
import 'academic_models.dart';

enum _ResultState { none, success, reached, invalid }

const double _passingGrade = 4.0;

/// Redondeo half-up para mostrar resultados a una décima.
double roundHalfUp(double value, int decimals) {
  final factor = math.pow(10, decimals).toDouble();
  return (value * factor + 0.5).floorToDouble() / factor;
}

/// Conversión para el futuro contrato academic.grades.score (10-70).
int toRawScore(double uiScore) {
  return roundHalfUp(uiScore * 10, 0).round().clamp(10, 70);
}

double fromRawScore(int rawScore) => rawScore / 10;

double? _parseNumber(String raw) {
  final normalized = raw.trim().replaceAll(',', '.');
  if (normalized.isEmpty) return null;
  return double.tryParse(normalized);
}

/// Nota del padre calculada como promedio ponderado de sus subnotas:
/// nota_padre = sum(subnota_i * subpeso_i) / sum(subpeso_i).
double? _computedParentNote(_GradeItem parent) {
  var sumScore = 0.0;
  var sumWeight = 0.0;
  for (final child in parent.children) {
    final score = _parseNumber(child.scoreController.text);
    final weight = (_parseNumber(child.weightController.text) ?? 0)
        .clamp(0.0, 100.0)
        .toDouble();
    if (score != null) {
      sumScore += score * weight;
      sumWeight += weight;
    }
  }
  if (sumWeight == 0) return null;
  return sumScore / sumWeight;
}

final RegExp _numberRegex = RegExp(r'^\d{0,3}([.,]\d{0,2})?$');

class _GradeItem {
  _GradeItem({required this.id})
    : scoreController = TextEditingController(),
      weightController = TextEditingController(),
      children = [];

  final String id;
  final TextEditingController scoreController;
  final TextEditingController weightController;
  List<_GradeItem> children;

  /// Nivel de confianza 1/2/3 (default 2 = Media) para el reparto.
  int confidence = 2;

  /// Nombre backend preservado al hidratar (para guardar sin renombrar).
  String? backendName;

  bool get isSubdivided => children.isNotEmpty;

  void dispose() {
    scoreController.dispose();
    weightController.dispose();
    for (final child in children) {
      child.dispose();
    }
  }
}

class _CalcEntry {
  const _CalcEntry({
    required this.label,
    required this.score,
    required this.weight,
  });

  final String label;
  final double? score;
  final double weight;
}

/// Hoja para el reparto por confianza (tops simples, examen/final o hijas).
class _ConfidenceLeaf {
  const _ConfidenceLeaf({required this.item, required this.entry});

  final _GradeItem item;
  final ConfidenceEntry entry;
}

class GradeCalculatorScreen extends StatefulWidget {
  const GradeCalculatorScreen({
    super.key,
    this.initialCourse,
    this.embedded = false,
    this.onSaved,
  });

  /// Curso preseleccionado (UX materiales: materia de la ventana).
  /// Si no está en la lista, se antepone.
  final String? initialCourse;

  /// Embebida en otra ventana (sin Scaffold/scroll/header propios:
  /// los provee el padre).
  final bool embedded;

  /// Se invoca tras un guardado backend exitoso (sin fallos parciales)
  /// para que el padre recargue IDs vivos (rename sobre IDs nuevos).
  final VoidCallback? onSaved;

  @override
  State<GradeCalculatorScreen> createState() => _GradeCalculatorScreenState();
}

class _GradeCalculatorScreenState extends State<GradeCalculatorScreen> {
  static const List<String> _baseCourses = [
    'Cálculo III',
    'Estructuras de Datos',
    'Física General II',
    'Ingeniería de Software',
  ];

  late final List<String> _courses;

  late String _selectedCourse;
  final TextEditingController _targetController = TextEditingController(
    text: '4.0',
  );

  /// Estado para modo normal.
  List<_GradeItem> _normalGrades = [];

  /// Estado para modo final + examen.
  _GradeItem? _finalGrade;
  _GradeItem? _exam;
  bool _finalExamMode = false;

  bool _isRedistributing = false;
  _ResultState _resultState = _ResultState.none;
  String? _error;
  double? _average;
  double? _neededPendingGrade;
  bool _isReachable = true;

  /// Fill-back de la calculadora con confianza: ids con nota auto-calculada
  /// + si con ellas se alcanza el objetivo (verde) o no (rojo). Se limpian
  /// al editar (vuelven a negro) vía [_resetResult].
  final Set<String> _calculatedIds = {};
  bool _calcReached = true;

  // Backend Academic: con sesión se hidratan las notas (GET anidados +
  // average) y se persiste el árbol (fan-out de hijas); sin sesión rige
  // el estado local.
  final AcademicApi _academicApi = AcademicApi();
  bool _backendMode = false;
  String? _backendSubjectId;
  double? _backendAverage;
  String? _backendAverageStatus;
  String? _backendError;
  bool _isSavingBackend = false;
  bool _isReloading = false;
  bool _loadedFromBackend = false;

  @override
  void initState() {
    super.initState();
    final initial = widget.initialCourse?.trim();
    if (initial != null &&
        initial.isNotEmpty &&
        !_baseCourses.contains(initial)) {
      _courses = [initial, ..._baseCourses];
    } else {
      _courses = List<String>.of(_baseCourses);
    }
    _selectedCourse = (initial != null && initial.isNotEmpty)
        ? initial
        : _courses.first;
    _initializeNormalGrades();
    _loadFromBackend();
  }

  @override
  void dispose() {
    _targetController.dispose();
    _disposeNormalGrades();
    _finalGrade?.dispose();
    _exam?.dispose();
    _academicApi.dispose();
    super.dispose();
  }

  // ---------------------------------------------------------------------
  // INITIALIZATION / RESET
  // ---------------------------------------------------------------------

  void _initializeNormalGrades() {
    _disposeNormalGrades();
    _normalGrades = List.generate(
      5,
      (index) => _GradeItem(id: 'normal-$index'),
    );
    _normalGrades.first.weightController.text = '100';
  }

  void _disposeNormalGrades() {
    for (final item in _normalGrades) {
      item.dispose();
    }
    _normalGrades = [];
  }

  void _resetResult() {
    _resultState = _ResultState.none;
    _error = null;
    _average = null;
    _neededPendingGrade = null;
    _isReachable = true;
    _calculatedIds.clear();
    _calcReached = true;
  }

  /// Confianza 1/2/3: invalida el resultado (limpia fill verde/rojo).
  void _changeConfidence(_GradeItem item, int value) {
    if (value < 1 || value > 3 || item.confidence == value) return;
    setState(() {
      item.confidence = value;
      _resetResult();
    });
  }

  Color _scoreColor(String id) {
    if (!_calculatedIds.contains(id)) return AppColors.text;
    return _calcReached ? AppColors.successDeep : AppColors.error;
  }

  String _formatCalc(double v) {
    return roundHalfUp(v, 1).toStringAsFixed(1);
  }

  void _resetCalculator() {
    setState(() {
      _resetResult();
      _loadedFromBackend = false;

      if (_finalExamMode) {
        _finalGrade?.dispose();
        _exam?.dispose();
        _finalGrade = _newItem('final', weight: 70);
        _exam = _newItem('exam', weight: 30);
      } else {
        _initializeNormalGrades();
      }
    });
  }

  _GradeItem _newItem(String id, {double? weight}) {
    final item = _GradeItem(id: id);
    if (weight != null) {
      item.weightController.text = _formatWeight(weight);
    }
    return item;
  }

  // ---------------------------------------------------------------------
  // BACKEND (hidratar + persistir)
  // ---------------------------------------------------------------------

  /// Curso visible -> materia backend (match por nombre, insensible a caso).
  AcademicSubject? _matchSubject(
    List<AcademicSubject> subjects,
    String course,
  ) {
    final want = course.trim().toLowerCase();
    for (final s in subjects) {
      if (s.name.trim().toLowerCase() == want) return s;
    }
    for (final s in subjects) {
      final name = s.name.trim().toLowerCase();
      if (name.contains(want) || want.contains(name)) return s;
    }
    return null;
  }

  String _formatScore(double v) {
    if (v == v.roundToDouble()) return v.toStringAsFixed(0);
    return v.toStringAsFixed(1);
  }

  _GradeItem _itemFromGrade(AcademicGrade g, String id) {
    final item = _GradeItem(id: id)..backendName = g.name;
    final ui = gradeRawToUi(g.score);
    if (ui != null) item.scoreController.text = _formatScore(ui);
    item.weightController.text = _formatWeight(fractionToPercent(g.weight));
    return item;
  }

  /// Hidrata el estado desde GET grades anidados + average. Sin match o sin
  /// sesión: modo local (comportamiento previo).
  ///
  /// Con [manual] (botón "Cargar Notas" en embebida): recarga bajo demanda
  /// que descarta cambios locales; reutiliza el mismo path post-guardado
  /// (entries + lista + promedio vía `onSaved`). Vacío backend → SnackBar
  /// [reloadEmptyMessage] sin tocar lo local. Error (sin sesión/red) →
  /// error visible + SnackBar (el botón reintenta, patrón existente).
  Future<void> _loadFromBackend({bool manual = false}) async {
    if (manual) {
      if (_isReloading) return;
      setState(() {
        _isReloading = true;
      });
    }
    AcademicSubject? match;
    List<AcademicGradeNode> nodes = const [];
    AcademicAverage? avg;
    try {
      final malla = await _academicApi.ensureCurriculum();
      final subjects = await _academicApi.listSubjects(malla.id);
      match = _matchSubject(subjects, _selectedCourse);
      if (match == null) {
        if (!mounted) return;
        if (manual) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text(reloadEmptyMessage),
              backgroundColor: AppColors.border,
            ),
          );
          return;
        }
        setState(() {
          _backendMode = false;
          _backendSubjectId = null;
          _backendAverage = null;
          _backendAverageStatus = null;
          _backendError = null;
        });
        return;
      }
      nodes = await _academicApi.listGrades(match.id);
      try {
        avg = await _academicApi.getAverage(match.id);
      } catch (_) {
        avg = null;
      }
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      if (manual) {
        setState(() {
          _backendMode = false;
          _backendSubjectId = null;
          _backendAverage = null;
          _backendAverageStatus = null;
          _backendError = e.message;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(e.message), backgroundColor: AppColors.border),
        );
        return;
      }
      setState(() {
        _backendMode = false;
        _backendSubjectId = null;
        _backendAverage = null;
        _backendAverageStatus = null;
        // Sin sesión es el modo local esperado: sin error visible.
        _backendError = e.code == 'no_session' ? null : e.message;
      });
      return;
    } finally {
      if (manual && mounted) {
        setState(() {
          _isReloading = false;
        });
      }
    }
    if (!mounted) return;
    if (manual) {
      if (!hasSavedNotes(nodes)) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text(reloadEmptyMessage),
            backgroundColor: AppColors.border,
          ),
        );
        return;
      }
      final subjectId = match.id;
      final average = avg;
      setState(() {
        _resetResult();
        _disposeNormalGrades();
        _normalGrades = nodes.map((n) {
          final item = _itemFromGrade(n.grade, 'backend-${n.grade.id}');
          item.children = n.children
              .map((c) => _itemFromGrade(c, 'backend-${c.id}'))
              .toList();
          return item;
        }).toList();
        if (_normalGrades.isEmpty) _initializeNormalGrades();
        _backendMode = true;
        _backendSubjectId = subjectId;
        _backendAverage = average?.average;
        _backendAverageStatus = average?.status;
        _backendError = null;
        _loadedFromBackend = true;
      });
      // Mismo path post-guardado: recarga lista + promedio en el padre para
      // no desincronizar (recarga fresca, sin duplicados ni IDs viejos).
      final cb = widget.onSaved;
      if (cb != null) cb();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(reloadSuccessMessage),
          backgroundColor: AppColors.border,
        ),
      );
      return;
    }
    setState(() {
      _resetResult();
      _disposeNormalGrades();
      _normalGrades = nodes.map((n) {
        final item = _itemFromGrade(n.grade, 'backend-${n.grade.id}');
        item.children = n.children
            .map((c) => _itemFromGrade(c, 'backend-${c.id}'))
            .toList();
        return item;
      }).toList();
      if (_normalGrades.isEmpty) _initializeNormalGrades();
      _backendMode = true;
      _backendSubjectId = match!.id;
      _backendAverage = avg?.average;
      _backendAverageStatus = avg?.status;
      _backendError = null;
    });
  }

  /// Estado local -> borradores (pesos %→fracción, notas UI→10–70).
  /// Omite filas totalmente vacías (sin nota, sin peso y sin hijas).
  List<GradeDraft> _normalDrafts() {
    final out = <GradeDraft>[];
    for (var i = 0; i < _normalGrades.length; i++) {
      final item = _normalGrades[i];
      final score = gradeUiToRaw(_parseNumber(item.scoreController.text));
      final weight = percentToFraction(_readWeight(item)) ?? 0;
      final children = <GradeDraft>[];
      for (var j = 0; j < item.children.length; j++) {
        final ch = item.children[j];
        final cs = gradeUiToRaw(_parseNumber(ch.scoreController.text));
        final cw = percentToFraction(_readWeight(ch)) ?? 0;
        if (cs == null && cw == 0) continue;
        children.add(
          GradeDraft(
            name: ch.backendName ?? 'SUBNOTA ${j + 1}',
            score: cs,
            weight: cw,
            status: cs != null ? 'graded' : 'pending',
          ),
        );
      }
      if (score == null && weight == 0 && children.isEmpty) continue;
      out.add(
        GradeDraft(
          name: item.backendName ?? 'NOTA ${i + 1}',
          score: score,
          weight: weight,
          status: score != null ? 'graded' : 'pending',
          children: children,
        ),
      );
    }
    return out;
  }

  /// Persiste el árbol (borra y recrea con fan-out de hijas). Reintentable
  /// sin duplicar.
  Future<void> _saveToBackend() async {
    final subjectId = _backendSubjectId;
    if (!_backendMode || subjectId == null || _isSavingBackend) return;
    setState(() {
      _isSavingBackend = true;
      _backendError = null;
    });
    try {
      final List<GradeDraft> tops;
      if (_finalExamMode && _finalGrade != null && _exam != null) {
        final fs = gradeUiToRaw(
          _parseNumber(_finalGrade!.scoreController.text),
        );
        final es = gradeUiToRaw(_parseNumber(_exam!.scoreController.text));
        tops = [
          GradeDraft(
            name: _finalGrade!.backendName ?? 'Final',
            score: fs,
            weight: (percentToFraction(_readWeight(_finalGrade!)) ?? 0.7),
            status: fs != null ? 'graded' : 'pending',
          ),
          GradeDraft(
            name: _exam!.backendName ?? 'Examen',
            score: es,
            weight: (percentToFraction(_readWeight(_exam!)) ?? 0.3),
            status: es != null ? 'graded' : 'pending',
            isFinalExam: true,
          ),
        ];
      } else {
        tops = _normalDrafts();
      }
      final res = await _academicApi.saveGradeTree(
        subjectId: subjectId,
        tops: tops,
      );
      AcademicAverage? avg;
      try {
        avg = await _academicApi.getAverage(subjectId);
      } catch (_) {
        avg = null;
      }
      if (!mounted) return;
      setState(() {
        _backendAverage = avg?.average;
        _backendAverageStatus = avg?.status;
      });
      final problems = [
        ...res.failedTops.entries.map((e) => '${e.key}: ${e.value}'),
        ...res.failedChildren.entries.map((e) => '${e.key}: ${e.value}'),
        if (res.deleteError != null) 'borrado: ${res.deleteError}',
      ];
      final cb = widget.onSaved;
      if (problems.isEmpty && cb != null) cb();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            problems.isEmpty
                ? 'Notas guardadas correctamente'
                : 'Guardado parcial: ${problems.join(' · ')}',
          ),
          backgroundColor: AppColors.border,
        ),
      );
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      setState(() => _backendError = e.message);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('No se pudo guardar: ${e.message}'),
          backgroundColor: AppColors.border,
        ),
      );
    } finally {
      if (mounted) setState(() => _isSavingBackend = false);
    }
  }

  /// Recordatorio compacto: todo lo de esta sección es simulación local.
  Widget _buildSimulatorNotice() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const NeobrutalistBadge(
          label: 'Simulación',
          tone: NeobrutalistTone.info,
          compact: true,
        ),
        const SizedBox(width: AppDimens.spaceSm),
        const Expanded(
          child: Text(
            'Prueba escenarios aquí. Nada se guarda como nota oficial hasta que uses Guardar.',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 11.5,
              color: AppColors.mutedStrong,
              height: 1.3,
            ),
          ),
        ),
      ],
    );
  }

  /// Estado de la fuente oficial. En la versión embebida el promedio oficial
  /// ya lo muestra el encabezado de la pestaña, así que aquí solo se muestran
  /// errores (evita duplicar el mismo valor).
  Widget _buildSourceStatus() {
    if (_backendError != null) {
      return Padding(
        padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
        child: Text(
          'No se pudieron cargar tus notas oficiales: $_backendError',
          style: const TextStyle(
            fontWeight: FontWeight.w700,
            fontSize: 11.5,
            color: AppColors.errorDeep,
          ),
        ),
      );
    }
    if (widget.embedded) return const SizedBox.shrink();
    final value = _backendAverage;
    if (value == null) {
      if (!_backendMode) return const SizedBox.shrink();
      return const Padding(
        padding: EdgeInsets.only(bottom: AppDimens.spaceMd),
        child: Text(
          'SIN NOTAS OFICIALES REGISTRADAS',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 11,
            letterSpacing: 0.4,
            color: AppColors.mutedStrong,
          ),
        ),
      );
    }
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      child: Row(
        children: [
          NeobrutalistBadge(
            label: 'Promedio oficial ${value.toStringAsFixed(1)}',
            tone: NeobrutalistTone.accent,
            icon: Icons.fact_check_rounded,
          ),
          if (_backendAverageStatus != null) ...[
            const SizedBox(width: AppDimens.spaceSm),
            NeobrutalistBadge(
              label: _formatAverageStatus(_backendAverageStatus!),
              tone: NeobrutalistTone.neutral,
            ),
          ],
        ],
      ),
    );
  }

  /// Traduce estados del backend a etiquetas comprensibles.
  String _formatAverageStatus(String raw) {
    final key = raw.trim().toLowerCase();
    return switch (key) {
      'approved' || 'aprobado' => 'Aprobado',
      'in_progress' || 'en curso' || 'cursando' => 'En curso',
      'pending' || 'pendiente' => 'Pendiente',
      'failed' || 'reprobado' => 'Reprobado',
      _ => raw,
    };
  }

  /// Aviso al cargar notas oficiales: son copias editables para simular.
  Widget _buildEditableCopyNotice() {
    return Container(
      margin: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      decoration: BoxDecoration(
        color: AppColors.accentYellow,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: const Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.content_copy_rounded, size: 18, color: AppColors.text),
          SizedBox(width: AppDimens.spaceSm),
          Expanded(
            child: Text(
              'Copias editables: estas notas se cargaron desde tus evaluaciones oficiales. Editarlas aquí solo afecta la simulación; no se guardan hasta que uses Guardar.',
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w700,
                color: AppColors.text,
                height: 1.35,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Sección de resultado: visible de inmediato, con pista cuando aún no hay.
  Widget _buildResultSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Text(
          'RESULTADO DE LA SIMULACIÓN',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 12,
            letterSpacing: 0.6,
            color: AppColors.text,
          ),
        ),
        const SizedBox(height: AppDimens.spaceSm),
        if (_resultState == _ResultState.none)
          Container(
            padding: const EdgeInsets.all(AppDimens.spaceLg),
            decoration: BoxDecoration(
              color: AppColors.surfaceLow,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radius),
            ),
            child: const Text(
              'Completa tus notas y presiona Calcular para ver el resultado aquí.',
              style: TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 12,
                color: AppColors.mutedStrong,
                height: 1.35,
              ),
            ),
          )
        else
          _buildResult(),
      ],
    );
  }

  // ---------------------------------------------------------------------
  // MODE SWITCH
  // ---------------------------------------------------------------------

  void _setFinalExamMode(bool enabled) {
    if (_finalExamMode == enabled) return;

    setState(() {
      _finalExamMode = enabled;
      _resetResult();

      if (enabled) {
        _disposeNormalGrades();
        _finalGrade?.dispose();
        _exam?.dispose();

        _finalGrade = _newItem('final', weight: 70);
        _exam = _newItem('exam', weight: 30);
      } else {
        _finalGrade?.dispose();
        _exam?.dispose();
        _finalGrade = null;
        _exam = null;
        _initializeNormalGrades();
      }
    });
  }

  // ---------------------------------------------------------------------
  // WEIGHT DISTRIBUTION
  // ---------------------------------------------------------------------

  String _formatWeight(double value) {
    final rounded = roundHalfUp(value, 1);
    if (rounded == rounded.roundToDouble()) {
      return rounded.toStringAsFixed(0);
    }
    return rounded.toStringAsFixed(1);
  }

  double _readWeight(_GradeItem item) {
    return (_parseNumber(item.weightController.text) ?? 0)
        .clamp(0.0, 100.0)
        .toDouble();
  }

  /// Dos componentes complementarios: final 70/examen 30, o viceversa.
  void _changeFinalWeight() {
    final finalItem = _finalGrade;
    final examItem = _exam;
    if (_isRedistributing || finalItem == null || examItem == null) return;

    setState(() {
      _isRedistributing = true;
      try {
        final finalWeight = _readWeight(finalItem);
        final textF = _formatWeight(finalWeight);
        final textE = _formatWeight(100 - finalWeight);
        finalItem.weightController.value = TextEditingValue(
          text: textF,
          selection: TextSelection.collapsed(offset: textF.length),
        );
        examItem.weightController.value = TextEditingValue(
          text: textE,
          selection: TextSelection.collapsed(offset: textE.length),
        );
      } finally {
        _isRedistributing = false;
      }
      _resetResult();
    });
  }

  void _changeExamWeight() {
    final finalItem = _finalGrade;
    final examItem = _exam;
    if (_isRedistributing || finalItem == null || examItem == null) return;

    setState(() {
      _isRedistributing = true;
      try {
        final examWeight = _readWeight(examItem);
        final textE = _formatWeight(examWeight);
        final textF = _formatWeight(100 - examWeight);
        examItem.weightController.value = TextEditingValue(
          text: textE,
          selection: TextSelection.collapsed(offset: textE.length),
        );
        finalItem.weightController.value = TextEditingValue(
          text: textF,
          selection: TextSelection.collapsed(offset: textF.length),
        );
      } finally {
        _isRedistributing = false;
      }
      _resetResult();
    });
  }

  /// Peso normal: el elemento editado queda fijo y el restante se reparte
  /// entre los demás. Ejemplo: 20% -> el resto pasa a sumar 80%.
  void _changeNormalWeight(_GradeItem changed) {
    if (_isRedistributing) return;

    setState(() {
      _redistributeNormalWeights(changed);
      _resetResult();
    });
  }

  void _redistributeNormalWeights(_GradeItem changed) {
    if (_normalGrades.isEmpty) return;

    _isRedistributing = true;
    try {
      final changedWeight = _readWeight(changed);
      final tChanged = _formatWeight(changedWeight);
      changed.weightController.value = TextEditingValue(
        text: tChanged,
        selection: TextSelection.collapsed(offset: tChanged.length),
      );

      final otherGrades = _normalGrades
          .where((item) => item != changed)
          .toList();
      if (otherGrades.isEmpty) return;

      final remaining = 100 - changedWeight;
      final previousTotal = otherGrades.fold<double>(
        0,
        (sum, item) => sum + _readWeight(item),
      );

      if (previousTotal == 0) {
        final equalWeight = remaining / otherGrades.length;
        for (final item in otherGrades) {
          final t = _formatWeight(equalWeight);
          item.weightController.value = TextEditingValue(
            text: t,
            selection: TextSelection.collapsed(offset: t.length),
          );
        }
      } else {
        for (final item in otherGrades) {
          final proportionalWeight =
              remaining * _readWeight(item) / previousTotal;
          final t = _formatWeight(proportionalWeight);
          item.weightController.value = TextEditingValue(
            text: t,
            selection: TextSelection.collapsed(offset: t.length),
          );
        }
      }
    } finally {
      _isRedistributing = false;
    }
  }

  /// Pesos dentro de una nota subdividida suman 100% relativo al padre.
  void _changeChildWeight(_GradeItem parent, _GradeItem changed) {
    if (_isRedistributing || parent.children.isEmpty) return;

    setState(() {
      _isRedistributing = true;
      try {
        final changedWeight = _readWeight(changed);
        final tChanged = _formatWeight(changedWeight);
        changed.weightController.value = TextEditingValue(
          text: tChanged,
          selection: TextSelection.collapsed(offset: tChanged.length),
        );

        final others = parent.children
            .where((item) => item != changed)
            .toList();
        if (others.isEmpty) return;

        final remaining = 100 - changedWeight;
        final previousTotal = others.fold<double>(
          0,
          (sum, item) => sum + _readWeight(item),
        );

        if (previousTotal == 0) {
          final equalWeight = remaining / others.length;
          for (final item in others) {
            final t = _formatWeight(equalWeight);
            item.weightController.value = TextEditingValue(
              text: t,
              selection: TextSelection.collapsed(offset: t.length),
            );
          }
        } else {
          for (final item in others) {
            final t = _formatWeight(
              remaining * _readWeight(item) / previousTotal,
            );
            item.weightController.value = TextEditingValue(
              text: t,
              selection: TextSelection.collapsed(offset: t.length),
            );
          }
        }
      } finally {
        _isRedistributing = false;
      }
      _resetResult();
    });
  }

  // ---------------------------------------------------------------------
  // NORMAL GRADES / SUBDIVISION
  // ---------------------------------------------------------------------

  void _addNormalGrade() {
    if (_finalExamMode) return;

    setState(() {
      final newItem = _newItem(
        'normal-${DateTime.now().microsecondsSinceEpoch}',
      );
      _normalGrades.add(newItem);

      final equalWeight = 100 / _normalGrades.length;
      for (final item in _normalGrades) {
        item.weightController.text = _formatWeight(equalWeight);
      }

      _resetResult();
    });
  }

  void _removeLastNormalGrade() {
    if (_finalExamMode || _normalGrades.length <= 1) return;

    setState(() {
      _normalGrades.removeLast().dispose();

      final equalWeight = 100 / _normalGrades.length;
      for (final item in _normalGrades) {
        item.weightController.text = _formatWeight(equalWeight);
      }

      _resetResult();
    });
  }

  void _subdivideNormalGrade(_GradeItem parent) {
    if (_finalExamMode || parent.isSubdivided) return;

    setState(() {
      final firstChild = _newItem('${parent.id}-child-1', weight: 50);
      final secondChild = _newItem('${parent.id}-child-2', weight: 50);

      parent.children = [firstChild, secondChild];
      _resetResult();
    });
  }

  void _addChild(_GradeItem parent) {
    if (_finalExamMode) return;

    setState(() {
      parent.children.add(
        _newItem('${parent.id}-child-${DateTime.now().microsecondsSinceEpoch}'),
      );

      final equalWeight = 100 / parent.children.length;
      for (final child in parent.children) {
        child.weightController.text = _formatWeight(equalWeight);
      }

      _resetResult();
    });
  }

  /// Al quedar una sola subnota, la nota vuelve al formato normal.
  void _removeChild(_GradeItem parent) {
    if (_finalExamMode || parent.children.length <= 1) return;

    setState(() {
      parent.children.removeLast().dispose();

      if (parent.children.length == 1) {
        final survivor = parent.children.single;
        parent.scoreController.text = survivor.scoreController.text;
        survivor.dispose();
        parent.children = [];
      } else {
        final equalWeight = 100 / parent.children.length;
        for (final child in parent.children) {
          child.weightController.text = _formatWeight(equalWeight);
        }
      }

      _resetResult();
    });
  }

  // ---------------------------------------------------------------------
  // CALCULATION
  // ---------------------------------------------------------------------

  /// Notas crudas de todos los controladores (tops, hijas, examen/final)
  /// para validar rango antes de aplanar/promediar.
  List<double?> _rawScores() {
    final out = <double?>[];
    void collect(_GradeItem item) {
      out.add(_parseNumber(item.scoreController.text));
      for (final child in item.children) {
        out.add(_parseNumber(child.scoreController.text));
      }
    }

    if (_finalExamMode && _finalGrade != null && _exam != null) {
      collect(_finalGrade!);
      collect(_exam!);
    } else {
      for (final item in _normalGrades) {
        collect(item);
      }
    }
    return out;
  }

  List<_CalcEntry> _buildEntries() {
    final entries = <_CalcEntry>[];

    void flatten(_GradeItem item, String label, double globalWeight) {
      if (item.isSubdivided) {
        var sumScore = 0.0;
        var sumWeight = 0.0;
        for (final child in item.children) {
          final childScore = _parseNumber(child.scoreController.text);
          final childWeight = _readWeight(child);
          if (childScore != null) {
            sumScore += childScore * childWeight;
            sumWeight += childWeight;
          }
        }
        entries.add(
          _CalcEntry(
            label: label,
            score: sumWeight > 0 ? sumScore / sumWeight : null,
            weight: globalWeight,
          ),
        );
        return;
      }

      entries.add(
        _CalcEntry(
          label: label,
          score: _parseNumber(item.scoreController.text),
          weight: globalWeight,
        ),
      );
    }

    if (_finalExamMode && _finalGrade != null && _exam != null) {
      flatten(_finalGrade!, 'Nota final', _readWeight(_finalGrade!) / 100);
      flatten(_exam!, 'Examen', _readWeight(_exam!) / 100);
    } else {
      for (var index = 0; index < _normalGrades.length; index++) {
        final item = _normalGrades[index];
        flatten(item, 'Nota ${index + 1}', _readWeight(item) / 100);
      }
    }

    return entries;
  }

  double _totalWeight() {
    return _buildEntries().fold<double>(0, (sum, entry) => sum + entry.weight);
  }

  /// Hojas para el reparto por confianza: tops simples, examen/final o
  /// hijas con peso efectivo (padre × hija) antes del promedio al padre.
  List<_ConfidenceLeaf> _buildConfidenceLeaves() {
    final out = <_ConfidenceLeaf>[];
    if (_finalExamMode && _finalGrade != null && _exam != null) {
      for (final item in <_GradeItem>[_finalGrade!, _exam!]) {
        out.add(
          _ConfidenceLeaf(
            item: item,
            entry: ConfidenceEntry(
              score: _parseNumber(item.scoreController.text),
              weight: _readWeight(item) / 100,
              confidence: item.confidence,
            ),
          ),
        );
      }
      return out;
    }
    for (final top in _normalGrades) {
      final parentFrac = _readWeight(top) / 100;
      if (top.isSubdivided) {
        for (final child in top.children) {
          final eff = parentFrac * (_readWeight(child) / 100);
          out.add(
            _ConfidenceLeaf(
              item: child,
              entry: ConfidenceEntry(
                score: _parseNumber(child.scoreController.text),
                weight: eff,
                confidence: child.confidence,
              ),
            ),
          );
        }
      } else {
        out.add(
          _ConfidenceLeaf(
            item: top,
            entry: ConfidenceEntry(
              score: _parseNumber(top.scoreController.text),
              weight: parentFrac,
              confidence: top.confidence,
            ),
          ),
        );
      }
    }
    return out;
  }

  void _calculate() {
    FocusManager.instance.primaryFocus?.unfocus();

    final target = _parseNumber(_targetController.text);
    if (target == null || target < 1 || target > 7) {
      _showError('La meta debe estar entre 1.0 y 7.0.');
      return;
    }

    final totalWeight = _totalWeight();
    if ((totalWeight - 1).abs() > 0.005) {
      _showError('La ponderación total debe sumar 100%.');
      return;
    }

    // Toda nota conocida (tops, hijas, examen/final) en [1.0, 7.0].
    // Se valida el input crudo: las hijas se promedian al aplanar y un 80
    // podría esconderse en el promedio del padre.
    if (!allScoresInRange(_rawScores())) {
      _showError('Las notas deben estar entre 1.0 y 7.0.');
      return;
    }

    final leaves = _buildConfidenceLeaves();
    double weightedKnown = 0;
    double pendingWeight = 0;
    for (final leaf in leaves) {
      if (leaf.entry.score != null) {
        weightedKnown += leaf.entry.score! * leaf.entry.weight;
      } else if (leaf.entry.weight > 0) {
        pendingWeight += leaf.entry.weight;
      }
    }

    setState(() {
      _calculatedIds.clear();
      _error = null;

      if (pendingWeight == 0) {
        _average = roundHalfUp(weightedKnown, 1);
        _neededPendingGrade = null;
        _resultState = _ResultState.success;
        _isReachable = weightedKnown >= target;
        _calcReached = true;
      } else {
        final res = computeNeeded(
          target: target,
          weightedKnown: weightedKnown,
          pendingWeight: pendingWeight,
        );
        if (res.status == NeededStatus.reached) {
          for (final leaf in leaves) {
            if (leaf.entry.score == null && leaf.entry.weight > 0) {
              leaf.item.scoreController.text = '1.0';
              _calculatedIds.add(leaf.item.id);
            }
          }
          _calcReached = true;
          _average = roundHalfUp(weightedKnown + pendingWeight * 1.0, 1);
          _neededPendingGrade = null;
          _resultState = _ResultState.reached;
          _isReachable = true;
        } else {
          final dist = distributeNeeded(
            target: target,
            entries: leaves.map((l) => l.entry).toList(),
          );
          _calcReached = dist.reached;
          for (var i = 0; i < leaves.length; i++) {
            final orig = leaves[i].entry.score;
            final a = dist.assigned[i];
            if (orig == null && a != null && leaves[i].entry.weight > 0) {
              leaves[i].item.scoreController.text = _formatCalc(a);
              _calculatedIds.add(leaves[i].item.id);
            }
          }
          _average = roundHalfUp(weightedKnown, 1);
          if (dist.reached) {
            _neededPendingGrade = roundHalfUp(res.needed!, 1);
            _resultState = _ResultState.success;
            _isReachable = true;
          } else {
            _neededPendingGrade = null;
            _resultState = _ResultState.success;
            _isReachable = false;
          }
        }
      }
    });
  }

  void _showError(String message) {
    setState(() {
      _error = message;
      _average = null;
      _neededPendingGrade = null;
      _resultState = _ResultState.invalid;
      _calculatedIds.clear();
      _calcReached = true;
    });
  }

  // ---------------------------------------------------------------------
  // SCREEN LAYOUT
  // ---------------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        MediaQuery.of(context).size.width > AppDimens.breakpointMedium;

    final content = Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (!widget.embedded) _buildHeader(),
        if (!widget.embedded) const SizedBox(height: AppDimens.spaceXl),
        _buildSimulatorNotice(),
        const SizedBox(height: AppDimens.spaceMd),
        if (widget.embedded)
          Padding(
            padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
            child: Text(
              _selectedCourse.toUpperCase(),
              style: const TextStyle(
                fontWeight: FontWeight.w800,
                fontSize: 11,
                letterSpacing: 0.5,
                color: AppColors.mutedStrong,
              ),
            ),
          ),
        _buildConfiguration(isDesktop),
        const SizedBox(height: AppDimens.spaceLg),
        _buildSourceStatus(),
        _buildReloadAction(),
        if (_loadedFromBackend) _buildEditableCopyNotice(),
        _finalExamMode ? _buildFinalExamMode() : _buildNormalMode(isDesktop),
        const SizedBox(height: AppDimens.spaceLg),
        _buildWeightSummary(),
        const SizedBox(height: AppDimens.spaceXl),
        _buildActions(),
        const SizedBox(height: AppDimens.spaceLg),
        _buildResultSection(),
      ],
    );

    // Embebida: el padre provee Scaffold + scroll; se conserva el ancho.
    if (widget.embedded) return content;

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: EdgeInsets.symmetric(
            horizontal: isDesktop ? AppDimens.spaceXxl : AppDimens.spaceLg,
            vertical: AppDimens.spaceXl,
          ),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 1120),
              child: content,
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildHeader() {
    final canPop = Navigator.of(context).canPop();

    return Row(
      children: [
        if (canPop) ...[
          _SquareIconButton(
            icon: Icons.arrow_back_rounded,
            tooltip: 'Volver',
            onPressed: () => Navigator.of(context).pop(),
          ),
          const SizedBox(width: 12),
        ],
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'CALCULADORA DE NOTAS',
                style: TextStyle(
                  color: AppColors.text,
                  fontWeight: FontWeight.w900,
                  fontSize: 24,
                ),
              ),
              const SizedBox(height: 4),
              _CourseDropdown(
                courses: _courses,
                selected: _selectedCourse,
                onChanged: (course) {
                  setState(() {
                    _selectedCourse = course;
                    _resetResult();
                  });
                  _loadFromBackend();
                },
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildConfiguration(bool isDesktop) {
    final targetCard = _TargetCard(
      controller: _targetController,
      onChanged: () {
        setState(_resetResult);
      },
    );

    final modeCard = _ModeSwitchCard(
      enabled: _finalExamMode,
      onChanged: _setFinalExamMode,
    );

    if (isDesktop) {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: ConstrainedBox(
              constraints: const BoxConstraints(minHeight: 120),
              child: targetCard,
            ),
          ),
          const SizedBox(width: AppDimens.spaceMd),
          Expanded(
            child: ConstrainedBox(
              constraints: const BoxConstraints(minHeight: 120),
              child: modeCard,
            ),
          ),
        ],
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        targetCard,
        const SizedBox(height: AppDimens.spaceMd),
        modeCard,
      ],
    );
  }

  Widget _buildFinalExamMode() {
    final finalItem = _finalGrade!;
    final examItem = _exam!;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Text(
          'NOTA FINAL Y EXAMEN',
          style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14),
        ),
        const SizedBox(height: 10),
        _FinalGradeCard(
          item: finalItem,
          onScoreChanged: () => setState(_resetResult),
          onWeightChanged: _changeFinalWeight,
          onConfidenceChanged: (v) => _changeConfidence(finalItem, v),
          scoreColor: _scoreColor(finalItem.id),
        ),
        const SizedBox(height: 12),
        _ExamCard(
          item: examItem,
          onScoreChanged: () => setState(_resetResult),
          onWeightChanged: _changeExamWeight,
          onConfidenceChanged: (v) => _changeConfidence(examItem, v),
          scoreColor: _scoreColor(examItem.id),
        ),
      ],
    );
  }

  Widget _buildNormalMode(bool isDesktop) {
    final columns = isDesktop ? 2 : 1;
    final canRemove = !_finalExamMode && _normalGrades.length > 1;

    final header = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: const [
        Text(
          'NOTAS DEL SIMULADOR',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 13,
            letterSpacing: 0.4,
            color: AppColors.text,
          ),
        ),
        SizedBox(height: AppDimens.spaceXs),
        Text(
          'Agrega, subdivide o ajusta ponderaciones para simular.',
          style: TextStyle(
            fontWeight: FontWeight.w700,
            fontSize: 11.5,
            color: AppColors.mutedStrong,
          ),
        ),
      ],
    );

    final addButton = NeobrutalistButton(
      label: 'Agregar nota',
      icon: Icons.add_rounded,
      variant: NeobrutalistButtonVariant.secondary,
      onPressed: _addNormalGrade,
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (context.isCompact)
          Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              header,
              const SizedBox(height: AppDimens.spaceSm),
              addButton,
            ],
          )
        else
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(child: header),
              const SizedBox(width: AppDimens.spaceMd),
              addButton,
            ],
          ),
        const SizedBox(height: AppDimens.spaceMd),
        LayoutBuilder(
          builder: (context, constraints) {
            final width = columns == 1
                ? constraints.maxWidth
                : (constraints.maxWidth - AppDimens.spaceMd) / 2;

            return Wrap(
              spacing: AppDimens.spaceMd,
              runSpacing: AppDimens.spaceMd,
              children: _normalGrades.map((item) {
                final index = _normalGrades.indexOf(item);

                return SizedBox(
                  width: width,
                  child: _NormalGradeCard(
                    key: ValueKey(item.id),
                    item: item,
                    label: gradeRowLabel(item.backendName, 'NOTA ${index + 1}'),
                    onScoreChanged: () => setState(_resetResult),
                    onWeightChanged: () => _changeNormalWeight(item),
                    onSubdivide: () => _subdivideNormalGrade(item),
                    onChildScoreChanged: () => setState(_resetResult),
                    onChildWeightChanged: (child) =>
                        _changeChildWeight(item, child),
                    onAddChild: () => _addChild(item),
                    onRemoveChild: () => _removeChild(item),
                    onConfidenceChanged: (v) => _changeConfidence(item, v),
                    scoreColor: _scoreColor(item.id),
                    onChildConfidenceChanged: (child, v) =>
                        _changeConfidence(child, v),
                    childScoreColor: (child) => _scoreColor(child.id),
                  ),
                );
              }).toList(),
            );
          },
        ),
        const SizedBox(height: AppDimens.spaceSm),
        Align(
          alignment: Alignment.centerLeft,
          child: NeobrutalistButton(
            label: 'Eliminar última',
            icon: Icons.delete_outline_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: canRemove ? _removeLastNormalGrade : null,
          ),
        ),
      ],
    );
  }

  Widget _buildWeightSummary() {
    final value = _totalWeight() * 100;

    return _WeightSummary(
      label: 'PONDERACIÓN TOTAL DEL SIMULADOR',
      value: value,
      isComplete: (value - 100).abs() <= 0.5,
    );
  }

  Widget _buildActions() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Text(
          'ACCIONES',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 12,
            letterSpacing: 0.6,
            color: AppColors.text,
          ),
        ),
        const SizedBox(height: AppDimens.spaceSm),
        NeobrutalistButton(
          label: 'Calcular promedio',
          icon: Icons.calculate_rounded,
          variant: NeobrutalistButtonVariant.accent,
          expand: true,
          onPressed: _calculate,
        ),
        if (_backendMode) ...[
          const SizedBox(height: AppDimens.spaceSm),
          NeobrutalistButton(
            label: _isSavingBackend ? 'Guardando…' : 'Guardar cambios',
            icon: Icons.cloud_upload_rounded,
            variant: NeobrutalistButtonVariant.primary,
            expand: true,
            onPressed: _isSavingBackend ? null : _saveToBackend,
          ),
        ],
        const SizedBox(height: AppDimens.spaceSm),
        Align(
          alignment: Alignment.centerLeft,
          child: NeobrutalistButton(
            label: 'Reset',
            icon: Icons.restart_alt_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: _resetCalculator,
          ),
        ),
      ],
    );
  }

  /// Botón "Cargar Notas" visible siempre en la calculadora embebida.
  /// Recarga entries + lista + promedio (mismo path post-guardado).
  Widget _buildReloadAction() {
    if (!widget.embedded) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceLg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          NeobrutalistButton(
            label: _isReloading ? 'Cargando…' : 'Cargar notas',
            icon: Icons.cloud_download_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: _isReloading
                ? null
                : () => _loadFromBackend(manual: true),
          ),
          const SizedBox(height: AppDimens.spaceXs),
          const Text(
            'Descarga tus notas oficiales como copias editables para simular.',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 11,
              color: AppColors.mutedStrong,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildResult() {
    final targetValue = _parseNumber(_targetController.text);
    final stats = (
      simulated: _average != null ? _average!.toStringAsFixed(1) : '—',
      target: targetValue != null ? targetValue.toStringAsFixed(1) : '—',
      official: _backendAverage != null
          ? _backendAverage!.toStringAsFixed(1)
          : '—',
    );

    if (_resultState == _ResultState.invalid) {
      return _ResultCard(
        background: AppColors.errorDeep,
        foreground: AppColors.surface,
        title: 'ERROR',
        message: _error ?? 'Revisa los datos.',
        simulated: stats.simulated,
        target: stats.target,
        official: stats.official,
      );
    }

    if (_resultState == _ResultState.reached) {
      final current = (_average ?? 0).toStringAsFixed(1);
      final filled = _calculatedIds.isNotEmpty;
      return _ResultCard(
        background: AppColors.accentYellow,
        foreground: AppColors.text,
        title: 'META YA ALCANZADA',
        value: current,
        message: filled
            ? 'Meta alcanzada: se rellenó 1.0 en las pendientes. Promedio calculado con esos 1.0.'
            : 'Tu promedio actual ya cubre la meta. Las evaluaciones pendientes solo pueden mantenerlo.',
        simulated: stats.simulated,
        target: stats.target,
        official: stats.official,
      );
    }

    if (_neededPendingGrade != null) {
      if (_isReachable) {
        return _ResultCard(
          background: AppColors.accentYellow,
          foreground: AppColors.text,
          title: 'NOTA NECESARIA PARA ALCANZAR TU META',
          value: _neededPendingGrade!.toStringAsFixed(1),
          message:
              'Necesitas aproximadamente esta nota en las evaluaciones pendientes.',
          simulated: stats.simulated,
          target: stats.target,
          official: stats.official,
        );
      }

      return _ResultCard(
        background: AppColors.errorDeep,
        foreground: AppColors.surface,
        title: 'META NO ALCANZABLE',
        message: 'La nota necesaria supera 7.0.',
        simulated: stats.simulated,
        target: stats.target,
        official: stats.official,
      );
    }

    if (_resultState == _ResultState.success &&
        !_isReachable &&
        _calculatedIds.isNotEmpty) {
      return _ResultCard(
        background: AppColors.errorDeep,
        foreground: AppColors.surface,
        title: 'META NO ALCANZABLE',
        message: 'La nota necesaria supera 7.0.',
        simulated: stats.simulated,
        target: stats.target,
        official: stats.official,
      );
    }

    final average = _average ?? 0;
    final passed = average >= _passingGrade;

    return _ResultCard(
      background: passed ? AppColors.accentYellow : AppColors.errorDeep,
      foreground: passed ? AppColors.text : AppColors.surface,
      title: passed ? 'PROMEDIO CALCULADO' : 'REPROBASTE EL RAMO',
      value: average.toStringAsFixed(1),
      message: passed
          ? 'Aprobado con promedio igual o superior a 4.0.'
          : 'Promedio inferior a 4.0.',
      simulated: stats.simulated,
      target: stats.target,
      official: stats.official,
    );
  }
}

// -------------------------------------------------------------------------
// WIDGETS PEQUEÑOS Y REUTILIZABLES
// -------------------------------------------------------------------------

class _CourseDropdown extends StatelessWidget {
  const _CourseDropdown({
    required this.courses,
    required this.selected,
    required this.onChanged,
  });

  final List<String> courses;
  final String selected;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 2),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: selected,
          isDense: true,
          icon: const Icon(Icons.expand_more_rounded),
          style: const TextStyle(
            fontWeight: FontWeight.w800,
            fontSize: 13,
            color: AppColors.text,
          ),
          items: courses
              .map(
                (course) => DropdownMenuItem<String>(
                  value: course,
                  child: Text(course.toUpperCase()),
                ),
              )
              .toList(),
          onChanged: (value) {
            if (value != null) onChanged(value);
          },
        ),
      ),
    );
  }
}

class _TargetCard extends StatelessWidget {
  const _TargetCard({required this.controller, required this.onChanged});

  final TextEditingController controller;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(AppDimens.spaceLg),
      decoration: _cardDecoration(
        color: AppColors.surface,
        shadow: const Offset(4, 4),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Text(
            'NOTA OBJETIVO',
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 11,
              letterSpacing: 0.6,
              color: AppColors.mutedStrong,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Row(
            children: [
              const Expanded(
                child: Text(
                  'Quiero un',
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 14,
                    color: AppColors.text,
                  ),
                ),
              ),
              SizedBox(
                width: 110,
                child: _NumericInput(
                  controller: controller,
                  hint: '4.0',
                  onChanged: onChanged,
                ),
              ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceXs),
          const Text(
            'Escala 1.0 – 7.0',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 11,
              color: AppColors.mutedStrong,
            ),
          ),
        ],
      ),
    );
  }
}

class _ModeSwitchCard extends StatelessWidget {
  const _ModeSwitchCard({required this.enabled, required this.onChanged});

  final bool enabled;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(AppDimens.spaceLg),
      decoration: _cardDecoration(
        color: AppColors.accentYellow,
        shadow: const Offset(3, 3),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Text(
            'MODO DE CÁLCULO',
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 11,
              letterSpacing: 0.6,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Row(
            children: [
              const Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Solo nota final y examen',
                      style: TextStyle(
                        fontWeight: FontWeight.w900,
                        fontSize: 13,
                        color: AppColors.text,
                      ),
                    ),
                    SizedBox(height: 2),
                    Text(
                      'Oculta las notas parciales del simulador.',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 11,
                        color: AppColors.text,
                      ),
                    ),
                  ],
                ),
              ),
              Switch(
                value: enabled,
                activeThumbColor: AppColors.text,
                onChanged: onChanged,
              ),
            ],
          ),
        ],
      ),
    );
  }
}

/// Nota final/promedio: es simple, no admite subdivisión, pero conserva el
/// selector de confianza solicitado por el usuario.
class _FinalGradeCard extends StatelessWidget {
  const _FinalGradeCard({
    required this.item,
    required this.onScoreChanged,
    required this.onWeightChanged,
    required this.onConfidenceChanged,
    required this.scoreColor,
  });

  final _GradeItem item;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final ValueChanged<int> onConfidenceChanged;
  final Color scoreColor;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: _cardDecoration(
        color: AppColors.surface,
        shadow: const Offset(3, 3),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            'NOTA FINAL / PROMEDIO GENERAL',
            style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14),
          ),
          const SizedBox(height: 8),
          _ScoreAndWeightRow(
            scoreController: item.scoreController,
            weightController: item.weightController,
            scoreHint: 'Promedio',
            onScoreChanged: onScoreChanged,
            onWeightChanged: onWeightChanged,
            confidence: item.confidence,
            onConfidenceChanged: onConfidenceChanged,
            scoreColor: scoreColor,
          ),
        ],
      ),
    );
  }
}

class _ExamCard extends StatelessWidget {
  const _ExamCard({
    required this.item,
    required this.onScoreChanged,
    required this.onWeightChanged,
    required this.onConfidenceChanged,
    required this.scoreColor,
  });

  final _GradeItem item;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final ValueChanged<int> onConfidenceChanged;
  final Color scoreColor;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: _cardDecoration(
        color: AppColors.accentYellow,
        shadow: const Offset(3, 3),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            'EXAMEN',
            style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14),
          ),
          const SizedBox(height: 8),
          _ScoreAndWeightRow(
            scoreController: item.scoreController,
            weightController: item.weightController,
            scoreHint: 'Nota examen',
            onScoreChanged: onScoreChanged,
            onWeightChanged: onWeightChanged,
            confidence: item.confidence,
            onConfidenceChanged: onConfidenceChanged,
            scoreColor: scoreColor,
          ),
        ],
      ),
    );
  }
}

class _NormalGradeCard extends StatelessWidget {
  const _NormalGradeCard({
    super.key,
    required this.item,
    required this.label,
    required this.onScoreChanged,
    required this.onWeightChanged,
    required this.onSubdivide,
    required this.onChildScoreChanged,
    required this.onChildWeightChanged,
    required this.onAddChild,
    required this.onRemoveChild,
    required this.onConfidenceChanged,
    required this.scoreColor,
    required this.onChildConfidenceChanged,
    required this.childScoreColor,
  });

  final _GradeItem item;
  final String label;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final VoidCallback onSubdivide;
  final VoidCallback onChildScoreChanged;
  final ValueChanged<_GradeItem> onChildWeightChanged;
  final VoidCallback onAddChild;
  final VoidCallback onRemoveChild;
  final ValueChanged<int> onConfidenceChanged;
  final Color scoreColor;
  final void Function(_GradeItem child, int value) onChildConfidenceChanged;
  final Color Function(_GradeItem child) childScoreColor;

  @override
  Widget build(BuildContext context) {
    final parentNote = item.isSubdivided ? _computedParentNote(item) : null;

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: _cardDecoration(
        color: item.isSubdivided ? AppColors.subjectBlue : AppColors.surface,
        shadow: Offset.zero,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  label,
                  style: const TextStyle(
                    fontSize: 11.5,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                  ),
                ),
              ),
              if (item.isSubdivided)
                const NeobrutalistBadge(
                  label: 'Subdividida',
                  tone: NeobrutalistTone.info,
                  compact: true,
                ),
            ],
          ),
          const SizedBox(height: 7),
          if (!item.isSubdivided) ...[
            _ScoreAndWeightRow(
              scoreController: item.scoreController,
              weightController: item.weightController,
              scoreHint: 'Nota opcional',
              onScoreChanged: onScoreChanged,
              onWeightChanged: onWeightChanged,
              confidence: item.confidence,
              onConfidenceChanged: onConfidenceChanged,
              scoreColor: scoreColor,
            ),
            const SizedBox(height: 8),
            Align(
              alignment: Alignment.centerLeft,
              child: _MiniActionButton(
                label: 'SUBDIVIDIR NOTA',
                icon: Icons.call_split_rounded,
                onPressed: onSubdivide,
              ),
            ),
          ] else ...[
            Row(
              children: [
                const Expanded(
                  child: Text('PESO EN EL RAMO', style: _fieldLabelStyle),
                ),
                SizedBox(
                  width: 68,
                  child: _NumericInput(
                    controller: item.weightController,
                    hint: '%',
                    integer: true,
                    onChanged: onWeightChanged,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Row(
              children: [
                const Expanded(
                  child: Text('NOTA PADRE (AUTO)', style: _fieldLabelStyle),
                ),
                Text(
                  parentNote == null
                      ? '—'
                      : roundHalfUp(parentNote, 1).toStringAsFixed(1),
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w900,
                    color: AppColors.accentBlue,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            for (var index = 0; index < item.children.length; index++) ...[
              if (index != 0) const SizedBox(height: 6),
              _ChildGradeRow(
                key: ValueKey(item.children[index].id),
                label: gradeRowLabel(
                  item.children[index].backendName,
                  'SUBNOTA ${index + 1}',
                ),
                item: item.children[index],
                onScoreChanged: onChildScoreChanged,
                onWeightChanged: () {
                  onChildWeightChanged(item.children[index]);
                },
                onConfidenceChanged: (v) {
                  onChildConfidenceChanged(item.children[index], v);
                },
                scoreColor: childScoreColor(item.children[index]),
              ),
            ],
            const SizedBox(height: 8),
            Row(
              children: [
                _MiniActionButton(
                  label: 'AGREGAR SUBNOTA',
                  icon: Icons.add_rounded,
                  onPressed: onAddChild,
                ),
                const SizedBox(width: 8),
                _MiniActionButton(
                  label: 'ELIMINAR',
                  icon: Icons.remove_rounded,
                  onPressed: onRemoveChild,
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

class _ConfidenceSwitch extends StatelessWidget {
  const _ConfidenceSwitch({required this.value, required this.onChanged});

  final int value;
  final ValueChanged<int> onChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [1, 2, 3].map((v) {
        final selected = v == value;
        return Padding(
          padding: EdgeInsets.only(left: v == 1 ? 0 : 3),
          child: InkWell(
            onTap: () => onChanged(v),
            borderRadius: BorderRadius.circular(4),
            child: Container(
              width: 26,
              height: 30,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: selected ? AppColors.text : AppColors.surface,
                border: Border.all(color: AppColors.border, width: 1.5),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                '$v',
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w900,
                  color: selected ? AppColors.surface : AppColors.text,
                ),
              ),
            ),
          ),
        );
      }).toList(),
    );
  }
}

class _ChildGradeRow extends StatelessWidget {
  const _ChildGradeRow({
    super.key,
    required this.label,
    required this.item,
    required this.onScoreChanged,
    required this.onWeightChanged,
    required this.onConfidenceChanged,
    required this.scoreColor,
  });

  final String label;
  final _GradeItem item;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final ValueChanged<int> onConfidenceChanged;
  final Color scoreColor;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ),
            _ConfidenceSwitch(
              value: item.confidence,
              onChanged: onConfidenceChanged,
            ),
          ],
        ),
        const SizedBox(height: AppDimens.spaceXs),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              flex: 2,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('NOTA', style: _fieldLabelStyle),
                  const SizedBox(height: AppDimens.spaceXs),
                  _NumericInput(
                    controller: item.scoreController,
                    hint: 'Nota',
                    onChanged: onScoreChanged,
                    textColor: scoreColor,
                  ),
                ],
              ),
            ),
            const SizedBox(width: AppDimens.spaceSm),
            SizedBox(
              width: 80,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('PESO %', style: _fieldLabelStyle),
                  const SizedBox(height: AppDimens.spaceXs),
                  _NumericInput(
                    controller: item.weightController,
                    hint: '%',
                    integer: true,
                    onChanged: onWeightChanged,
                  ),
                ],
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _ScoreAndWeightRow extends StatelessWidget {
  const _ScoreAndWeightRow({
    required this.scoreController,
    required this.weightController,
    required this.scoreHint,
    required this.onScoreChanged,
    required this.onWeightChanged,
    required this.confidence,
    required this.onConfidenceChanged,
    required this.scoreColor,
  });

  final TextEditingController scoreController;
  final TextEditingController weightController;
  final String scoreHint;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final int confidence;
  final ValueChanged<int> onConfidenceChanged;
  final Color scoreColor;

  @override
  Widget build(BuildContext context) {
    const labelStyle = TextStyle(
      fontSize: 9.5,
      fontWeight: FontWeight.w900,
      letterSpacing: 0.4,
      color: AppColors.mutedStrong,
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('NOTA', style: labelStyle),
                  const SizedBox(height: AppDimens.spaceXs),
                  _NumericInput(
                    controller: scoreController,
                    hint: scoreHint,
                    onChanged: onScoreChanged,
                    textColor: scoreColor,
                  ),
                ],
              ),
            ),
            const SizedBox(width: AppDimens.spaceSm),
            SizedBox(
              width: 120,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('PONDERACIÓN (%)', style: labelStyle),
                  const SizedBox(height: AppDimens.spaceXs),
                  _NumericInput(
                    controller: weightController,
                    hint: '%',
                    integer: true,
                    onChanged: onWeightChanged,
                  ),
                ],
              ),
            ),
          ],
        ),
        const SizedBox(height: AppDimens.spaceSm),
        Row(
          children: [
            const Expanded(child: Text('CONFIANZA', style: labelStyle)),
            _ConfidenceSwitch(
              value: confidence,
              onChanged: onConfidenceChanged,
            ),
          ],
        ),
      ],
    );
  }
}

class _NumericInput extends StatelessWidget {
  const _NumericInput({
    required this.controller,
    required this.hint,
    required this.onChanged,
    this.integer = false,
    this.textColor = AppColors.text,
  });

  final TextEditingController controller;
  final String hint;
  final VoidCallback onChanged;
  final bool integer;
  final Color textColor;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      keyboardType: integer
          ? TextInputType.number
          : const TextInputType.numberWithOptions(decimal: true),
      inputFormatters: [FilteringTextInputFormatter.allow(_numberRegex)],
      style: TextStyle(
        fontWeight: FontWeight.w800,
        fontSize: 13,
        color: textColor,
      ),
      decoration: appInputDecoration(hint).copyWith(
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: AppDimens.spaceMd,
          vertical: AppDimens.spaceSm + AppDimens.borderWidth,
        ),
        hintStyle: const TextStyle(
          color: AppColors.muted,
          fontWeight: FontWeight.w700,
          fontSize: 12,
        ),
      ),
      onChanged: (_) => onChanged(),
    );
  }
}

class _WeightSummary extends StatelessWidget {
  const _WeightSummary({
    required this.label,
    required this.value,
    required this.isComplete,
  });

  final String label;
  final double value;
  final bool isComplete;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
      decoration: BoxDecoration(
        color: isComplete ? AppColors.bg : AppColors.surfaceLow,
        border: Border.all(
          color: isComplete ? AppColors.border : AppColors.error,
          width: 2,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Row(
        children: [
          Icon(
            isComplete
                ? Icons.check_circle_rounded
                : Icons.warning_amber_rounded,
            size: 18,
            color: isComplete ? AppColors.text : AppColors.error,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              label,
              style: const TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          Text(
            '${value.toStringAsFixed(0)}%',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w900,
              color: isComplete ? AppColors.text : AppColors.error,
            ),
          ),
        ],
      ),
    );
  }
}

class _MiniActionButton extends StatelessWidget {
  const _MiniActionButton({
    required this.label,
    required this.icon,
    required this.onPressed,
  });

  final String label;
  final IconData icon;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: const BoxConstraints(minHeight: 44, minWidth: 44),
      child: InkWell(
        onTap: onPressed,
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        child: Container(
          padding: const EdgeInsets.symmetric(
            horizontal: AppDimens.spaceSm,
            vertical: AppDimens.spaceSm,
          ),
          decoration: BoxDecoration(
            color: AppColors.bg,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 15, color: AppColors.text),
              const SizedBox(width: AppDimens.spaceXs),
              Text(
                label,
                style: const TextStyle(
                  fontSize: 9.5,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

const TextStyle _fieldLabelStyle = TextStyle(
  fontSize: 9.5,
  fontWeight: FontWeight.w900,
  letterSpacing: 0.4,
  color: AppColors.mutedStrong,
);

class _ResultCard extends StatelessWidget {
  const _ResultCard({
    required this.background,
    required this.foreground,
    required this.title,
    required this.message,
    this.value,
    this.simulated,
    this.target,
    this.official,
  });

  final Color background;
  final Color foreground;
  final String title;
  final String message;
  final String? value;
  final String? simulated;
  final String? target;
  final String? official;

  @override
  Widget build(BuildContext context) {
    final stats = <Widget>[
      if (simulated != null) _ResultStat(label: 'Simulado', value: simulated!),
      if (target != null) _ResultStat(label: 'Meta', value: target!),
      if (official != null) _ResultStat(label: 'Oficial', value: official!),
    ];

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(AppDimens.spaceXl),
      decoration: BoxDecoration(
        color: background,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidthThick,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.dialog,
      ),
      child: Column(
        children: [
          Text(
            title,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
              color: foreground,
            ),
          ),
          if (value != null) ...[
            const SizedBox(height: AppDimens.spaceSm),
            Text(
              value!,
              style: TextStyle(
                fontSize: 52,
                fontWeight: FontWeight.w900,
                height: 1,
                color: foreground,
              ),
            ),
          ],
          if (stats.isNotEmpty) ...[
            const SizedBox(height: AppDimens.spaceLg),
            Wrap(
              alignment: WrapAlignment.center,
              spacing: AppDimens.spaceSm,
              runSpacing: AppDimens.spaceSm,
              children: stats,
            ),
          ],
          const SizedBox(height: AppDimens.spaceMd),
          Text(
            message,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w700,
              color: foreground,
              height: 1.35,
            ),
          ),
        ],
      ),
    );
  }
}

class _ResultStat extends StatelessWidget {
  const _ResultStat({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceMd,
        vertical: AppDimens.spaceSm,
      ),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            '$label ',
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.4,
              color: AppColors.mutedStrong,
            ),
          ),
          Text(
            value,
            style: const TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ],
      ),
    );
  }
}

class _SquareIconButton extends StatelessWidget {
  const _SquareIconButton({
    required this.icon,
    required this.onPressed,
    this.tooltip,
  });

  final IconData icon;
  final VoidCallback onPressed;
  final String? tooltip;

  @override
  Widget build(BuildContext context) {
    final button = InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onPressed,
      child: Container(
        width: 42,
        height: 42,
        alignment: Alignment.center,
        decoration: _cardDecoration(
          color: AppColors.surface,
          shadow: const Offset(2, 2),
        ),
        child: Icon(icon, size: 18, color: AppColors.text),
      ),
    );

    if (tooltip == null) return button;
    return Tooltip(message: tooltip!, child: button);
  }
}

BoxDecoration _cardDecoration({required Color color, required Offset shadow}) {
  return BoxDecoration(
    color: color,
    border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
    borderRadius: BorderRadius.circular(AppDimens.radius),
    boxShadow: [
      BoxShadow(color: AppColors.border, offset: shadow, blurRadius: 0),
    ],
  );
}
