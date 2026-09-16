import 'package:flutter/material.dart';
import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';

/// RF-13, RF-14 / Regla 2.5
/// Cálculo oficial: % asistencia = 1 - (COUNT(absences no justificadas) / subjects.total_classes)
/// - justified:true NO resta
/// - total_classes null/0 => "No configurado"

double? _computeAttendance({required int? totalClasses, required int unjustified}) {
  if (totalClasses == null || totalClasses <= 0) return null;
  final ratio = 1 - (unjustified / totalClasses);
  return (ratio * 100).clamp(0, 100).toDouble();
}

/// Wrapper público requerido por spec (unjustifiedAbsences)
double? computeAttendance({required int? totalClasses, required int unjustifiedAbsences}) {
  return _computeAttendance(totalClasses: totalClasses, unjustified: unjustifiedAbsences);
}

int? _remainingAbsences({required int? totalClasses, required int? minPct, required int unjustified}) {
  if (totalClasses == null || totalClasses <= 0 || minPct == null) return null;
  final maxUnjustified = (totalClasses * (1 - minPct / 100)).floor();
  return maxUnjustified - unjustified;
}

String _formatDate(DateTime d) {
  final dd = d.day.toString().padLeft(2, '0');
  final mm = d.month.toString().padLeft(2, '0');
  return '$dd/$mm';
}

String _formatDateFull(DateTime d) {
  final dd = d.day.toString().padLeft(2, '0');
  final mm = d.month.toString().padLeft(2, '0');
  return '$dd/$mm/${d.year}';
}

String _weekdayLabel(DateTime d) {
  const labels = ['Lunes', 'Martes', 'Miércoles', 'Jueves', 'Viernes', 'Sábado', 'Domingo'];
  return labels[d.weekday - 1];
}

const TextStyle _headerStyle = TextStyle(
  fontWeight: FontWeight.w900,
  fontSize: 11,
  letterSpacing: 0.5,
  color: AppColors.muted,
);

class AttendanceScreen extends StatefulWidget {
  final String? initialSubjectId;
  const AttendanceScreen({super.key, this.initialSubjectId});

  @override
  State<AttendanceScreen> createState() => _AttendanceScreenState();
}

class _AttendanceScreenState extends State<AttendanceScreen> {
  bool _isLoading = true;
  bool _hasError = false;

  final ScrollController _scrollController = ScrollController();

  // filtro rápido general: Todos / Críticos
  String _filterMode = 'todos'; // 'todos' | 'criticos'
  String? _selectedSubjectId;

  // mock semestre / programa (solo visual, replica pmn)
  String _selectedSemester = '2026 - Semestre 2';
  String _selectedProgram = 'Ingeniería Civil en Informática';

  // Datos mock que respetan MER — estado vivo centralizado
  List<Map<String, dynamic>> _subjects = [];
  List<Map<String, dynamic>> _absences = [];

  // config editable por asignatura en vista específica
  final Map<String, Map<String, dynamic>> _subjectConfigs = {};

