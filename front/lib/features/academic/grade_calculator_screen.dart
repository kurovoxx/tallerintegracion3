
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';

enum _ResultState { none, success, invalid }

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

/// Los niveles 1/2/3 vienen del comportamiento de la calculadora de
/// referencia. Estos factores son locales hasta conectar el endpoint real.
double confidenceFactor(int confidence) {
  switch (confidence) {
    case 1:
      return 0.60;
    case 2:
      return 0.80;
    case 3:
      return 1.00;
    default:
      return 0.80;
  }
}

double? _parseNumber(String raw) {
  final normalized = raw.trim().replaceAll(',', '.');
  if (normalized.isEmpty) return null;
  return double.tryParse(normalized);
}

final RegExp _numberRegex = RegExp(r'^\d{0,3}([.,]\d{0,2})?$');

class _GradeItem {
  _GradeItem({
    required this.id,
    this.confidence = 2,
  })  : scoreController = TextEditingController(),
        weightController = TextEditingController(),
        children = [];

  final String id;
  final TextEditingController scoreController;
  final TextEditingController weightController;
  int confidence;
  List<_GradeItem> children;

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
    required this.confidence,
  });

  final String label;
  final double? score;
  final double weight;
  final int confidence;
}

class GradeCalculatorScreen extends StatefulWidget {
  const GradeCalculatorScreen({super.key});

  @override
  State<GradeCalculatorScreen> createState() => _GradeCalculatorScreenState();
}

class _GradeCalculatorScreenState extends State<GradeCalculatorScreen> {
  final List<String> _courses = const [
    'Cálculo III',
    'Estructuras de Datos',
    'Física General II',
    'Ingeniería de Software',
  ];

  late String _selectedCourse;
  final TextEditingController _targetController =
      TextEditingController(text: '4.0');

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

  @override
  void initState() {
    super.initState();
    _selectedCourse = _courses.first;
    _initializeNormalGrades();
  }

  @override
  void dispose() {
    _targetController.dispose();
    _disposeNormalGrades();
    _finalGrade?.dispose();
    _exam?.dispose();
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
  }

  void _resetCalculator() {
    setState(() {
      _resetResult();

      if (_finalExamMode) {
        _finalGrade?.dispose();
        _exam?.dispose();
        _finalGrade = _newItem('final', confidence: 2, weight: 70);
        _exam = _newItem('exam', confidence: 1, weight: 30);
      } else {
        _initializeNormalGrades();
      }
    });
  }