  @override
  void initState() {
    super.initState();
    _selectedSubjectId = widget.initialSubjectId;
    _loadData();
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  Future<void> _loadData() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
    });
    try {
      await Future.delayed(const Duration(milliseconds: 650));

      _subjects = [
        {
          'id': 'arq-hw',
          'name': 'Arquitectura de Hardware',
          'code': 'INF-210',
          'total_classes': 32,
          'min_attendance_pct': 75,
          'professor': 'Prof. Soto',
          'credits': 5,
          'days_per_week': 2,
          'weeks': 16,
          'absences': <Map<String, dynamic>>[],
        },
        {
          'id': 'calc-3',
          'name': 'Cálculo III',
          'code': 'INF-1111',
          'total_classes': 40,
          'min_attendance_pct': 75,
          'professor': 'Prof. 1',
          'credits': 5,
          'days_per_week': 2,
          'weeks': 20,
          'absences': <Map<String, dynamic>>[],
        },
        {
          'id': 'taller-3',
          'name': 'Taller de Integración III',
          'code': 'INF-360',
          'total_classes': 28,
          'min_attendance_pct': 70,
          'professor': 'Prof. 2',
          'credits': 4,
          'days_per_week': 2,
          'weeks': 14,
          'absences': <Map<String, dynamic>>[],
        },
        {
          'id': 'redes',
          'name': 'Redes de Computadores',
          'code': 'INF-330',
          'total_classes': 30,
          'min_attendance_pct': 75,
          'professor': 'Profesora Morales',
          'credits': 5,
          'days_per_week': 2,
          'weeks': 15,
          'absences': <Map<String, dynamic>>[],
        },
        {
          'id': 'seg-inf',
          'name': 'Seguridad Informática',
          'code': 'INF-350',
          'total_classes': null,
          'min_attendance_pct': 75,
          'professor': 'Prof. de la Vega',
          'credits': 4,
          'days_per_week': 2,
          'weeks': 16,
          'absences': <Map<String, dynamic>>[],
        },
      ];

      _absences = [
        {'id': 'abs-1', 'subject_id': 'arq-hw', 'date': DateTime(2026, 4, 12), 'justified': false, 'source': 'self', 'type': 'Teoría'},
        {'id': 'abs-2', 'subject_id': 'arq-hw', 'date': DateTime(2026, 4, 19), 'justified': false, 'source': 'self', 'type': 'Teoría'},
        {'id': 'abs-3', 'subject_id': 'arq-hw', 'date': DateTime(2026, 4, 26), 'justified': false, 'source': 'teacher', 'type': 'Lab'},
        {'id': 'abs-4', 'subject_id': 'arq-hw', 'date': DateTime(2026, 5, 3), 'justified': false, 'source': 'self', 'type': 'Teoría'},
        {'id': 'abs-5', 'subject_id': 'arq-hw', 'date': DateTime(2026, 5, 10), 'justified': true, 'source': 'teacher', 'type': 'Teoría'},
        {'id': 'abs-6', 'subject_id': 'arq-hw', 'date': DateTime(2026, 5, 17), 'justified': false, 'source': 'self', 'type': 'Teoría'},
        {'id': 'abs-7', 'subject_id': 'arq-hw', 'date': DateTime(2026, 3, 28), 'justified': true, 'source': 'teacher', 'type': 'Lab'},
        {'id': 'abs-8', 'subject_id': 'arq-hw', 'date': DateTime(2026, 4, 5), 'justified': false, 'source': 'self', 'type': 'Teoría'},
        {'id': 'abs-9', 'subject_id': 'calc-3', 'date': DateTime(2026, 4, 5), 'justified': true, 'source': 'teacher', 'type': 'Lab'},
        {'id': 'abs-10', 'subject_id': 'calc-3', 'date': DateTime(2026, 5, 2), 'justified': false, 'source': 'self', 'type': 'Teoría'},
        {'id': 'abs-11', 'subject_id': 'calc-3', 'date': DateTime(2026, 11, 2), 'justified': false, 'source': 'self', 'type': 'Clase regular'},
        {'id': 'abs-12', 'subject_id': 'taller-3', 'date': DateTime(2026, 3, 15), 'justified': true, 'source': 'teacher', 'type': 'Taller'},
        {'id': 'abs-13', 'subject_id': 'redes', 'date': DateTime(2026, 4, 8), 'justified': true, 'source': 'teacher', 'type': 'Teoría'},
      ];

      // configs por defecto sincronizados con subjects
      for (final s in _subjects) {
        final id = s['id'] as String;
        _subjectConfigs[id] = {
          'daysPerWeek': s['days_per_week'] as int? ?? 2,
          'weeks': s['weeks'] as int? ?? 16,
          'minPct': (s['min_attendance_pct'] as int?) ?? 75,
        };
      }

      // Centralizar inasistencias dentro de cada asignatura para reactividad
      for (final s in _subjects) {
        final sid = s['id'] as String;
        s['absences'] = _absences.where((a) => a['subject_id'] == sid).toList();
      }

      // validar initialSubjectId: si no existe en _subjects, volver a vista general
      if (_selectedSubjectId != null && !_subjects.any((s) => s['id'] == _selectedSubjectId)) {
        _selectedSubjectId = null;
      }

      if (!mounted) return;
      setState(() => _isLoading = false);
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _hasError = true;
      });
    }
  }

  Map<String, dynamic>? _subjectById(String id) {
    try {
      return _subjects.firstWhere((s) => s['id'] == id);
    } catch (_) {
      return null;
    }
  }

  List<Map<String, dynamic>> _absencesFor(String subjectId) {
    return _absences.where((a) => a['subject_id'] == subjectId).toList()
      ..sort((a, b) => (b['date'] as DateTime).compareTo(a['date'] as DateTime));
  }

  // ignore: unused_element
  int _unjustifiedCount(String subjectId) {
    return _absencesFor(subjectId).where((a) => a['justified'] == false).length;
  }

  bool _isCritical(Map<String, dynamic> subject) {
    final total = subject['total_classes'] as int?;
    final absList = subject['absences'] as List? ?? _absences.where((a) => a['subject_id'] == subject['id']).toList();
    final unjust = absList.where((a) => a['justified'] != true).length;
    final pct = _computeAttendance(totalClasses: total, unjustified: unjust);
    final minPct = (subject['min_attendance_pct'] as num?)?.toDouble() ?? (_subjectConfigs[subject['id']] != null ? (_subjectConfigs[subject['id']]!['minPct'] as num).toDouble() : 75.0);
    if (pct == null) return false;
    final maxAllowed = total != null ? (total * (1 - (minPct / 100))).floor() : 0;
    final remaining = maxAllowed - unjust;
    return pct < minPct || remaining <= 1;
  }

  // Getter puro reactivo — incluye lógica de remainingAbsences <=1
  List<Map<String, dynamic>> get _criticalSubjects {
    return _subjects.where((s) {
      final total = s['total_classes'] as int?;
      final absences = s['absences'] as List? ?? _absences.where((a) => a['subject_id'] == s['id']).toList();
      final unjustified = absences.where((a) => a['justified'] != true).length;
      final pct = computeAttendance(totalClasses: total, unjustifiedAbsences: unjustified);
      final minPct = (s['min_attendance_pct'] as num?)?.toDouble() ?? 75.0;
      if (pct == null) return false;
      final maxAllowedUnjustified = total != null ? (total * (1 - (minPct / 100))).floor() : 0;
      final remainingAbsences = maxAllowedUnjustified - unjustified;
      return pct < minPct || remainingAbsences <= 1;
    }).toList();
  }

  // ignore: unused_element
  int get _criticalCount => _criticalSubjects.length;

  void _updateSubjectAttendance({
    required String subjectId,
    required int totalClasses,
    required double minAttendancePct,
    List<Map<String, dynamic>>? newAbsences,
  }) {
    setState(() {
      final idx = _subjects.indexWhere((s) => s['id'] == subjectId);
      if (idx != -1) {
        _subjects[idx]['total_classes'] = totalClasses;
        _subjects[idx]['min_attendance_pct'] = minAttendancePct;
        if (_subjectConfigs.containsKey(subjectId)) {
          _subjectConfigs[subjectId]!['minPct'] = minAttendancePct.round();
        }
        if (newAbsences != null) {
          _subjects[idx]['absences'] = newAbsences;
          // sincronizar lista global
          _absences.removeWhere((a) => a['subject_id'] == subjectId);
          _absences.addAll(newAbsences);
        }
      }
    });
  }

  void _saveCourseConfig(String subjectId, int daysPerWeek, int weeks, double minPct) {
    final totalClasses = daysPerWeek * weeks;
    setState(() {
      final idx = _subjects.indexWhere((s) => s['id'] == subjectId);
      if (idx != -1) {
        _subjects[idx]['days_per_week'] = daysPerWeek;
        _subjects[idx]['weeks'] = weeks;
        _subjects[idx]['total_classes'] = totalClasses;
        _subjects[idx]['min_attendance_pct'] = minPct;
        if (_subjectConfigs.containsKey(subjectId)) {
          _subjectConfigs[subjectId]!['daysPerWeek'] = daysPerWeek;
          _subjectConfigs[subjectId]!['weeks'] = weeks;
          _subjectConfigs[subjectId]!['minPct'] = minPct.round();
        }
      }
    });
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Configuración guardada correctamente'), backgroundColor: AppColors.border),
    );
  }

  List<Map<String, dynamic>> get _filteredSubjects {
    if (_filterMode == 'criticos') {
      return _subjects.where(_isCritical).toList();
    }
    return _subjects;
  }

  List<Map<String, dynamic>> get _filteredAbsences {
    if (_filterMode == 'criticos') {
      final critIds = _criticalSubjects.map((s) => s['id'] as String).toSet();
      return _absences.where((a) => critIds.contains(a['subject_id'])).toList()
        ..sort((a, b) => (b['date'] as DateTime).compareTo(a['date'] as DateTime));
    }
    final list = [..._absences];
    list.sort((a, b) => (b['date'] as DateTime).compareTo(a['date'] as DateTime));
    return list;
  }

  // ignore: unused_element
  void _openSpecific(String subjectId) {
    _openSubjectDetail(subjectId);
  }

  void _openSubjectDetail(String subjectId) {
    setState(() {
      _selectedSubjectId = subjectId;
    });
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.jumpTo(0.0);
      }
    });
  }

  void _backToGeneral() {
    setState(() => _selectedSubjectId = null);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.jumpTo(0.0);
      }
    });
  }

  Future<void> _showRegisterAbsenceSheet(String subjectId) async {
    final subject = _subjectById(subjectId);
    if (subject == null) return;

    DateTime selectedDate = DateTime.now();
    bool justified = false;

    final result = await showModalBottomSheet<Map<String, dynamic>>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (ctx) {
        return StatefulBuilder(builder: (ctx2, setSheet) {
          return Padding(
            padding: EdgeInsets.only(bottom: MediaQuery.of(ctx2).viewInsets.bottom),
            child: Container(
              decoration: BoxDecoration(
                color: AppColors.surface,
                border: const Border(top: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
                borderRadius: BorderRadius.vertical(top: Radius.circular(AppDimens.radius * 2)),
              ),
              padding: const EdgeInsets.fromLTRB(20, 14, 20, 20),
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Center(child: Container(width: 44, height: 5, decoration: BoxDecoration(color: AppColors.muted.withOpacity(0.6), borderRadius: BorderRadius.circular(4)))),
                    const SizedBox(height: 16),
                    Text('REGISTRAR INASISTENCIA · ${(subject['name'] as String).toUpperCase()}', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
                    const SizedBox(height: 18),
                    const AppFieldLabel('FECHA DE LA INASISTENCIA'),
                    const SizedBox(height: 6),
                    InkWell(
                      onTap: () async {
                        final picked = await showDatePicker(
                          context: ctx2,
                          initialDate: selectedDate,
                          firstDate: DateTime(2025, 1, 1),
                          lastDate: DateTime.now().add(const Duration(days: 1)),
                          helpText: 'SELECCIONA FECHA',
                          confirmText: 'CONFIRMAR',
                          cancelText: 'CANCELAR',
                        );
                        if (picked != null) setSheet(() => selectedDate = picked);
                      },
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                        decoration: BoxDecoration(
                          color: AppColors.bg,
                          border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
                          borderRadius: BorderRadius.circular(AppDimens.radius),
                        ),
                        child: Row(
                          children: [
                            const Icon(Icons.calendar_month_rounded, size: 18, color: AppColors.text),
                            const SizedBox(width: 10),
                            Text(_formatDateFull(selectedDate), style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 14, color: AppColors.text)),
                            const Spacer(),
                            const Icon(Icons.expand_more_rounded, size: 20, color: AppColors.muted),
                          ],
                        ),
                      ),
                    ),
                    const SizedBox(height: 16),
                    Container(
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                      child: Row(
                        children: [
                          Checkbox(
                            value: justified,
                            activeColor: AppColors.accentYellow,
                            checkColor: AppColors.text,
                            side: const BorderSide(color: AppColors.border, width: 2),
                            onChanged: (v) => setSheet(() => justified = v ?? false),
                          ),
                          const SizedBox(width: 4),
                          const Expanded(child: Text('MARCAR COMO JUSTIFICADA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text))),
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                            decoration: BoxDecoration(color: justified ? const Color(0xFFD6E3FF) : const Color(0xFFFFDAD6), border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                            child: Text(justified ? 'JUSTIFICADA' : 'INJUSTIFICADA', style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w900)),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 8),
                    const Text('Las faltas justificadas NO restan porcentaje de asistencia.', style: TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: AppColors.muted)),
                    const SizedBox(height: 22),
                    Row(
                      children: [
                        Expanded(child: OutlinedButton(onPressed: () => Navigator.of(ctx2).pop(), style: OutlinedButton.styleFrom(side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), padding: const EdgeInsets.symmetric(vertical: 14)), child: const Text('CANCELAR', style: TextStyle(fontWeight: FontWeight.w900, color: AppColors.text)))),
                        const SizedBox(width: 12),
                        Expanded(child: SubmitButton(text: 'REGISTRAR', onPressed: () => Navigator.of(ctx2).pop({'date': selectedDate, 'justified': justified}))),
                      ],
                    ),
                  ],
                ),
              ),
            ),
          );
        });
      },
    );

    if (result != null) {
      final date = result['date'] as DateTime;
      final just = result['justified'] as bool;
      final newEntry = {
        'id': 'abs-${DateTime.now().microsecondsSinceEpoch}',
        'subject_id': subjectId,
        'date': date,
        'justified': just,
        'source': 'self',
        'type': 'Clase regular',
      };
      // Centralizar actualización para que el resumen crítico reaccione
      final currentAbs = List<Map<String, dynamic>>.from(
        (_subjectById(subjectId)?['absences'] as List?)?.cast<Map<String, dynamic>>() ?? _absencesFor(subjectId),
      );
      final updatedAbs = [newEntry, ...currentAbs];
      final subj = _subjectById(subjectId);
      final total = subj?['total_classes'] as int? ?? 0;
      final minPct = (_subjectConfigs[subjectId]?['minPct'] as num?)?.toDouble() ?? (subj?['min_attendance_pct'] as num?)?.toDouble() ?? 75.0;
      _updateSubjectAttendance(
        subjectId: subjectId,
        totalClasses: total,
        minAttendancePct: minPct,
        newAbsences: updatedAbs,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(just ? 'Inasistencia justificada registrada' : 'Inasistencia registrada', style: const TextStyle(fontWeight: FontWeight.w700, color: Colors.white)),
          backgroundColor: AppColors.border,
        ),
      );
    }
  }

  void _deleteAbsence(String absenceId) {
    final abs = _absences.firstWhere((a) => a['id'] == absenceId, orElse: () => <String, dynamic>{});
    if (abs.isEmpty) return;
    final subjectId = abs['subject_id'] as String;
    final currentAbs = List<Map<String, dynamic>>.from(
      (_subjectById(subjectId)?['absences'] as List?)?.cast<Map<String, dynamic>>() ?? _absencesFor(subjectId),
    );
    final updatedAbs = currentAbs.where((a) => a['id'] != absenceId).toList();
    final subj = _subjectById(subjectId);
    final total = subj?['total_classes'] as int? ?? 0;
    final minPct = (_subjectConfigs[subjectId]?['minPct'] as num?)?.toDouble() ?? (subj?['min_attendance_pct'] as num?)?.toDouble() ?? 75.0;
    _updateSubjectAttendance(
      subjectId: subjectId,
      totalClasses: total,
      minAttendancePct: minPct,
      newAbsences: updatedAbs,
    );
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Registro eliminado'), backgroundColor: AppColors.border));
  }

  void _saveConfig(String subjectId) {
    final config = _subjectConfigs[subjectId]!;
    final daysPerWeek = config['daysPerWeek'] as int;
    final weeks = config['weeks'] as int;
    final minPct = (config['minPct'] as num).toDouble();
    _saveCourseConfig(subjectId, daysPerWeek, weeks, minPct);
  }

  // ---------------------------------------------------------------------
  @override
  Widget build(BuildContext context) {
    final screenWidth = MediaQuery.of(context).size.width;
    final isMobile = screenWidth < 768;
    final canPop = Navigator.of(context).canPop();

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: _hasError
            ? _buildErrorState()
            : _isLoading
                ? _buildLoadingCard()
                : SingleChildScrollView(
                    controller: _scrollController,
                    physics: const AlwaysScrollableScrollPhysics(),
                    padding: EdgeInsets.symmetric(horizontal: isMobile ? 16 : 32, vertical: 20),
                    child: Center(
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 1200),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            _buildHeader(canPop, !isMobile),
                            const SizedBox(height: 16),
                            if (_selectedSubjectId == null)
                              _buildGeneralSummaryView(isMobile)
                            else
                              _buildSubjectDetailView(isMobile),
                          ],
                        ),
                      ),
                    ),
                  ),
      ),
    );
  }

  // Wrappers para compatibilidad con spec de rescate
  Widget _buildGeneralSummaryView(bool isMobile) => _buildGeneralView(!isMobile);
  Widget _buildSubjectDetailView(bool isMobile) => _buildSpecificView(!isMobile);

  Widget _buildHeader(bool canPop, bool isDesktop) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        if (canPop) ...[
          _SquareIconButton(icon: Icons.arrow_back_rounded, tooltip: 'Volver', onPressed: () => Navigator.of(context).pop()),
          const SizedBox(width: 12),
        ],
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('CONTROL DE INASISTENCIAS', style: TextStyle(color: AppColors.text, fontWeight: FontWeight.w900, fontSize: isDesktop ? 26 : 20, letterSpacing: -0.5)),
              const SizedBox(height: 3),
              const Text('Monitoreo detallado del registro de asistencia y límites permitidos por asignatura.', style: TextStyle(color: AppColors.muted, fontWeight: FontWeight.w700, fontSize: 12.5)),
            ],
          ),
        ),
        const SizedBox(width: 12),
        _SquareIconButton(icon: Icons.refresh_rounded, tooltip: 'Actualizar', onPressed: _loadData),
      ],
    );
  }

  // ---------------------------------------------------------------------
  // VISTA GENERAL
  // ---------------------------------------------------------------------
  Widget _buildGeneralView(bool isDesktop) {
    final isMobile = !isDesktop;
    final total = _absences.length;
    final justified = _absences.where((a) => a['justified'] == true).length;
    final pending = _absences.where((a) => a['justified'] == false).length;

    // Cálculo reactivo derivado en cada build() — lista centralizada con remaining <=1
    final criticalSubjects = _criticalSubjects;

    String criticalSubjectName = '';
    if (criticalSubjects.isNotEmpty) {
      criticalSubjectName = criticalSubjects.first['name'] as String;
    }

    final filteredAbsences = _filteredAbsences;
    final filteredSubjects = _filteredSubjects;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Filtros superiores (replica pmn: semestre + programa + filtro rápido)
        Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)]),
          child: Wrap(
            spacing: 16,
            runSpacing: 14,
            crossAxisAlignment: WrapCrossAlignment.end,
            children: [
              SizedBox(
                width: isDesktop ? 220 : double.infinity,
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  const AppFieldLabel('SEMESTRE ACADÉMICO'),
                  const SizedBox(height: 6),
                  _DropdownField(
                    value: _selectedSemester,
                    items: const ['2026 - Semestre 2', '2026 - Semestre 1', '2025 - Semestre 2'],
                    onChanged: (v) => setState(() => _selectedSemester = v),
                  ),
                ]),
              ),
              SizedBox(
                width: isDesktop ? 260 : double.infinity,
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  const AppFieldLabel('PROGRAMA'),
                  const SizedBox(height: 6),
                  _DropdownField(value: _selectedProgram, items: const ['Ingeniería Civil en Informática', 'Ingeniería Civil Industrial'], onChanged: (v) => setState(() => _selectedProgram = v)),
                ]),
              ),
              Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                const AppFieldLabel('FILTRO RÁPIDO'),
                const SizedBox(height: 6),
                Container(
                  decoration: BoxDecoration(border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                  child: Row(mainAxisSize: MainAxisSize.min, children: [
                    _FilterChip(label: 'Todos', active: _filterMode == 'todos', onTap: () => setState(() => _filterMode = 'todos')),
                    _FilterChip(label: 'Críticos', active: _filterMode == 'criticos', onTap: () => setState(() => _filterMode = 'criticos'), isLast: true),
                  ]),
                ),
              ]),
              SizedBox(
                height: 42,
                child: ElevatedButton.icon(
                  onPressed: () {
                    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Filtros aplicados'), backgroundColor: AppColors.border));
                  },
                  style: ElevatedButton.styleFrom(backgroundColor: AppColors.accentBlue, foregroundColor: Colors.white, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), elevation: 0),
                  icon: const Icon(Icons.filter_alt_rounded, size: 18),
                  label: const Text('FILTRAR', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        // Tarjetas superiores de resumen — estructura segura: Column en mobile, Row+Expanded en desktop
        if (isMobile)
          Column(
            children: [
              SizedBox(width: double.infinity, child: _CriticalCard(count: criticalSubjects.length, subjectName: criticalSubjectName, criticalNames: criticalSubjects.map((s) => s['name'] as String).toList(), onView: criticalSubjects.isNotEmpty ? () => _openSubjectDetail(criticalSubjects.first['id'] as String) : null)),
              const SizedBox(height: 16),
              SizedBox(width: double.infinity, child: _SummaryCard(total: total, justified: justified, pending: pending)),
            ],
          )
        else
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(child: _CriticalCard(count: criticalSubjects.length, subjectName: criticalSubjectName, criticalNames: criticalSubjects.map((s) => s['name'] as String).toList(), onView: criticalSubjects.isNotEmpty ? () => _openSubjectDetail(criticalSubjects.first['id'] as String) : null)),
              const SizedBox(width: 16),
              Expanded(child: _SummaryCard(total: total, justified: justified, pending: pending)),
            ],
          ),
        const SizedBox(height: 16),
        // Tabla Historial — responsiva mobile/desktop
        Container(
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                decoration: const BoxDecoration(color: AppColors.accentYellow, border: Border(bottom: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)), borderRadius: BorderRadius.vertical(top: Radius.circular(AppDimens.radius - 1))),
                child: Row(
                  children: [
                    const Expanded(child: Text('HISTORIAL DE FALTAS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text))),
                    InkWell(
                      onTap: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Exportando historial...'), backgroundColor: AppColors.border)),
                      child: Row(children: const [Icon(Icons.download_rounded, size: 16, color: AppColors.text), SizedBox(width: 4), Text('EXPORTAR', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text, decoration: TextDecoration.underline))]),
                    ),
                  ],
                ),
              ),
              if (filteredAbsences.isEmpty)
                const Padding(padding: EdgeInsets.all(24), child: Center(child: Text('SIN REGISTROS PARA EL FILTRO SELECCIONADO', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 12, color: AppColors.muted))))
              else if (isMobile)
                Padding(
                  padding: const EdgeInsets.all(12),
                  child: Column(
                    children: filteredAbsences.take(8).map((a) {
                      final date = a['date'] as DateTime;
                      final justifiedAbs = a['justified'] as bool;
                      final subject = _subjectById(a['subject_id'] as String);
                      final subjectName = subject != null ? subject['name'] as String : a['subject_id'] as String;
                      return Padding(
                        padding: const EdgeInsets.only(bottom: 10),
                        child: _MobileAbsenceCard(
                          date: date,
                          subjectName: subjectName,
                          type: a['type'] as String,
                          justified: justifiedAbs,
                          onTap: () => _openSubjectDetail(a['subject_id'] as String),
                        ),
                      );
                    }).toList(),
                  ),
                )
              else
                _buildDesktopAbsenceTable(filteredAbsences.take(8).toList()),
              // Footer paginación mock
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                decoration: const BoxDecoration(border: Border(top: BorderSide(color: AppColors.border, width: 2))),
                child: Row(
                  children: [
                    Text('Mostrando ${filteredAbsences.take(8).length} de ${filteredAbsences.length} registros', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
                    const Spacer(),
                    _PageButton(icon: Icons.chevron_left_rounded, enabled: false, onTap: () {}),
                    const SizedBox(width: 4),
                    _PageButton(label: '1', active: true, onTap: () {}),
                    const SizedBox(width: 4),
                    _PageButton(label: '2', onTap: () {}),
                    const SizedBox(width: 4),
                    _PageButton(icon: Icons.chevron_right_rounded, onTap: () {}),
                  ],
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        // Listado rápido de asignaturas con asistencia
        Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)]),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const Text('ASISTENCIA POR ASIGNATURA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text)),
            const SizedBox(height: 4),
            const Divider(color: AppColors.border, thickness: AppDimens.borderWidth, height: 18),
            if (filteredSubjects.isEmpty)
              const Padding(padding: EdgeInsets.symmetric(vertical: 12), child: Text('Ninguna asignatura en estado crítico', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 12, color: AppColors.muted)))
            else
              ...filteredSubjects.map((s) {
                final absList = s['absences'] as List? ?? _absences.where((a) => a['subject_id'] == s['id']).toList();
                final unjust = absList.where((a) => a['justified'] != true).length;
                final totalClasses = s['total_classes'] as int?;
                final pct = _computeAttendance(totalClasses: totalClasses, unjustified: unjust);
                final minPct = (s['min_attendance_pct'] as num?)?.toDouble() ?? (_subjectConfigs[s['id']]!['minPct'] as num).toDouble();
                final isCrit = _isCritical(s);
                final remaining = _remainingAbsences(totalClasses: totalClasses, minPct: minPct.round(), unjustified: unjust);
                return Container(
                  margin: const EdgeInsets.only(bottom: 10),
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(color: isCrit ? const Color(0xFFFFE1DE) : AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                  child: Row(
                    children: [
                      Expanded(
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Text((s['name'] as String).toUpperCase(), style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12.5, color: AppColors.text)),
                          const SizedBox(height: 2),
                          Text('${s['code']} · ${s['total_classes'] == null ? 'Sin config.' : '${s['total_classes']} clases'} · Mín ${minPct.round()}%', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
                          const SizedBox(height: 6),
                          Row(children: [
                            Container(width: 90, height: 8, decoration: BoxDecoration(border: Border.all(color: AppColors.border, width: 1.5)), child: pct == null ? Container(color: AppColors.muted.withOpacity(0.2)) : FractionallySizedBox(alignment: Alignment.centerLeft, widthFactor: (pct / 100).clamp(0, 1), child: Container(color: isCrit ? AppColors.error : AppColors.accentYellow))),
                            const SizedBox(width: 8),
                            Text(pct == null ? 'No configurado' : '${pct.round()}% asistencia', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: pct == null ? AppColors.muted : (isCrit ? AppColors.error : AppColors.text))),
                          ]),
                          if (remaining != null)
                            Padding(
                              padding: const EdgeInsets.only(top: 4),
                              child: Text(remaining < 0 ? 'Reprobado por asistencia' : remaining == 0 ? 'Sin faltas restantes' : 'Te quedan $remaining faltas', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: remaining < 0 ? AppColors.error : AppColors.text)),
                            ),
                        ]),
                      ),
                      const SizedBox(width: 10),
                      InkWell(
                        onTap: () => _openSubjectDetail(s['id'] as String),
                        borderRadius: BorderRadius.circular(4),
                        child: Container(padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8), decoration: BoxDecoration(color: AppColors.border, borderRadius: BorderRadius.circular(4)), child: const Text('VER', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: Colors.white))),
                      ),
                    ],
                  ),
                );
              }),
          ]),
        ),
      ],
    );
  }

  double cWidth(BuildContext context) {
    final w = MediaQuery.of(context).size.width;
    // ancho mínimo para tabla desktop para evitar compresión
    return w < 700 ? 640 : w * 0.9;
  }

  Widget _buildDesktopAbsenceTable(List<Map<String, dynamic>> records) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Cabecera de la tabla
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
          decoration: const BoxDecoration(
            color: AppColors.surface,
            border: Border(
              bottom: BorderSide(color: AppColors.border, width: 2),
            ),
          ),
          child: Row(
            children: const [
              SizedBox(width: 80, child: Text('FECHA', style: _headerStyle)),
              Expanded(flex: 4, child: Text('ASIGNATURA', style: _headerStyle)),
              Expanded(flex: 2, child: Text('TIPO', style: _headerStyle)),
              Expanded(flex: 2, child: Center(child: Text('ESTADO', style: _headerStyle))),
              SizedBox(width: 120, child: Align(alignment: Alignment.centerRight, child: Text('ACCIÓN', style: _headerStyle))),
            ],
          ),
        ),
        // Filas de faltas
        ListView.separated(
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          itemCount: records.length,
          separatorBuilder: (_, __) => const Divider(
            color: Color(0xFFE2DDD4),
            height: 1,
            thickness: 1,
          ),
          itemBuilder: (context, index) {
            final item = records[index];
            final isJustified = item['justified'] == true;
            final dateVal = item['date'];
            final dateStr = dateVal is DateTime ? _formatDate(dateVal) : (dateVal?.toString() ?? '');
            final subjectId = item['subject_id'] as String? ?? '';
            final subject = _subjectById(subjectId);
            final subjectName = subject != null ? subject['name'] as String : (item['subject_name']?.toString() ?? subjectId);
            final typeStr = item['type']?.toString() ?? 'Clase regular';
            return InkWell(
              onTap: () => _openSubjectDetail(subjectId),
              hoverColor: AppColors.surface,
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
                child: Row(
                  children: [
                    // Fecha
                    SizedBox(
                      width: 80,
                      child: Text(
                        dateStr,
                        style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text),
                      ),
                    ),
                    // Asignatura
                    Expanded(
                      flex: 4,
                      child: Text(
                        subjectName,
                        style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13.5, color: AppColors.text),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    // Tipo (Teoría / Lab / Clase regular)
                    Expanded(
                      flex: 2,
                      child: Text(
                        typeStr,
                        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12, color: AppColors.muted),
                      ),
                    ),
                    // Badge Estado (Centrado en su columna)
                    Expanded(
                      flex: 2,
                      child: Center(
                        child: Container(
                          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                          decoration: BoxDecoration(
                            color: isJustified ? const Color(0xFFD6E3FF) : const Color(0xFFFFDAD6),
                            border: Border.all(color: AppColors.border, width: 1.5),
                            borderRadius: BorderRadius.circular(4),
                          ),
                          child: Text(
                            isJustified ? 'JUSTIFICADA' : 'INJUSTIFICADA',
                            style: TextStyle(
                              fontSize: 10,
                              fontWeight: FontWeight.w900,
                              color: isJustified ? const Color(0xFF003399) : const Color(0xFF990000),
                              letterSpacing: 0.3,
                            ),
                          ),
                        ),
                      ),
                    ),
                    // Acción
                    SizedBox(
                      width: 120,
                      child: Align(
                        alignment: Alignment.centerRight,
                        child: TextButton(
                          onPressed: () => _openSubjectDetail(subjectId),
                          style: TextButton.styleFrom(
                            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                            minimumSize: Size.zero,
                            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                          ),
                          child: Text(
                            isJustified ? 'Ver Doc.' : 'Justificar →',
                            style: TextStyle(
                              fontWeight: FontWeight.w900,
                              fontSize: 12,
                              color: isJustified ? AppColors.text : AppColors.accentBlue,
                              decoration: isJustified ? null : TextDecoration.underline,
                            ),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            );
          },
        ),
      ],
    );
  }

  // ---------------------------------------------------------------------
  // VISTA ESPECÍFICA — apilada vertical en mobile
  // ---------------------------------------------------------------------
  Widget _buildSpecificView(bool isDesktop) {
    final subjectId = _selectedSubjectId!;
    final subject = _subjectById(subjectId);
    if (subject == null) {
      return Container(
        padding: const EdgeInsets.all(18),
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
        child: Column(children: [
          const Text('ASIGNATURA NO ENCONTRADA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
          const SizedBox(height: 10),
          SubmitButton(text: 'VOLVER AL RESUMEN', onPressed: _backToGeneral),
        ]),
      );
    }
    final config = _subjectConfigs[subjectId]!;
    final absList = subject['absences'] as List? ?? _absencesFor(subjectId);
    final unjust = absList.where((a) => a['justified'] != true).length;
    final totalClasses = subject['total_classes'] as int?;
    final minPct = config['minPct'] as int;
    final pct = _computeAttendance(totalClasses: totalClasses, unjustified: unjust);
    final remaining = _remainingAbsences(totalClasses: totalClasses, minPct: minPct, unjustified: unjust);
    final subjectAbsences = List<Map<String, dynamic>>.from(absList.cast<Map<String, dynamic>>())..sort((a, b) => (b['date'] as DateTime).compareTo(a['date'] as DateTime));

    final pctLabel = pct == null ? '—' : '${pct.round()}%';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        InkWell(
          onTap: _backToGeneral,
          borderRadius: BorderRadius.circular(AppDimens.radius),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
            child: Row(mainAxisSize: MainAxisSize.min, children: const [Icon(Icons.arrow_back_rounded, size: 16, color: AppColors.text), SizedBox(width: 6), Text('VOLVER AL RESUMEN GENERAL', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text))]),
          ),
        ),
        const SizedBox(height: 14),
        Text('CONTROL DE INASISTENCIAS · ${(subject['name'] as String).toUpperCase()}', style: TextStyle(fontWeight: FontWeight.w900, fontSize: isDesktop ? 18 : 15, color: AppColors.text, letterSpacing: -0.3)),
        const Divider(color: AppColors.border, thickness: 4, height: 20),
        // Configuración del curso — siempre arriba
        Container(
          padding: EdgeInsets.all(isDesktop ? 20 : 16),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(children: [const Expanded(child: Text('CONFIGURACIÓN DEL CURSO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text))), Container(width: 36, height: 36, decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)), child: const Icon(Icons.settings_rounded, size: 18, color: AppColors.text))]),
              const Divider(color: AppColors.border, thickness: 2, height: 20),
              Wrap(
                spacing: 24,
                runSpacing: 16,
                children: [
                  SizedBox(
                    width: isDesktop ? 200 : double.infinity,
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const AppFieldLabel('DÍAS DE CLASE / SEMANA'),
                      const SizedBox(height: 8),
                      Row(children: [
                        _SquareIconButton(
                          icon: Icons.remove_rounded,
                          onPressed: () => setState(() => config['daysPerWeek'] = ((config['daysPerWeek'] as int) - 1).clamp(1, 7)),
                        ),
                        const SizedBox(width: 8),
                        Container(width: 64, height: 40, alignment: Alignment.center, decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)), child: Text('${config['daysPerWeek']}', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 18, color: AppColors.text))),
                        const SizedBox(width: 8),
                        _SquareIconButton(
                          icon: Icons.add_rounded,
                          onPressed: () => setState(() => config['daysPerWeek'] = ((config['daysPerWeek'] as int) + 1).clamp(1, 7)),
                        ),
                      ]),
                    ]),
                  ),
                  SizedBox(
                    width: isDesktop ? 200 : double.infinity,
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const AppFieldLabel('SEMANAS DEL SEMESTRE'),
                      const SizedBox(height: 8),
                      SizedBox(
                        width: 120,
                        child: TextFormField(
                          initialValue: '${config['weeks']}',
                          keyboardType: TextInputType.number,
                          style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text),
                          decoration: appInputDecoration('16'),
                          onChanged: (v) {
                            final n = int.tryParse(v);
                            if (n != null) setState(() => config['weeks'] = n.clamp(1, 30));
                          },
                        ),
                      ),
                    ]),
                  ),
                  SizedBox(
                    width: isDesktop ? 260 : double.infinity,
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      Row(children: [
                        const Expanded(child: AppFieldLabel('MÍNIMO ASISTENCIA')),
                        Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3), decoration: BoxDecoration(color: AppColors.border, borderRadius: BorderRadius.circular(4)), child: Text('$minPct%', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: Colors.white))),
                      ]),
                      Slider(
                        value: minPct.toDouble(),
                        min: 0,
                        max: 100,
                        divisions: 20,
                        activeColor: AppColors.border,
                        inactiveColor: AppColors.muted.withOpacity(0.2),
                        onChanged: (v) => setState(() => config['minPct'] = v.round()),
                      ),
                    ]),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              Align(
                alignment: Alignment.centerRight,
                child: ElevatedButton(
                  onPressed: () => _saveConfig(subjectId),
                  style: ElevatedButton.styleFrom(backgroundColor: AppColors.accentYellow, foregroundColor: AppColors.text, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), elevation: 0, padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 14)),
                  child: const Text('GUARDAR CONFIGURACIÓN', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        // En mobile apila vertical: ESTADO ACTUAL arriba, HISTORIAL debajo. En desktop lado a lado.
        LayoutBuilder(builder: (context, c) {
          final useRow = isDesktop && c.maxWidth > 800;
          final colWidth = useRow ? (c.maxWidth - 16) / 2 : c.maxWidth;
          // Mobile: forzar apilado vertical garantizado
          final children = [
            SizedBox(
              width: colWidth,
              child: Container(
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(color: AppColors.border, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
                child: Column(
                  children: [
                    const Align(alignment: Alignment.centerLeft, child: Text('ESTADO ACTUAL', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: Colors.white, letterSpacing: 0.4))),
                    const SizedBox(height: 16),
                    Container(
                      width: 160,
                      height: 160,
                      decoration: BoxDecoration(shape: BoxShape.circle, border: Border.all(color: AppColors.accentYellow, width: 8), color: AppColors.border),
                      child: Center(child: Text(pctLabel, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 44, color: AppColors.accentYellow))),
                    ),
                    const SizedBox(height: 16),
                    Transform.rotate(
                      angle: -0.02,
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
                        child: Column(children: [
                          if (pct == null)
                            const Text('NO CONFIGURADO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.muted))
                          else if (remaining == null)
                            const Text('SIN LÍMITE CONFIGURADO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text))
                          else if (remaining < 0)
                            const Text('REPROBADO POR ASISTENCIA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.error))
                          else
                            Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                              const Text('Te quedan ', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text)),
                              Text('$remaining', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 22, color: AppColors.error)),
                              const Text(' faltas', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text)),
                            ]),
                          const SizedBox(height: 2),
                          Text(pct == null ? 'Define total de clases' : 'antes de reprobar por asistencia', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 10, color: AppColors.muted)),
                        ]),
                      ),
                    ),
                    const SizedBox(height: 12),
                    Container(padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6), decoration: BoxDecoration(color: AppColors.muted.withOpacity(0.25), border: Border.all(color: Colors.white.withOpacity(0.5), width: 1), borderRadius: BorderRadius.circular(4)), child: Text('$minPct% REQUERIDO', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: Colors.white))),
                    const SizedBox(height: 16),
                    SizedBox(
                      width: double.infinity,
                      child: ElevatedButton.icon(
                        onPressed: () => _showRegisterAbsenceSheet(subjectId),
                        style: ElevatedButton.styleFrom(backgroundColor: AppColors.error, foregroundColor: Colors.white, side: const BorderSide(color: Colors.white, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), elevation: 0, padding: const EdgeInsets.symmetric(vertical: 16)),
                        icon: const Icon(Icons.add_circle_rounded, size: 20),
                        label: const Text('REGISTRAR INASISTENCIA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13)),
                      ),
                    ),
                    if (totalClasses == null) ...[
                      const SizedBox(height: 12),
                      Container(
                        padding: const EdgeInsets.all(10),
                        decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                        child: const Row(children: [Icon(Icons.warning_amber_rounded, size: 18, color: AppColors.text), SizedBox(width: 8), Expanded(child: Text('Configura total_classes para calcular asistencia', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.text)))]),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            SizedBox(
              width: colWidth,
              child: Container(
                decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                      decoration: const BoxDecoration(color: AppColors.border, borderRadius: BorderRadius.vertical(top: Radius.circular(AppDimens.radius - 1))),
                      child: Row(children: [
                        const Expanded(child: Text('HISTORIAL DE FALTAS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: Colors.white))),
                        Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4), decoration: BoxDecoration(color: Colors.white, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text('TOTAL ${subjectAbsences.length}', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text))),
                      ]),
                    ),
                    if (subjectAbsences.isEmpty)
                      const Padding(padding: EdgeInsets.all(24), child: Center(child: Text('SIN INASISTENCIAS REGISTRADAS', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 12, color: AppColors.muted))))
                    else
                      Padding(
                        padding: const EdgeInsets.all(12),
                        child: Column(
                          children: subjectAbsences.map((a) {
                            final d = a['date'] as DateTime;
                            return Container(
                              margin: const EdgeInsets.only(bottom: 8),
                              padding: const EdgeInsets.all(12),
                              decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                              child: Row(
                                children: [
                                  Container(
                                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                                    decoration: BoxDecoration(color: AppColors.border, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                                    child: Text('${d.day.toString().padLeft(2, '0')} ${_monthShort(d.month)}', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: Colors.white)),
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                                      Text(_weekdayLabel(d), style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text)),
                                      Text((a['type'] as String).toUpperCase(), style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 10, color: AppColors.muted)),
                                    ]),
                                  ),
                                  Container(
                                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
                                    decoration: BoxDecoration(color: (a['justified'] as bool) ? const Color(0xFFD6E3FF) : const Color(0xFFFFDAD6), border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                                    child: Text((a['justified'] as bool) ? 'JUST.' : 'INJUST.', style: const TextStyle(fontSize: 9, fontWeight: FontWeight.w900)),
                                  ),
                                  const SizedBox(width: 8),
                                  _SquareIconButton(icon: Icons.delete_rounded, tooltip: 'Eliminar', onPressed: () => _deleteAbsence(a['id'] as String)),
                                ],
                              ),
                            );
                          }).toList(),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ];
          if (useRow) {
            return Wrap(spacing: 16, runSpacing: 16, children: children);
          }
          // Mobile: apilar vertical garantizado sin Wrap lateral
          return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            children[0],
            const SizedBox(height: 16),
            children[1],
          ]);
        }),
      ],
    );
  }

  String _monthShort(int m) {
    const months = ['Ene', 'Feb', 'Mar', 'Abr', 'May', 'Jun', 'Jul', 'Ago', 'Sep', 'Oct', 'Nov', 'Dic'];
    return months[m - 1];
  }

  // ---------------------------------------------------------------------
  Widget _buildLoadingCard() {
    return Container(
      height: 260,
      decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
      child: const Center(child: CircularProgressIndicator(color: AppColors.text)),
    );
  }

  Widget _buildErrorState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Icon(Icons.error_outline_rounded, size: 48, color: AppColors.text),
          const SizedBox(height: 12),
          const Text('NO SE PUDO CARGAR ASISTENCIA', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
          const SizedBox(height: 8),
          const Text('Revisa tu conexión e intenta nuevamente.', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.muted)),
          const SizedBox(height: 18),
          SizedBox(width: 200, child: SubmitButton(text: 'REINTENTAR', onPressed: _loadData)),
        ]),
      ),
    );
  }
}