  _GradeItem _newItem(
    String id, {
    int confidence = 2,
    double? weight,
  }) {
    final item = _GradeItem(id: id, confidence: confidence);
    if (weight != null) {
      item.weightController.text = _formatWeight(weight);
    }
    return item;
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

        _finalGrade = _newItem('final', confidence: 2, weight: 70);
        _exam = _newItem('exam', confidence: 1, weight: 30);
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
        finalItem.weightController.text = _formatWeight(finalWeight);
        examItem.weightController.text = _formatWeight(100 - finalWeight);
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
        examItem.weightController.text = _formatWeight(examWeight);
        finalItem.weightController.text = _formatWeight(100 - examWeight);
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
      changed.weightController.text = _formatWeight(changedWeight);

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
          item.weightController.text = _formatWeight(equalWeight);
        }
      } else {
        for (final item in otherGrades) {
          final proportionalWeight = remaining * _readWeight(item) / previousTotal;
          item.weightController.text = _formatWeight(proportionalWeight);
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
        changed.weightController.text = _formatWeight(changedWeight);

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
            item.weightController.text = _formatWeight(equalWeight);
          }
        } else {
          for (final item in others) {
            item.weightController.text = _formatWeight(
              remaining * _readWeight(item) / previousTotal,
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
      final firstChild = _newItem(
        '${parent.id}-child-1',
        confidence: parent.confidence,
        weight: 50,
      );
      final secondChild = _newItem(
        '${parent.id}-child-2',
        confidence: parent.confidence,
        weight: 50,
      );

      parent.children = [firstChild, secondChild];
      parent.scoreController.clear();
      parent.weightController.clear();
      _resetResult();
    });
  }

  void _addChild(_GradeItem parent) {
    if (_finalExamMode) return;

    setState(() {
      parent.children.add(
        _newItem(
          '${parent.id}-child-${DateTime.now().microsecondsSinceEpoch}',
          confidence: parent.confidence,
        ),
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
        parent.weightController.text = survivor.weightController.text;
        parent.confidence = survivor.confidence;
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

  List<_CalcEntry> _buildEntries() {
    final entries = <_CalcEntry>[];

    void flatten(
      _GradeItem item,
      String label,
      double globalWeight,
    ) {
      if (item.isSubdivided) {
        for (var index = 0; index < item.children.length; index++) {
          final child = item.children[index];
          final childFraction = _readWeight(child) / 100;
          flatten(
            child,
            '$label.${index + 1}',
            globalWeight * childFraction,
          );
        }
        return;
      }

      entries.add(
        _CalcEntry(
          label: label,
          score: _parseNumber(item.scoreController.text),
          weight: globalWeight,
          confidence: item.confidence,
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
    return _buildEntries().fold<double>(
      0,
      (sum, entry) => sum + entry.weight,
    );
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

    final entries = _buildEntries();
    final knownEntries = entries.where((entry) => entry.score != null);
    final pendingEntries = entries.where(
      (entry) => entry.score == null && entry.weight > 0,
    );

    double weightedKnown = 0;
    for (final entry in knownEntries) {
      weightedKnown += entry.score! *
          entry.weight *
          confidenceFactor(entry.confidence);
    }

    final pendingWeight = pendingEntries.fold<double>(
      0,
      (sum, entry) => sum + entry.weight,
    );

    setState(() {
      _average = roundHalfUp(weightedKnown, 1);
      _error = null;
      _resultState = _ResultState.success;

      if (pendingWeight == 0) {
        _neededPendingGrade = null;
        _isReachable = weightedKnown >= target;
      } else {
        final needed = (target - weightedKnown) / pendingWeight;
        _neededPendingGrade = roundHalfUp(needed, 1);
        _isReachable = needed <= 7;
      }
    });
  }

  void _showError(String message) {
    setState(() {
      _error = message;
      _average = null;
      _neededPendingGrade = null;
      _resultState = _ResultState.invalid;
    });
  }

  // ---------------------------------------------------------------------
  // SCREEN LAYOUT
  // ---------------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: EdgeInsets.symmetric(
            horizontal: isDesktop ? 40 : 18,
            vertical: 22,
          ),
          child: Center(
            child: ConstrainedBox(
              constraints: BoxConstraints(maxWidth: isDesktop ? 1040 : 640),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildHeader(),
                  const SizedBox(height: 20),
                  _buildConfiguration(isDesktop),
                  const SizedBox(height: 18),
                  _finalExamMode
                      ? _buildFinalExamMode()
                      : _buildNormalMode(isDesktop),
                  const SizedBox(height: 18),
                  _buildWeightSummary(),
                  const SizedBox(height: 20),
                  _buildActions(),
                  const SizedBox(height: 22),
                  if (_resultState != _ResultState.none) _buildResult(),
                ],
              ),
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
            flex: 2,
            child: targetCard,
          ),
          const SizedBox(width: 16),
          Expanded(
            flex: 3,
            child: modeCard,
          ),
        ],
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        targetCard,
        const SizedBox(height: 16),
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
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 14,
          ),
        ),
        const SizedBox(height: 10),
        _FinalGradeCard(
          item: finalItem,
          onScoreChanged: () => setState(_resetResult),
          onWeightChanged: _changeFinalWeight,
          onConfidenceChanged: (confidence) {
            setState(() {
              finalItem.confidence = confidence;
              _resetResult();
            });
          },
        ),
        const SizedBox(height: 12),
        _ExamCard(
          item: examItem,
          onScoreChanged: () => setState(_resetResult),
          onWeightChanged: _changeExamWeight,
          onConfidenceChanged: (confidence) {
            setState(() {
              examItem.confidence = confidence;
              _resetResult();
            });
          },
        ),
      ],
    );
  }

  Widget _buildNormalMode(bool isDesktop) {
    final columns = isDesktop ? 2 : 1;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            const Expanded(
              child: Text(
                'NOTAS DE LA UNIVERSIDAD',
                style: TextStyle(
                  fontWeight: FontWeight.w900,
                  fontSize: 14,
                ),
              ),
            ),
            _SmallActionButton(
              label: 'AGREGAR NOTA',
              icon: Icons.add_rounded,
              onPressed: _addNormalGrade,
            ),
          ],
        ),
        const SizedBox(height: 10),
        LayoutBuilder(
          builder: (context, constraints) {
            final width = columns == 1
                ? constraints.maxWidth
                : (constraints.maxWidth - 14) / 2;

            return Wrap(
              spacing: 14,
              runSpacing: 14,
              children: _normalGrades.map((item) {
                final index = _normalGrades.indexOf(item);

                return SizedBox(
                  width: width,
                  child: _NormalGradeCard(
                    key: ValueKey(item.id),
                    item: item,
                    label: 'NOTA ${index + 1}',
                    onScoreChanged: () => setState(_resetResult),
                    onWeightChanged: () => _changeNormalWeight(item),
                    onConfidenceChanged: (confidence) {
                      setState(() {
                        item.confidence = confidence;
                        _resetResult();
                      });
                    },
                    onSubdivide: () => _subdivideNormalGrade(item),
                    onChildScoreChanged: () => setState(_resetResult),
                    onChildWeightChanged: (child) =>
                        _changeChildWeight(item, child),
                    onAddChild: () => _addChild(item),
                    onRemoveChild: () => _removeChild(item),
                  ),
                );
              }).toList(),
            );
          },
        ),
        const SizedBox(height: 8),
        Align(
          alignment: Alignment.centerLeft,
          child: TextButton.icon(
            onPressed: _removeLastNormalGrade,
            icon: const Icon(Icons.remove_circle_outline_rounded),
            label: const Text('ELIMINAR ÚLTIMA'),
          ),
        ),
      ],
    );
  }

  Widget _buildWeightSummary() {
    final value = _totalWeight() * 100;

    return _WeightSummary(
      label: 'PONDERACIÓN TOTAL',
      value: value,
      isComplete: (value - 100).abs() <= 0.5,
    );
  }

  Widget _buildActions() {
    return Row(
      children: [
        Expanded(
          child: OutlinedButton.icon(
            onPressed: _resetCalculator,
            icon: const Icon(Icons.restart_alt_rounded),
            label: const Text('RESET'),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          flex: 2,
          child: SubmitButton(
            text: 'CALCULAR PROMEDIO',
            onPressed: _calculate,
          ),
        ),
      ],
    );
  }

  Widget _buildResult() {
    if (_resultState == _ResultState.invalid) {
      return _ResultCard(
        background: const Color(0xFFFFE1DE),
        foreground: AppColors.error,
        title: 'ERROR',
        message: _error ?? 'Revisa los datos.',
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
        );
      }

      return _ResultCard(
        background: const Color(0xFFFFE1DE),
        foreground: AppColors.error,
        title: 'META NO ALCANZABLE',
        message: 'La nota necesaria supera 7.0.',
      );
    }

    final average = _average ?? 0;
    final passed = average >= _passingGrade;

    return _ResultCard(
      background: passed ? AppColors.accentYellow : const Color(0xFFFFE1DE),
      foreground: passed ? AppColors.text : AppColors.error,
      title: passed ? 'PROMEDIO CALCULADO' : 'REPROBASTE EL RAMO',
      value: average.toStringAsFixed(1),
      message: passed
          ? 'Aprobado con promedio igual o superior a 4.0.'
          : 'Promedio inferior a 4.0.',
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
  const _TargetCard({
    required this.controller,
    required this.onChanged,
  });

  final TextEditingController controller;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: _cardDecoration(
        color: AppColors.surface,
        shadow: const Offset(4, 4),
      ),
      child: Row(
        children: [
          const Expanded(
            child: Text(
              'QUIERO UN',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 14,
              ),
            ),
          ),
          SizedBox(
            width: 100,
            child: _NumericInput(
              controller: controller,
              hint: '4.0',
              onChanged: onChanged,
            ),
          ),
        ],
      ),
    );
  }
}