// ---------------------------------------------------------------------
// Widgets auxiliares locales (replican estilo de profile_screen/schedule)
// ---------------------------------------------------------------------

class _MobileAbsenceCard extends StatelessWidget {
  final DateTime date;
  final String subjectName;
  final String type;
  final bool justified;
  final VoidCallback onTap;

  const _MobileAbsenceCard({
    required this.date,
    required this.subjectName,
    required this.type,
    required this.justified,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final dd = date.day.toString().padLeft(2, '0');
    final mm = date.month.toString().padLeft(2, '0');
    return InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border, width: 2),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                  child: Text('$dd/$mm', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text)),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                    decoration: BoxDecoration(color: justified ? const Color(0xFFD6E3FF) : const Color(0xFFFFDAD6), border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                    child: Text(justified ? 'JUSTIFICADA' : 'INJUSTIFICADA', textAlign: TextAlign.center, style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text)),
                  ),
                ),
                const SizedBox(width: 8),
                Text(justified ? 'Ver' : 'Justificar →', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: justified ? AppColors.text : AppColors.accentBlue, decoration: TextDecoration.underline)),
              ],
            ),
            const SizedBox(height: 10),
            Text(subjectName, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text)),
            const SizedBox(height: 4),
            Text(type, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
          ],
        ),
      ),
    );
  }
}

class _SquareIconButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onPressed;
  final String? tooltip;
  const _SquareIconButton({required this.icon, required this.onPressed, this.tooltip});

  @override
  Widget build(BuildContext context) {
    final button = InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onPressed,
      child: Container(
        width: 42,
        height: 42,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
        child: Icon(icon, size: 18, color: AppColors.text),
      ),
    );
    return tooltip != null ? Tooltip(message: tooltip!, child: button) : button;
  }
}

class _DropdownField extends StatelessWidget {
  final String value;
  final List<String> items;
  final ValueChanged<String> onChanged;
  const _DropdownField({required this.value, required this.items, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          isExpanded: true,
          icon: const Icon(Icons.expand_more_rounded, size: 20, color: AppColors.text),
          style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 12.5, color: AppColors.text),
          items: items.map((e) => DropdownMenuItem(value: e, child: Text(e, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 12.5, color: AppColors.text)))).toList(),
          onChanged: (v) {
            if (v != null) onChanged(v);
          },
        ),
      ),
    );
  }
}

class _FilterChip extends StatelessWidget {
  final String label;
  final bool active;
  final VoidCallback onTap;
  final bool isLast;
  const _FilterChip({required this.label, required this.active, required this.onTap, this.isLast = false});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        decoration: BoxDecoration(
          color: active ? AppColors.border : AppColors.surface,
          border: Border(right: isLast ? BorderSide.none : const BorderSide(color: AppColors.border, width: 2)),
        ),
        child: Text(label.toUpperCase(), style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: active ? Colors.white : AppColors.text)),
      ),
    );
  }
}