class _ModeSwitchCard extends StatelessWidget {
  const _ModeSwitchCard({
    required this.enabled,
    required this.onChanged,
  });

  final bool enabled;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: _cardDecoration(
        color: AppColors.accentYellow,
        shadow: const Offset(3, 3),
      ),
      child: Row(
        children: [
          const Expanded(
            child: Text(
              'SOLO NOTA FINAL Y EXAMEN',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 13,
              ),
            ),
          ),
          Switch(
            value: enabled,
            activeColor: AppColors.text,
            onChanged: onChanged,
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
  });

  final _GradeItem item;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final ValueChanged<int> onConfidenceChanged;

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
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 14,
            ),
          ),
          const SizedBox(height: 8),
          _ScoreAndWeightRow(
            scoreController: item.scoreController,
            weightController: item.weightController,
            scoreHint: 'Promedio',
            onScoreChanged: onScoreChanged,
            onWeightChanged: onWeightChanged,
          ),
          const SizedBox(height: 8),
          _ConfidenceSelector(
            value: item.confidence,
            onChanged: onConfidenceChanged,
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
  });

  final _GradeItem item;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final ValueChanged<int> onConfidenceChanged;

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
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 14,
            ),
          ),
          const SizedBox(height: 8),
          _ScoreAndWeightRow(
            scoreController: item.scoreController,
            weightController: item.weightController,
            scoreHint: 'Nota examen',
            onScoreChanged: onScoreChanged,
            onWeightChanged: onWeightChanged,
          ),
          const SizedBox(height: 8),
          _ConfidenceSelector(
            value: item.confidence,
            onChanged: onConfidenceChanged,
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
    required this.onConfidenceChanged,
    required this.onSubdivide,
    required this.onChildScoreChanged,
    required this.onChildWeightChanged,
    required this.onAddChild,
    required this.onRemoveChild,
  });

  final _GradeItem item;
  final String label;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;
  final ValueChanged<int> onConfidenceChanged;
  final VoidCallback onSubdivide;
  final VoidCallback onChildScoreChanged;
  final ValueChanged<_GradeItem> onChildWeightChanged;
  final VoidCallback onAddChild;
  final VoidCallback onRemoveChild;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: _cardDecoration(
        color: item.isSubdivided
            ? const Color(0xFFD6E3FF)
            : AppColors.surface,
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
                    fontSize: 11,
                    fontWeight: FontWeight.w900,
                    color: AppColors.muted,
                  ),
                ),
              ),
              if (item.isSubdivided)
                const Text(
                  'SUBDIVIDIDA',
                  style: TextStyle(
                    fontSize: 9,
                    fontWeight: FontWeight.w900,
                    color: AppColors.accentBlue,
                  ),
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
            ),
            const SizedBox(height: 8),
            _ConfidenceSelector(
              value: item.confidence,
              onChanged: onConfidenceChanged,
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
            for (var index = 0; index < item.children.length; index++) ...[
              if (index != 0) const SizedBox(height: 6),
              _ChildGradeRow(
                key: ValueKey(item.children[index].id),
                label: 'SUBNOTA ${index + 1}',
                item: item.children[index],
                onScoreChanged: onChildScoreChanged,
                onWeightChanged: () {
                  onChildWeightChanged(item.children[index]);
                },
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

class _ChildGradeRow extends StatelessWidget {
  const _ChildGradeRow({
    super.key,
    required this.label,
    required this.item,
    required this.onScoreChanged,
    required this.onWeightChanged,
  });

  final String label;
  final _GradeItem item;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
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
        Expanded(
          flex: 2,
          child: _NumericInput(
            controller: item.scoreController,
            hint: 'Nota',
            onChanged: onScoreChanged,
          ),
        ),
        const SizedBox(width: 6),
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
  });

  final TextEditingController scoreController;
  final TextEditingController weightController;
  final String scoreHint;
  final VoidCallback onScoreChanged;
  final VoidCallback onWeightChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: _NumericInput(
            controller: scoreController,
            hint: scoreHint,
            onChanged: onScoreChanged,
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 78,
          child: _NumericInput(
            controller: weightController,
            hint: '%',
            integer: true,
            onChanged: onWeightChanged,
          ),
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
  });

  final TextEditingController controller;
  final String hint;
  final VoidCallback onChanged;
  final bool integer;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      keyboardType: integer
          ? TextInputType.number
          : const TextInputType.numberWithOptions(decimal: true),
      inputFormatters: [
        FilteringTextInputFormatter.allow(_numberRegex),
      ],
      style: const TextStyle(
        fontWeight: FontWeight.w800,
        fontSize: 13,
        color: AppColors.text,
      ),
      decoration: appInputDecoration(hint),
      onChanged: (_) => onChanged(),
    );
  }
}