class _CriticalCard extends StatelessWidget {
  final int count;
  final String subjectName;
  final List<String> criticalNames;
  final VoidCallback? onView;
  const _CriticalCard({required this.count, required this.subjectName, this.criticalNames = const [], required this.onView});

  @override
  Widget build(BuildContext context) {
    final isCritical = count > 0;
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(color: isCritical ? AppColors.error : AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [Icon(Icons.warning_amber_rounded, color: isCritical ? Colors.white : AppColors.text, size: 18), const SizedBox(width: 6), Text('ESTADO CRÍTICO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: isCritical ? Colors.white : AppColors.text))]),
          const SizedBox(height: 12),
          Text('$count', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 44, color: isCritical ? Colors.white : AppColors.text, height: 1)),
          const SizedBox(height: 6),
          Text(count == 0 ? 'Sin cursos en riesgo' : count == 1 ? 'Curso en riesgo' : '$count cursos en riesgo', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: isCritical ? Colors.white : AppColors.text)),
          if (isCritical && criticalNames.isNotEmpty) ...[
            const SizedBox(height: 8),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(color: Colors.white.withOpacity(0.15), border: Border.all(color: Colors.white.withOpacity(0.5), width: 1), borderRadius: BorderRadius.circular(4)),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: criticalNames.map((n) => Padding(padding: const EdgeInsets.only(bottom: 2), child: Text('• $n', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: Colors.white)))).toList(),
              ),
            ),
            if (criticalNames.length == 1) ...[
              const SizedBox(height: 2),
              Text('($subjectName)', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: Colors.white70)),
            ],
          ] else if (count > 0) ...[
            const SizedBox(height: 2),
            Text('($subjectName)', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: isCritical ? Colors.white : AppColors.text)),
          ],
          if (isCritical) ...[
            const SizedBox(height: 14),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                onPressed: onView,
                style: ElevatedButton.styleFrom(backgroundColor: AppColors.border, foregroundColor: Colors.white, side: const BorderSide(color: Colors.white, width: 1.5), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(4)), elevation: 0, padding: const EdgeInsets.symmetric(vertical: 10)),
                child: const Text('VER DETALLES', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
              ),
            ),
          ] else ...[
            const SizedBox(height: 6),
            Text('Todos los ramos dentro del límite', style: TextStyle(fontWeight: FontWeight.w600, fontSize: 11, color: isCritical ? Colors.white70 : AppColors.muted)),
          ],
        ],
      ),
    );
  }
}