class _ConfidenceSelector extends StatelessWidget {
  const _ConfidenceSelector({
    required this.value,
    required this.onChanged,
  });

  final int value;
  final ValueChanged<int> onChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Text(
          'CONFIANZA',
          style: TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w900,
            color: AppColors.muted,
          ),
        ),
        const SizedBox(width: 8),
        for (final level in [1, 2, 3]) ...[
          if (level != 1) const SizedBox(width: 4),
          GestureDetector(
            onTap: () => onChanged(level),
            child: Container(
              width: 28,
              height: 25,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: value == level
                    ? AppColors.accentBlue
                    : AppColors.bg,
                border: Border.all(color: AppColors.border, width: 1.5),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                '$level',
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w900,
                  color: value == level ? Colors.white : AppColors.text,
                ),
              ),
            ),
          ),
        ],
      ],
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
        color: isComplete ? AppColors.bg : const Color(0xFFFFE1DE),
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

class _SmallActionButton extends StatelessWidget {
  const _SmallActionButton({
    required this.label,
    required this.icon,
    required this.onPressed,
  });

  final String label;
  final IconData icon;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onPressed,
      borderRadius: BorderRadius.circular(4),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 7),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border, width: 1.5),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14),
            const SizedBox(width: 4),
            Text(
              label,
              style: const TextStyle(
                fontSize: 9,
                fontWeight: FontWeight.w900,
              ),
            ),
          ],
        ),
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
    return InkWell(
      onTap: onPressed,
      borderRadius: BorderRadius.circular(4),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 6),
        decoration: BoxDecoration(
          color: AppColors.bg,
          border: Border.all(color: AppColors.border, width: 1.5),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 13),
            const SizedBox(width: 3),
            Text(
              label,
              style: const TextStyle(
                fontSize: 8.5,
                fontWeight: FontWeight.w900,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ResultCard extends StatelessWidget {
  const _ResultCard({
    required this.background,
    required this.foreground,
    required this.title,
    required this.message,
    this.value,
  });

  final Color background;
  final Color foreground;
  final String title;
  final String message;
  final String? value;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: _cardDecoration(
        color: background,
        shadow: const Offset(4, 4),
      ),
      child: Column(
        children: [
          Text(
            title,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w900,
              color: foreground,
            ),
          ),
          if (value != null) ...[
            const SizedBox(height: 8),
            Text(
              value!,
              style: TextStyle(
                fontSize: 46,
                fontWeight: FontWeight.w900,
                color: foreground,
              ),
            ),
          ],
          const SizedBox(height: 8),
          Text(
            message,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w700,
              color: foreground,
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

BoxDecoration _cardDecoration({
  required Color color,
  required Offset shadow,
}) {
  return BoxDecoration(
    color: color,
    border: Border.all(
      color: AppColors.border,
      width: AppDimens.borderWidth,
    ),
    borderRadius: BorderRadius.circular(AppDimens.radius),
    boxShadow: [
      BoxShadow(
        color: AppColors.border,
        offset: shadow,
        blurRadius: 0,
      ),
    ],
  );
}