class _SummaryCard extends StatelessWidget {
  final int total;
  final int justified;
  final int pending;
  const _SummaryCard({required this.total, required this.justified, required this.pending});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('RESUMEN DEL PERIODO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.muted)),
          const SizedBox(height: 14),
          _SummaryRow(label: 'Total Inasistencias', value: '$total', valueColor: AppColors.text),
          const SizedBox(height: 10),
          _SummaryRow(label: 'Justificadas', value: '$justified', valueColor: AppColors.accentBlue),
          const SizedBox(height: 10),
          _SummaryRow(label: 'Por Justificar', value: '$pending', valueColor: AppColors.error),
        ],
      ),
    );
  }
}

class _SummaryRow extends StatelessWidget {
  final String label;
  final String value;
  final Color valueColor;
  const _SummaryRow({required this.label, required this.value, required this.valueColor});

  @override
  Widget build(BuildContext context) {
    return Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
      Text(label, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text)),
      Text(value, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 22, color: valueColor)),
    ]);
  }
}

class _PageButton extends StatelessWidget {
  final String? label;
  final IconData? icon;
  final bool active;
  final bool enabled;
  final VoidCallback onTap;
  const _PageButton({this.label, this.icon, this.active = false, this.enabled = true, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: enabled ? onTap : null,
      borderRadius: BorderRadius.circular(4),
      child: Container(
        width: 30,
        height: 30,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: active ? AppColors.border : AppColors.surface, border: Border.all(color: enabled ? AppColors.border : AppColors.muted.withOpacity(0.4), width: 1.5), borderRadius: BorderRadius.circular(4)),
        child: icon != null ? Icon(icon, size: 16, color: active ? Colors.white : (enabled ? AppColors.text : AppColors.muted)) : Text(label!, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: active ? Colors.white : AppColors.text)),
      ),
    );
  }
}
