import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/common_widgets.dart';
import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'sprint_editor_dialog.dart';

// ---------------------------------------------------------------------------
// Modelo de datos (tipeado + store estático en memoria para aguantar el hot
// reload sin perder el trabajo del usuario).
// ---------------------------------------------------------------------------

class _SprintTask {
  _SprintTask({
    required this.member,
    required this.task,
    required this.priority,
    required this.status,
    required this.estimate,
    required Map<String, double> days,
    this.backendId,
  }) : days = Map<String, double>.from(days);

  String member;
  String task;
  String priority;
  String status;
  double estimate;
  final Map<String, double> days;

  /// Id backend (sprint-sheet task). Null = fila local de vista previa.
  final String? backendId;

  /// true si la fila vive en el backend (toda mutación va por HTTP).
  bool get isReal => backendId != null && backendId!.isNotEmpty;

  double get used => days.values.fold(0.0, (a, b) => a + b);
  double get remaining => (estimate - used).clamp(0.0, 999.0);
  // Excedidas: solo presentación (used > estimated). No requiere schema.
  double get exceeded => (used - estimate) > 0 ? used - estimate : 0.0;

  _SprintTask copy() => _SprintTask(
    member: member,
    task: task,
    priority: priority,
    status: status,
    estimate: estimate,
    days: Map<String, double>.from(days),
    backendId: backendId,
  );
}

class _Sprint {
  _Sprint({
    required this.name,
    required this.goal,
    required this.start,
    required this.end,
    required this.days,
    required this.dayDates,
    required this.tasks,
    List<String>? dayIsoDates,
  }) : dayIsoDates = dayIsoDates ?? [];

  String name;
  String goal;
  String start;
  String end;
  final List<String> days;
  final List<String> dayDates;

  /// Fechas ISO (YYYY-MM-DD) paralelas a [days], para imputar horas reales.
  /// Vacío en vista previa local.
  final List<String> dayIsoDates;
  final List<_SprintTask> tasks;

  double get totalEstimate => tasks.fold(0.0, (s, t) => s + t.estimate);
  double get totalUsed => tasks.fold(0.0, (s, t) => s + t.used);
  double get totalRemaining => tasks.fold(0.0, (s, t) => s + t.remaining);
  double get totalExceeded => tasks.fold(0.0, (s, t) => s + t.exceeded);
}

class SprintSheetScreen extends StatefulWidget {
  const SprintSheetScreen({super.key, this.groupId, SocialService? service})
    : _serviceOverride = service;

  final String? groupId;
  final SocialService? _serviceOverride;

  @override
  State<SprintSheetScreen> createState() => _SprintSheetScreenState();
}

class _SprintSheetScreenState extends State<SprintSheetScreen> {
  late final SocialService _social;
  bool _loadingReal = false;
  String? _realError;
  List<SprintTask> _realTasks = const [];
  List<SprintSheetInfo> _realSheets = const [];
  bool _creatingReal = false;

  /// Índice de la hoja real seleccionada en los chips.
  int _realSheetIndex = 0;

  /// Miembros por user_id (nombres) y horas por tarea y fecha ISO.
  Map<String, GroupMember> _memberById = {};
  Map<String, Map<String, double>> _hoursByTask = {};

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? SocialService();
    if (_isRealGroup) _loadReal();
  }

  @override
  void dispose() {
    _tableScroll.dispose();
    if (widget._serviceOverride == null) _social.dispose();
    super.dispose();
  }

  @override
  void didUpdateWidget(SprintSheetScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    // Cambio de grupo: limpiar todo lo del anterior y cargar el nuevo.
    // Nunca se mezclan tareas/miembros/horas entre grupos.
    if (oldWidget.groupId?.trim() != widget.groupId?.trim()) {
      _realTasks = const [];
      _realSheets = const [];
      _realSheetIndex = 0;
      _memberById = {};
      _hoursByTask = {};
      _realError = null;
      _loadingReal = false;
      _creatingReal = false;
      if (_isRealGroup) _loadReal();
      if (mounted) setState(() {});
    }
  }

  bool get _isRealGroup {
    final id = widget.groupId?.trim() ?? '';
    final uuid = RegExp(
      r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
    );
    return id.isNotEmpty && uuid.hasMatch(id);
  }

  // Lectura: GET /groups/:id/workspace (sprint_sheet) con fallback a
  // GET /groups/:id/sprint-sheet. Luego miembros (nombres) y horas por tarea.
  SprintSheetInfo? get _activeSheet => _realSheets.isEmpty
      ? null
      : _realSheets[_realSheetIndex.clamp(0, _realSheets.length - 1)];

  Future<void> _loadReal({String? selectId}) async {
    if (!_isRealGroup || !mounted) return;
    final groupId = widget.groupId!.trim();
    final selected = selectId ?? _activeSheet?.id;
    setState(() {
      _loadingReal = true;
      _realError = null;
    });
    try {
      List<SprintSheetInfo> sheets;
      List<SprintTask> tasks;
      try {
        final ws = await _social.getWorkspace(groupId);
        sheets = ws.sheets;
        tasks = ws.sprintTasks;
      } on SocialApiException {
        sheets = await _social.listSprintSheets(groupId);
        tasks = await _social.listSprintTasks(groupId);
      }
      if (!mounted || groupId != widget.groupId?.trim()) return;
      setState(() {
        _realSheets = sheets;
        _realTasks = tasks;
        final index = sheets.indexWhere((s) => s.id == selected);
        _realSheetIndex = index < 0 ? 0 : index;
        _loadingReal = false;
      });
      await _loadRealSupport();
    } catch (_) {
      if (!mounted || groupId != widget.groupId?.trim()) return;
      setState(() {
        _realError = 'No se pudo cargar la hoja de sprint.';
        _loadingReal = false;
      });
    }
  }

  Future<void> _loadRealSupport() async {
    final groupId = widget.groupId?.trim();
    if (!_isRealGroup || !mounted) return;
    try {
      final members = await _social.listMembers(groupId!);
      if (!mounted || groupId != widget.groupId?.trim()) return;
      setState(() => _memberById = {for (final m in members) m.userId: m});
    } catch (_) {}
    final results = await Future.wait(
      _realTasks.map((t) async {
        try {
          return MapEntry(t.id, await _social.listHours(t.id));
        } catch (_) {
          return MapEntry<String, SprintHoursList?>(t.id, null);
        }
      }),
    );
    if (!mounted || groupId != widget.groupId?.trim()) return;
    setState(
      () => _hoursByTask = {
        for (final e in results)
          if (e.value != null)
            e.key: {
              for (final h in e.value!.entries)
                if ((h.logDate ?? '').isNotEmpty) h.logDate!: h.hours,
            },
      },
    );
  }

  final _tableScroll = ScrollController();

  Future<void> _manageSprint({bool edit = false}) async {
    final groupId = widget.groupId!.trim();
    final sheet = edit ? _activeSheet : null;
    final result = await showDialog<SprintDraft>(
      context: context,
      builder: (_) => SprintEditorDialog(
        sheet: sheet,
        suggestedName: 'Sprint ${_realSheets.length + 1}',
      ),
    );
    if (result == null || !mounted || groupId != widget.groupId?.trim()) return;
    try {
      final saved = await _social.saveSprintSheet(
        groupId: groupId,
        sheetId: sheet?.id,
        name: result.name,
        start: result.start,
        end: result.end,
      );
      if (!mounted || groupId != widget.groupId?.trim()) return;
      await _loadReal(selectId: saved.id);
    } catch (_) {
      if (!mounted || groupId != widget.groupId?.trim()) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'No se pudo guardar el sprint. Revisa las fechas e inténtalo de nuevo.',
          ),
        ),
      );
    }
  }

  /// Vista rica construida con datos reales (hojas, tareas, horas, nombres).
  /// Siempre retorna carcasa para grupo real (incluso vacío) para mantener
  /// la misma estructura visual: tabla visible + mensaje integrado.
  /// Null solo si no es grupo real o hay error/carga (lo cubre el panel).
  _Sprint? get _realSprintView {
    if (!_isRealGroup) return null;
    if (_realError != null || _loadingReal) return null;
    final sheets = _realSheets;
    final sheet = sheets.isNotEmpty
        ? sheets[_realSheetIndex.clamp(0, sheets.length - 1)]
        : null;
    final sheetId = sheet?.id ?? '';
    final tasks = _realTasks.where((t) => t.sheetId == sheetId).toList();
    if (tasks.isEmpty && sheets.isNotEmpty) {
      // La hoja elegida no tiene tareas: igual se muestra su cabecera.
    }
    final days = _realDayNames(sheet);
    final isoByDay = _realDayIso(sheet);
    return _Sprint(
      name: sheet?.name ?? 'Hoja de sprint',
      goal: '',
      start: _fmtDateLabel(sheet?.periodStart),
      end: _fmtDateLabel(sheet?.periodEnd),
      days: days,
      // Cabecera de fecha (dd/MM) paralela al nombre del día.
      dayDates: [
        for (final iso in isoByDay)
          () {
            final d = DateTime.tryParse(iso);
            if (d == null) return '';
            return '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}';
          }(),
      ],
      dayIsoDates: isoByDay,
      tasks: [
        for (final t in tasks)
          _SprintTask(
            member: _displayFor(t.assignedTo),
            task: t.title,
            priority: _capitalized(t.priority),
            status: _sprintStatusLabel(t.status),
            estimate: t.estimatedHours,
            days: Map<String, double>.from(_hoursByTask[t.id] ?? const {}),
            backendId: t.id,
          ),
      ],
    );
  }

  static const _weekdayNames = <int, String>{
    DateTime.monday: 'Lunes',
    DateTime.tuesday: 'Martes',
    DateTime.wednesday: 'Miércoles',
    DateTime.thursday: 'Jueves',
    DateTime.friday: 'Viernes',
    DateTime.saturday: 'Sábado',
    DateTime.sunday: 'Domingo',
  };

  /// Columnas de días corridos del período completo, o semana actual para hojas históricas.
  List<String> _realDayNames(SprintSheetInfo? sheet) =>
      _realDayPairs(sheet).map((p) => p.$1).toList();

  List<String> _realDayIso(SprintSheetInfo? sheet) =>
      _realDayPairs(sheet).map((p) => p.$2).toList();

  List<(String, String)> _realDayPairs(SprintSheetInfo? sheet) {
    final start = DateTime.tryParse(sheet?.periodStart ?? '');
    final end = DateTime.tryParse(sheet?.periodEnd ?? '');
    final out = <(String, String)>[];
    if (start != null && end != null && !end.isBefore(start)) {
      var d = DateTime(start.year, start.month, start.day);
      final last = DateTime(end.year, end.month, end.day);
      while (!d.isAfter(last)) {
        out.add((_weekdayNames[d.weekday] ?? '', _iso(d)));
        d = DateTime(d.year, d.month, d.day + 1);
      }
    }
    if (out.isEmpty) {
      final now = DateTime.now();
      var monday = now.subtract(Duration(days: now.weekday - 1));
      for (var i = 0; i < 5; i++) {
        final d = monday.add(Duration(days: i));
        out.add((_weekdayNames[d.weekday] ?? '', _iso(d)));
      }
    }
    return out;
  }

  static String _iso(DateTime d) =>
      '${d.year.toString().padLeft(4, '0')}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';

  static String _fmtDateLabel(String? raw) {
    final d = DateTime.tryParse(raw ?? '');
    if (d == null) return '—';
    return '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year.toString().substring(2)}';
  }

  /// Nombre humano del responsable (display_name/email) o id corto.
  String _displayFor(String userId) {
    final m = _memberById[userId];
    if (m != null) return m.displayLabel;
    if (userId.isEmpty) return 'Sin asignar';
    return userId.length > 8 ? '${userId.substring(0, 8)}…' : userId;
  }

  /// userId a partir del nombre visible de la fila (o id corto/UUID).
  String? _memberIdForDisplay(String display) {
    for (final entry in _memberById.entries) {
      if (entry.value.displayLabel == display) return entry.key;
    }
    final t = display.trim();
    if (t.isEmpty || t == 'Sin asignar') return null;
    if (_memberById.containsKey(t)) return t;
    return null;
  }

  static String _capitalized(String v) {
    final t = v.trim();
    if (t.isEmpty) return t;
    return t[0].toUpperCase() + t.substring(1).toLowerCase();
  }

  /// Backend (sin_empezar|en_proceso|listo) <-> etiqueta de la tabla.
  static String _sprintStatusLabel(String backend) {
    switch (backend.trim().toLowerCase()) {
      case 'en_proceso':
        return 'En proceso';
      case 'listo':
        return 'Listo';
      default:
        return 'Sin empezar';
    }
  }

  static String _sprintStatusBackend(String label) {
    switch (label.trim().toLowerCase()) {
      case 'en proceso':
        return 'en_proceso';
      case 'listo':
        return 'listo';
      default:
        return 'sin_empezar';
    }
  }

  /// Miembros del grupo para el selector de responsable.
  /// El backend no entrega nombres: se muestra id corto + rol.
  Future<List<GroupMember>?> _loadMembers() async {
    if (!_isRealGroup) return null;
    try {
      return await _social.listMembers(widget.groupId!.trim());
    } on SocialApiException catch (_) {
      if (!mounted) return null;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudieron cargar los miembros del grupo.'),
          backgroundColor: AppColors.error,
        ),
      );
      return null;
    }
  }

  Future<void> _createRealTask([GroupMember? preselect]) async {
    if (!_isRealGroup || _creatingReal) return;
    final members = await _loadMembers();
    if (!mounted) return;
    if (members == null || members.isEmpty) {
      if (members != null) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('El grupo aún no tiene miembros para asignar.'),
            backgroundColor: AppColors.error,
          ),
        );
      }
      return;
    }
    final titleCtrl = TextEditingController();
    final estimateCtrl = TextEditingController(text: '2.0');
    // Identificador estable (nunca el objeto): evita el assertion
    // "exactly one item with value" si la lista se reconstruye.
    String? selectedUserId =
        preselect != null && members.any((m) => m.userId == preselect.userId)
        ? preselect.userId
        : null;
    String priority = 'media';
    String status = 'sin_empezar';
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text(
          'NUEVA TAREA',
          style: TextStyle(fontWeight: FontWeight.w900),
        ),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: titleCtrl,
                decoration: const InputDecoration(
                  labelText: 'Título (requerido)',
                ),
              ),
              const SizedBox(height: 8),
              StatefulBuilder(
                builder: (context, setDialogState) => Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Text(
                      'Responsable (requerido)',
                      style: TextStyle(fontSize: 12),
                    ),
                    DropdownButton<String>(
                      value: selectedUserId,
                      hint: const Text('Elige un miembro'),
                      isExpanded: true,
                      items: [
                        for (final m in members)
                          DropdownMenuItem<String>(
                            value: m.userId,
                            child: Text(m.displayLabel),
                          ),
                      ],
                      onChanged: (v) =>
                          setDialogState(() => selectedUserId = v),
                    ),
                    const SizedBox(height: 8),
                    TextField(
                      controller: estimateCtrl,
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                      decoration: const InputDecoration(
                        labelText: 'Horas estimadas',
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'Prioridad: alta, media o baja. Estado inicial: sin empezar.',
                style: TextStyle(fontSize: 11),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancelar'),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Crear'),
          ),
        ],
      ),
    );
    final title = titleCtrl.text.trim();
    final estimate =
        double.tryParse(estimateCtrl.text.trim().replaceAll(',', '.')) ?? 0.0;
    titleCtrl.dispose();
    estimateCtrl.dispose();
    if (ok != true || !mounted) return;
    if (title.isEmpty || selectedUserId == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Título y responsable son requeridos.'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    setState(() => _creatingReal = true);
    try {
      var sheet = _activeSheet;
      if (sheet == null) {
        // La primera tarea conserva la compatibilidad con grupos sin hoja.
        final now = DateTime.now();
        sheet = await _social.saveSprintSheet(
          groupId: widget.groupId!.trim(),
          name: 'Sprint 1',
          start: _iso(now),
          end: _iso(now.add(const Duration(days: 4))),
        );
        _realSheets = [sheet];
        _realSheetIndex = 0;
      }
      await _social.createSprintTask(
        groupId: widget.groupId!.trim(),
        sheetId: sheet.id,
        title: title,
        assignedTo: selectedUserId!,
        priority: priority,
        status: status,
        estimatedHours: estimate,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Tarea creada.'),
          backgroundColor: AppColors.border,
        ),
      );
      await _loadReal();
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo crear la tarea.'),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _creatingReal = false);
    }
  }

  /// Edición de una fila de la tabla rica con persistencia real.
  /// Resuelve el responsable visible a su user_id vía [_memberById].
  Future<void> _editTableRow(_SprintTask row) async {
    if (!_isRealGroup || !row.isReal) return;
    final members = await _loadMembers();
    if (!mounted) return;
    if (members == null) return;
    final titleCtrl = TextEditingController(text: row.task);
    final estimateCtrl = TextEditingController(
      text: row.estimate.toStringAsFixed(1),
    );
    // userId estable: la lista puede reconstruirse sin romper la selección.
    String? selectedUserId;
    for (final m in members) {
      if (m.userId == _memberIdForDisplay(row.member)) {
        selectedUserId = m.userId;
      }
    }
    String priority = row.priority;
    String status = row.status;
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text(
          'EDITAR TAREA',
          style: TextStyle(fontWeight: FontWeight.w900),
        ),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: titleCtrl,
                decoration: const InputDecoration(
                  labelText: 'Título (requerido)',
                ),
              ),
              const SizedBox(height: 8),
              StatefulBuilder(
                builder: (context, setDialogState) => Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Text(
                      'Responsable (requerido)',
                      style: TextStyle(fontSize: 12),
                    ),
                    DropdownButton<String>(
                      value: selectedUserId,
                      hint: const Text('Elige un miembro'),
                      isExpanded: true,
                      items: [
                        for (final m in members)
                          DropdownMenuItem<String>(
                            value: m.userId,
                            child: Text(m.displayLabel),
                          ),
                      ],
                      onChanged: (v) =>
                          setDialogState(() => selectedUserId = v),
                    ),
                    const SizedBox(height: 8),
                    TextField(
                      controller: estimateCtrl,
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                      decoration: const InputDecoration(
                        labelText: 'Horas asignadas',
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancelar'),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Guardar'),
          ),
        ],
      ),
    );
    final title = titleCtrl.text.trim();
    titleCtrl.dispose();
    // Misma regla que al crear: número válido >= 0 (estimated_hours real).
    final estimate = double.tryParse(
      estimateCtrl.text.trim().replaceAll(',', '.'),
    );
    estimateCtrl.dispose();
    if (ok != true || !mounted) return;
    if (title.isEmpty || selectedUserId == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Título y responsable son requeridos.'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    if (estimate == null || !estimate.isFinite || estimate < 0) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Horas asignadas inválidas.'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    try {
      await _social.updateSprintTask(
        groupId: widget.groupId!.trim(),
        taskId: row.backendId!,
        title: title,
        assignedTo: selectedUserId!,
        priority: priority.toLowerCase(),
        status: _sprintStatusBackend(status),
        estimatedHours: estimate,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Tarea actualizada.'),
          backgroundColor: AppColors.border,
        ),
      );
      await _loadReal();
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo actualizar la tarea.'),
          backgroundColor: AppColors.error,
        ),
      );
    }
  }

  Future<void> _deleteTableRow(_SprintTask row) async {
    if (!_isRealGroup || !row.isReal) return;
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text(
          'ELIMINAR TAREA',
          style: TextStyle(fontWeight: FontWeight.w900),
        ),
        content: Text('¿Eliminar "${row.task}"?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancelar'),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Eliminar'),
          ),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    try {
      await _social.deleteSprintTask(
        groupId: widget.groupId!.trim(),
        taskId: row.backendId!,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Tarea eliminada.'),
          backgroundColor: AppColors.border,
        ),
      );
      await _loadReal();
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo eliminar la tarea.'),
          backgroundColor: AppColors.error,
        ),
      );
    }
  }

  static const List<String> _previewMembers = [
    'Sofía • MAT1002',
    'Matías • INF220',
    'Ana • INF-360',
    'Tú • INF-360',
  ];

  static final List<_Sprint> _sprints = _seedSprints();
  static int _activeSprint = 0;

  static String _fmt(DateTime d) =>
      '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year.toString().substring(2)}';

  static DateTime _nextTuesday(DateTime now) {
    var d = DateTime(now.year, now.month, now.day);
    while (d.weekday != DateTime.tuesday) {
      d = DateTime(d.year, d.month, d.day + 1);
    }
    return d;
  }

  static List<String> _weekDatesForSprint(int index) {
    final base = _nextTuesday(DateTime.now()).add(Duration(days: 7 * index));
    return [for (var i = 0; i < 4; i++) _fmt(base.add(Duration(days: i)))];
  }

  static List<_Sprint> _seedSprints() {
    const names = ['Martes', 'Miércoles', 'Jueves', 'Viernes'];

    final sprint1Dates = ['05/11/26', '06/11/26', '07/11/26', '08/11/26'];
    final sprint2Dates = _weekDatesForSprint(1);
    final sprint3Dates = _weekDatesForSprint(2);

    Map<String, double> d(Map<String, double> v) => Map.of(v);

    return [
      _Sprint(
        name: 'Sprint 1',
        goal: 'Avance de proyectos y preparación de evaluaciones.',
        start: sprint1Dates.first,
        end: sprint1Dates.last,
        days: names,
        dayDates: sprint1Dates,
        tasks: [
          _SprintTask(
            member: 'Sofía • MAT1002',
            task: 'Investigar derivadas parciales',
            priority: 'alta',
            status: 'En proceso',
            estimate: 5.0,
            days: d({
              'Martes': 2.0,
              'Miércoles': 1.0,
              'Jueves': 0.0,
              'Viernes': 0.5,
            }),
          ),
          _SprintTask(
            member: 'Sofía • MAT1002',
            task: 'Resumen de apuntes SQL',
            priority: 'media',
            status: 'Listo',
            estimate: 3.0,
            days: d({
              'Martes': 1.5,
              'Miércoles': 1.5,
              'Jueves': 0.0,
              'Viernes': 0.0,
            }),
          ),
          _SprintTask(
            member: 'Matías • INF220',
            task: 'Mapa OSI - presentación',
            priority: 'alta',
            status: 'Sin empezar',
            estimate: 4.0,
            days: d({
              'Martes': 0.0,
              'Miércoles': 0.0,
              'Jueves': 2.0,
              'Viernes': 1.0,
            }),
          ),
          _SprintTask(
            member: 'Ana • INF-360',
            task: 'Revisión de ejercicios',
            priority: 'baja',
            status: 'Pendiente',
            estimate: 2.0,
            days: d({
              'Martes': 0.0,
              'Miércoles': 0.0,
              'Jueves': 0.0,
              'Viernes': 1.0,
            }),
          ),
          _SprintTask(
            member: 'Tú • INF-360',
            task: 'Setup Drift FTS5',
            priority: 'alta',
            status: 'En proceso',
            estimate: 6.0,
            days: d({
              'Martes': 2.0,
              'Miércoles': 2.0,
              'Jueves': 1.0,
              'Viernes': 0.0,
            }),
          ),
        ],
      ),
      _Sprint(
        name: 'Sprint 2',
        goal: 'Foco backend y pull requests pendientes.',
        start: sprint2Dates.first,
        end: sprint2Dates.last,
        days: names,
        dayDates: sprint2Dates,
        tasks: [
          _SprintTask(
            member: 'Tú • INF-360',
            task: 'Endpoint POST /notes',
            priority: 'alta',
            status: 'Sin empezar',
            estimate: 4.0,
            days: d({
              'Martes': 0.0,
              'Miércoles': 0.0,
              'Jueves': 0.0,
              'Viernes': 0.0,
            }),
          ),
          _SprintTask(
            member: 'Ana • INF-360',
            task: 'QA de login Google',
            priority: 'media',
            status: 'En proceso',
            estimate: 3.0,
            days: d({
              'Martes': 1.0,
              'Miércoles': 0.5,
              'Jueves': 0.0,
              'Viernes': 0.0,
            }),
          ),
        ],
      ),
      _Sprint(
        name: 'Sprint 3',
        goal: 'Reservado.',
        start: sprint3Dates.first,
        end: sprint3Dates.last,
        days: names,
        dayDates: sprint3Dates,
        tasks: [],
      ),
    ];
  }

  _Sprint get _sprint {
    if (_isRealGroup) return _realSprintView ?? _emptyRealSprint();
    return _sprints[_activeSprint];
  }

  // Carcasa vacía para grupo real sin tareas: misma estructura, sin filas.
  _Sprint _emptyRealSprint() {
    final sheets = _realSheets;
    final sheet = sheets.isNotEmpty
        ? sheets[_realSheetIndex.clamp(0, sheets.length - 1)]
        : null;
    final days = _realDayNames(sheet);
    final isoByDay = _realDayIso(sheet);
    return _Sprint(
      name: sheet?.name ?? 'Hoja de sprint',
      goal: '',
      start: _fmtDateLabel(sheet?.periodStart),
      end: _fmtDateLabel(sheet?.periodEnd),
      days: days,
      dayDates: [
        for (final iso in isoByDay)
          () {
            final d = DateTime.tryParse(iso);
            if (d == null) return '';
            return '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}';
          }(),
      ],
      dayIsoDates: isoByDay,
      tasks: const [],
    );
  }

  /// true cuando la tabla muestra datos del backend (incluye grupo vacío:
  /// la carcasa real siempre se muestra para grupo real sin error).
  bool get _usingRealTable => _isRealGroup && _realError == null;

  /// Miembros para agrupar la tabla: nombres reales con grupo, mocks solo
  /// en vista previa local.
  List<String> get _members {
    if (_usingRealTable) {
      final names = <String>[];
      for (final t in _sprint.tasks) {
        if (!names.contains(t.member)) names.add(t.member);
      }
      if (names.isEmpty) {
        // Grupo vacío: carcasa visible sin filas (mensaje integrado abajo).
        return const [];
      }
      return names;
    }
    return _previewMembers;
  }

  List<String> get _days => _sprint.days;
  List<String> get _dayKeys => _usingRealTable ? _sprint.dayIsoDates : _days;
  List<String> get _dayDates => _sprint.dayDates;
  List<_SprintTask> get _tasks => _sprint.tasks;

  double get totalEst => _sprint.totalEstimate;
  double get totalUsed => _sprint.totalUsed;
  double get totalRemaining => _sprint.totalRemaining;
  double get progressPercent {
    if (totalEst == 0) return 0;
    return (totalUsed / totalEst * 100).clamp(0.0, 100.0);
  }

  Color _statusColor(String s) => _statusChipColor(s);

  // -------------------------------------------------------------------------
  // ACCIONES
  // -------------------------------------------------------------------------

  void _selectSprint(int index) {
    setState(() {
      _activeSprint = index;
    });
  }

  Future<void> _showNewSprintDialog() async {
    final nameCtrl = TextEditingController(
      text: 'Sprint ${_sprints.length + 1}',
    );
    final goalCtrl = TextEditingController();

    await showDialog<void>(
      context: context,
      barrierColor: AppColors.scrim,
      builder: (dialogContext) =>
          _NewSprintDialog(nameController: nameCtrl, goalController: goalCtrl),
    );

    if (!mounted) return;
    final name = nameCtrl.text.trim();
    if (name.isEmpty) return;
    final goal = goalCtrl.text.trim();

    final index = _sprints.length;
    final dates = _weekDatesForSprint(index);
    setState(() {
      _sprints.add(
        _Sprint(
          name: name,
          goal: goal.isEmpty ? 'Nuevo sprint.' : goal,
          start: dates.first,
          end: dates.last,
          days: List.of(_sprint.days),
          dayDates: dates,
          tasks: [],
        ),
      );
      _activeSprint = index;
    });
  }

  Future<void> _showAddTaskDialog({String? member}) async {
    // En tabla real se crea por backend con selector de miembros.
    // Un solo botón NUEVA TAREA arriba; sin + por usuario.
    if (_isRealGroup) {
      GroupMember? preselect;
      if (member != null) {
        for (final m in _memberById.values) {
          if (m.displayLabel == member) preselect = m;
        }
      }
      await _createRealTask(preselect);
      return;
    }
    final result = await showDialog<_TaskDraft>(
      context: context,
      barrierColor: AppColors.scrim,
      builder: (_) => _NewTaskDialog(members: _members, initialMember: member),
    );
    if (result == null || !mounted) return;

    setState(() {
      _tasks.add(
        _SprintTask(
          member: result.member,
          task: result.task,
          priority: result.priority,
          status: result.status,
          estimate: result.estimate,
          days: {for (final day in _days) day: 0.0},
        ),
      );
    });
  }

  Future<void> _showImputeHours(_SprintTask task, String day) async {
    // En fila real se imputa por backend con la fecha ISO de la columna.
    if (_usingRealTable && task.isReal) {
      await _imputeRealHours(task, day);
      return;
    }
    final current = task.days[day] ?? 0.0;
    final value = await showDialog<double>(
      context: context,
      barrierColor: AppColors.scrim,
      builder: (_) => _ImputeHoursDialog(
        taskTitle: task.task,
        day: day,
        initial: current,
        estimate: task.estimate,
      ),
    );
    if (value == null || !mounted) return;

    setState(() {
      task.days[day] = value.clamp(0.0, 999.0);
    });
  }

  /// Imputa horas reales en la fecha ISO de la columna.
  Future<void> _imputeRealHours(_SprintTask task, String day) async {
    final iso = day;
    if (iso.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo determinar la fecha de la columna.'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    final current = task.days[day] ?? 0.0;
    final value = await showDialog<double>(
      context: context,
      barrierColor: AppColors.scrim,
      builder: (_) => _ImputeHoursDialog(
        taskTitle: task.task,
        day: day,
        initial: current,
        estimate: task.estimate,
      ),
    );
    if (value == null || !mounted) return;
    try {
      await _social.logHours(
        taskId: task.backendId!,
        logDate: iso,
        hours: value,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Horas registradas.'),
          backgroundColor: AppColors.border,
        ),
      );
      await _loadReal();
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudieron registrar las horas.'),
          backgroundColor: AppColors.error,
        ),
      );
    }
  }

  void _changeTaskStatus(_SprintTask task, String status) {
    // En fila real persiste por PATCH (etiqueta -> código backend).
    if (_usingRealTable && task.isReal) {
      _patchTableRow(task, status: _sprintStatusBackend(status));
      return;
    }
    setState(() => task.status = status);
  }

  void _changeTaskPriority(_SprintTask task, String priority) {
    // En fila real persiste por PATCH (minúsculas: alta|media|baja).
    if (_usingRealTable && task.isReal) {
      _patchTableRow(task, priority: priority.toLowerCase());
      return;
    }
    setState(() => task.priority = priority);
  }

  /// PATCH parcial de una fila real + recarga.
  Future<void> _patchTableRow(
    _SprintTask task, {
    String? title,
    String? assignedTo,
    String? priority,
    String? status,
  }) async {
    try {
      await _social.updateSprintTask(
        groupId: widget.groupId!.trim(),
        taskId: task.backendId!,
        title: title,
        assignedTo: assignedTo,
        priority: priority,
        status: status,
      );
      if (!mounted) return;
      await _loadReal();
    } on SocialApiException catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo actualizar la tarea.'),
          backgroundColor: AppColors.error,
        ),
      );
      await _loadReal();
    }
  }

  // -------------------------------------------------------------------------
  // BUILD
  // -------------------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    // Todos los grupos muestran la misma carcasa: con datos o vacía con
    // mensaje integrado. Sin cabecera blanca duplicada en modo real:
    // NUEVA TAREA + RECARGAR viven en el encabezado rico.
    final showRichTable =
        !_isRealGroup || (_isRealGroup && !_loadingReal && _realError == null);
    // Solo estados que necesitan panel: sin grupo, carga o error.
    final showPanel = !_isRealGroup || _loadingReal || _realError != null;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SingleChildScrollView(
        scrollDirection: Axis.vertical,
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (showPanel) _buildRealPanel(),
            if (showRichTable) ...[
              // Con grupo: tabla rica con datos reales. Sin grupo: la misma
              // tabla como vista previa local de ejemplo, rotulada.
              if (showPanel) const SizedBox(height: 12),
              _buildHeader(),
              const SizedBox(height: 12),
              _buildSprintSelector(),
              const SizedBox(height: 16),
              _buildMetricsBar(),
              // Leyenda de estados solo en vista previa: en grupo real el
              // estado ya se ve y edita por fila (no es filtro ni botón).
              if (!_isRealGroup) ...[
                const SizedBox(height: 12),
                _buildLegendBar(),
                const SizedBox(height: 8),
                const Text(
                  'Vista previa local: datos de ejemplo.',
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    color: AppColors.mutedStrong,
                  ),
                ),
              ],
              const SizedBox(height: 8),
              if (_isRealGroup && _tasks.isEmpty)
                _buildSprintTable()
              else
                Scrollbar(
                  key: const ValueKey('sprint-date-scrollbar'),
                  controller: _tableScroll,
                  thumbVisibility: true,
                  trackVisibility: true,
                  scrollbarOrientation: ScrollbarOrientation.bottom,
                  child: SingleChildScrollView(
                    controller: _tableScroll,
                    scrollDirection: Axis.horizontal,
                    padding: const EdgeInsets.only(bottom: 18),
                    child: SizedBox(
                      width: math.max(1200, 740 + 90.0 * _days.length),
                      child: _buildSprintTable(),
                    ),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }

  // Panel mínimo: sin grupo, carga o error. En modo real con datos o vacío
  // no se muestra (la carcasa rica + su mensaje integrado bastan).
  Widget _buildRealPanel() {
    if (!_isRealGroup) {
      return Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
        ),
        child: const Text(
          'Selecciona un grupo para ver su hoja de sprint.',
          style: TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.w700,
            color: AppColors.mutedStrong,
          ),
        ),
      );
    }
    if (_loadingReal) {
      return const Center(
        child: SizedBox(
          width: 22,
          height: 22,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
      );
    }
    return const Text(
      'No se pudo cargar la hoja de sprint.',
      style: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w700,
        color: AppColors.error,
      ),
    );
  }

  Widget _buildHeader() {
    // En modo real el encabezado rico integra el único NUEVA TAREA + Recargar.
    if (_isRealGroup) {
      return Row(
        children: [
          const Expanded(
            child: Text(
              'HOJA DE SPRINT',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
                letterSpacing: -0.5,
              ),
            ),
          ),
          NeobrutalistButton(
            label: _creatingReal ? 'Creando...' : 'Nueva tarea',
            icon: Icons.add_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: _creatingReal ? null : () => _createRealTask(),
          ),
          const SizedBox(width: 8),
          NeobrutalistButton(
            label: 'Recargar',
            icon: Icons.refresh_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: _loadingReal ? null : _loadReal,
          ),
        ],
      );
    }
    return Row(
      children: [
        const Expanded(
          child: Text(
            'HOJA DE SPRINT',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
              letterSpacing: -0.5,
            ),
          ),
        ),
        NeobrutalistButton(
          label: 'AGREGAR TAREA',
          icon: Icons.add_rounded,
          variant: NeobrutalistButtonVariant.accent,
          onPressed: () => _showAddTaskDialog(),
        ),
      ],
    );
  }

  Widget _buildSprintSelector() {
    if (_usingRealTable) return _buildRealSheetSelector();
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      physics: const BouncingScrollPhysics(),
      child: Row(
        children: [
          for (var i = 0; i < _sprints.length; i++) ...[
            if (i != 0) const SizedBox(width: 8),
            _SprintChip(
              label: _sprints[i].name,
              dates: '${_sprints[i].start} → ${_sprints[i].end}',
              selected: i == _activeSprint,
              taskCount: _sprints[i].tasks.length,
              onTap: () => _selectSprint(i),
            ),
          ],
          const SizedBox(width: 8),
          NeobrutalistButton(
            label: 'NUEVO SPRINT',
            icon: Icons.add_circle_outline_rounded,
            variant: NeobrutalistButtonVariant.primary,
            onPressed: _showNewSprintDialog,
          ),
          const SizedBox(width: 8),
          NeobrutalistButton(
            label: 'VER GRÁFICO BURNDOWN',
            icon: Icons.show_chart_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: _showBurndownDialog,
          ),
        ],
      ),
    );
  }

  /// Acciones visibles para todas las hojas, incluida la primera sin fechas.
  Widget _buildRealSheetSelector() {
    final sheets = _realSheets;
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (var i = 0; i < sheets.length; i++) ...[
          if (i != 0) const SizedBox(width: 8),
          _SprintChip(
            label: sheets[i].name,
            dates:
                '${_fmtDateLabel(sheets[i].periodStart)} → ${_fmtDateLabel(sheets[i].periodEnd)}',
            selected: i == _realSheetIndex,
            taskCount: _realTasks
                .where((t) => t.sheetId == sheets[i].id)
                .length,
            onTap: () => setState(() => _realSheetIndex = i),
          ),
        ],
        const SizedBox(width: 8),
        NeobrutalistButton(
          label: '+ NUEVO SPRINT',
          icon: Icons.add_circle_outline_rounded,
          onPressed: _loadingReal ? null : () => _manageSprint(),
        ),
        if (_activeSheet != null) ...[
          const SizedBox(width: 8),
          NeobrutalistButton(
            label: 'EDITAR SPRINT',
            icon: Icons.edit_outlined,
            onPressed: _loadingReal ? null : () => _manageSprint(edit: true),
          ),
        ],
        const SizedBox(width: 8),
        NeobrutalistButton(
          label: 'VER GRÁFICO BURNDOWN',
          icon: Icons.show_chart_rounded,
          variant: NeobrutalistButtonVariant.accent,
          onPressed: _showBurndownDialog,
        ),
      ],
    );
  }

  double get totalExceeded => _sprint.totalExceeded;

  Widget _buildMetricsBar() {
    // HORAS ASIGNADAS/ESTIMADAS + USADAS + RESTANTES. Si used > estimated se
    // muestra "0h restantes" + "Xh excedidas" (solo presentación).
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 2),
        boxShadow: AppShadows.card,
      ),
      child: Wrap(
        spacing: 10,
        runSpacing: 10,
        children: [
          _MetricTile(
            label: 'CONFIGURACIÓN',
            value: '${_sprint.name} • ${_days.length} días',
            bg: AppColors.accentYellow,
          ),
          _MetricTile(
            label: 'HORAS ASIGNADAS',
            value: '${totalEst.toStringAsFixed(1)}h',
            bg: AppColors.surfaceLow,
          ),
          _MetricTile(
            label: 'HORAS USADAS',
            value: '${totalUsed.toStringAsFixed(1)}h',
            bg: const Color(0xFFD6E3FF),
          ),
          _MetricTile(
            label: 'HORAS RESTANTES',
            value: totalExceeded > 0
                ? '0.0h restantes · ${totalExceeded.toStringAsFixed(1)}h excedidas'
                : '${totalRemaining.toStringAsFixed(1)}h restantes',
            bg: AppColors.subjectPeach,
          ),
        ],
      ),
    );
  }

  void _showBurndownDialog() {
    final chart = _BurndownChart(
      dayLabels: _dayDates,
      cumulativeUsed: _cumulativeUsedByDay(),
      totalEstimate: totalEst,
      achievements: {
        'Sprint': _sprint.name,
        'Objetivo': _sprint.goal,
        'Días': '${_days.length}',
        'Tareas': '${_tasks.length}',
      },
      totalEst: totalEst,
      totalUsed: totalUsed,
      totalRemaining: totalRemaining,
      progressPercent: progressPercent,
    );

    showDialog<void>(
      context: context,
      barrierColor: AppColors.scrim,
      builder: (dialogContext) => Dialog(
        backgroundColor: AppColors.surface,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.zero,
          side: BorderSide(
            color: AppColors.border,
            width: AppDimens.borderWidthThick,
          ),
        ),
        insetPadding: const EdgeInsets.all(16),
        child: SingleChildScrollView(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 820),
            child: Padding(
              padding: const EdgeInsets.all(14),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Row(
                    children: [
                      const Expanded(
                        child: Text(
                          'GRÁFICO BURNDOWN',
                          style: TextStyle(
                            fontWeight: FontWeight.w900,
                            fontSize: 13,
                            color: AppColors.text,
                            letterSpacing: 0.3,
                          ),
                        ),
                      ),
                      NeobrutalistIconButton(
                        icon: Icons.close_rounded,
                        tooltip: 'Cerrar',
                        onPressed: () => Navigator.of(dialogContext).pop(),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  const Divider(color: AppColors.border, thickness: 2),
                  const SizedBox(height: 10),
                  chart,
                  // Sin leyenda en grupo real (redundante con la tabla).
                  if (!_isRealGroup) ...[
                    const SizedBox(height: 12),
                    _buildLegendBar(),
                  ],
                  const SizedBox(height: 14),
                  Center(
                    child: NeobrutalistButton(
                      label: 'CERRAR',
                      icon: Icons.close_rounded,
                      variant: NeobrutalistButtonVariant.secondary,
                      onPressed: () => Navigator.of(dialogContext).pop(),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  List<double> _cumulativeUsedByDay() {
    final result = <double>[];
    var acc = 0.0;
    for (final day in _dayKeys) {
      acc += _tasks.fold(0.0, (sum, t) => sum + (t.days[day] ?? 0.0));
      result.add(acc);
    }
    return result;
  }

  Widget _buildLegendBar() {
    return _LegendBar(
      items: [
        _LegendChip(color: _statusColor('En proceso'), label: 'En proceso'),
        _LegendChip(color: _statusColor('Sin empezar'), label: 'Sin empezar'),
        _LegendChip(color: _statusColor('Listo'), label: 'Listo'),
        _LegendChip(color: _statusColor('Pendiente'), label: 'Pendiente'),
      ],
    );
  }

  Widget _buildSprintTable() {
    // Grupo vacío: misma carcasa con mensaje integrado (no caja mínima).
    if (_isRealGroup && _tasks.isEmpty) {
      return Container(
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border, width: 2),
          boxShadow: AppShadows.card,
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // Cabecera única fecha/día (ej. 29/09 MARTES) sin duplicar.
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              decoration: const BoxDecoration(
                color: AppColors.accentYellow,
                border: Border(
                  bottom: BorderSide(color: AppColors.border, width: 2),
                ),
              ),
              child: Text(
                '${_sprint.name.toUpperCase()} • ${_sprint.start} → ${_sprint.end}',
                style: const TextStyle(
                  fontWeight: FontWeight.w900,
                  fontSize: 11,
                  color: AppColors.text,
                ),
              ),
            ),
            const Padding(
              padding: EdgeInsets.all(20),
              child: Text(
                'Aún no hay tareas en este sprint.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                  color: AppColors.mutedStrong,
                ),
              ),
            ),
          ],
        ),
      );
    }
    final grouped = <String, List<_SprintTask>>{};
    for (final m in _members) {
      grouped[m] = _tasks.where((t) => t.member == m).toList();
    }

    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 2),
        boxShadow: AppShadows.card,
      ),
      child: Table(
        border: TableBorder.all(color: AppColors.border, width: 2),
        defaultColumnWidth: const FixedColumnWidth(90),
        columnWidths: const {
          0: FixedColumnWidth(220),
          1: FixedColumnWidth(90),
          2: FixedColumnWidth(110),
          3: FixedColumnWidth(110),
          4: FixedColumnWidth(100),
          5: FixedColumnWidth(110),
          6: FixedColumnWidth(90),
          7: FixedColumnWidth(90),
          8: FixedColumnWidth(90),
          9: FixedColumnWidth(90),
        },
        children: [
          // Header fila 1: tareas/prioridad/estado/horas en amarillo técnico.
          TableRow(
            children: [
              _HeaderCellFixed('TAREAS', 220, bg: AppColors.accentYellow),
              _HeaderCellFixed('PRIORIDAD', 90, bg: AppColors.accentYellow),
              _HeaderCellFixed('ESTADO', 110, bg: AppColors.accentYellow),
              _HeaderCellFixed('HORAS ASIG.', 110, bg: AppColors.accentYellow),
              _HeaderCellFixed('HORAS USADAS', 100, bg: AppColors.accentYellow),
              _HeaderCellFixed('HORAS REST.', 110, bg: AppColors.accentYellow),
              for (int i = 0; i < _days.length; i++)
                Container(
                  height: 28,
                  color: AppColors.surfaceLow,
                  alignment: Alignment.center,
                  child: Text(
                    _dayDates[i],
                    style: const TextStyle(
                      fontFamily: 'monospace',
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: AppColors.text,
                    ),
                  ),
                ),
            ],
          ),
          // Header fila 2: días en tinta con texto blanco.
          TableRow(
            children: [
              Container(height: 26, color: AppColors.accentYellow),
              Container(height: 26, color: AppColors.accentYellow),
              Container(height: 26, color: AppColors.accentYellow),
              Container(height: 26, color: AppColors.accentYellow),
              Container(height: 26, color: AppColors.accentYellow),
              Container(height: 26, color: AppColors.accentYellow),
              for (final d in _days)
                Container(
                  height: 26,
                  color: AppColors.border,
                  alignment: Alignment.center,
                  child: Text(
                    d.toUpperCase(),
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: AppColors.surface,
                    ),
                  ),
                ),
            ],
          ),
          // Filas por integrante
          for (final member in _members) ...[
            TableRow(
              decoration: const BoxDecoration(color: AppColors.surfaceLow),
              children: [
                Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 5,
                  ),
                  child: Row(
                    children: [
                      Expanded(
                        child: Text(
                          member,
                          style: const TextStyle(
                            fontWeight: FontWeight.w900,
                            fontSize: 11,
                            color: AppColors.text,
                          ),
                        ),
                      ),
                      // En modo real el alta es única (NUEVA TAREA arriba).
                      if (!_isRealGroup)
                        NeobrutalistIconButton(
                          icon: Icons.add_rounded,
                          size: 30,
                          variant: NeobrutalistButtonVariant.accent,
                          tooltip: 'Agregar tarea',
                          onPressed: () => _showAddTaskDialog(member: member),
                        ),
                    ],
                  ),
                ),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                for (final _ in _days) const SizedBox(height: 32),
              ],
            ),
            for (final t in grouped[member]!)
              TableRow(
                children: [
                  Padding(
                    padding: const EdgeInsets.all(6),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            t.task,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontWeight: FontWeight.w700,
                              fontSize: 10,
                              color: AppColors.text,
                            ),
                          ),
                        ),
                        if (t.isReal)
                          PopupMenuButton<String>(
                            icon: const Icon(Icons.more_vert_rounded, size: 16),
                            onSelected: (value) {
                              if (value == 'edit') {
                                _editTableRow(t);
                              } else if (value == 'delete') {
                                _deleteTableRow(t);
                              }
                            },
                            itemBuilder: (_) => const [
                              PopupMenuItem(
                                value: 'edit',
                                child: Text('Editar'),
                              ),
                              PopupMenuItem(
                                value: 'delete',
                                child: Text('Eliminar'),
                              ),
                            ],
                          ),
                      ],
                    ),
                  ),
                  Center(
                    child: _PriorityChip(
                      priority: t.priority,
                      onChanged: (v) => _changeTaskPriority(t, v),
                    ),
                  ),
                  Center(
                    child: _StatusChip(
                      status: t.status,
                      color: _statusColor(t.status),
                      onChanged: (v) => _changeTaskStatus(t, v),
                    ),
                  ),
                  Center(
                    child: Text(
                      '${t.estimate.toStringAsFixed(1)}h',
                      style: const TextStyle(
                        fontWeight: FontWeight.w800,
                        fontSize: 10,
                      ),
                    ),
                  ),
                  Center(
                    child: Text(
                      '${t.used.toStringAsFixed(1)}h',
                      style: const TextStyle(
                        fontWeight: FontWeight.w800,
                        fontSize: 10,
                      ),
                    ),
                  ),
                  Center(
                    child: Text(
                      t.exceeded > 0
                          ? '0.0h\n${t.exceeded.toStringAsFixed(1)}h excedidas'
                          : '${t.remaining.toStringAsFixed(1)}h restantes',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontWeight: FontWeight.w800,
                        fontSize: 10,
                        color: t.exceeded > 0
                            ? AppColors.error
                            : (t.remaining == 0
                                  ? AppColors.successDeep
                                  : AppColors.text),
                      ),
                    ),
                  ),
                  for (final day in _dayKeys)
                    _DayCell(
                      value: t.days[day] ?? 0.0,
                      onTap: () => _showImputeHours(t, day),
                    ),
                ],
              ),
          ],
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// DIALOGOS
// ---------------------------------------------------------------------------

class _SprintChip extends StatelessWidget {
  const _SprintChip({
    required this.label,
    required this.dates,
    required this.selected,
    required this.taskCount,
    required this.onTap,
  });

  final String label;
  final String dates;
  final bool selected;
  final int taskCount;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ClickCursor(
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: AnimatedContainer(
          duration: AppMotion.fast,
          curve: AppMotion.standard,
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: BoxDecoration(
            color: selected ? AppColors.accentYellow : AppColors.surface,
            border: Border.all(color: AppColors.border, width: 2),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: selected ? AppShadows.button : const <BoxShadow>[],
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                label.toUpperCase(),
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w900,
                  color: selected ? AppColors.text : AppColors.muted,
                ),
              ),
              const SizedBox(width: 8),
              NeobrutalistBadge(
                label: taskCount == 0 ? 'VACÍO' : '$taskCount TAREAS',
                tone: selected
                    ? NeobrutalistTone.neutral
                    : NeobrutalistTone.accent,
                compact: true,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _TaskDraft {
  _TaskDraft({
    required this.member,
    required this.task,
    required this.priority,
    required this.status,
    required this.estimate,
  });

  final String member;
  final String task;
  final String priority;
  final String status;
  final double estimate;
}

class _NewTaskDialog extends StatefulWidget {
  const _NewTaskDialog({required this.members, this.initialMember});

  final List<String> members;
  final String? initialMember;

  @override
  State<_NewTaskDialog> createState() => _NewTaskDialogState();
}

class _NewTaskDialogState extends State<_NewTaskDialog> {
  late String _selectedMember;
  final _titleController = TextEditingController();
  final _hoursController = TextEditingController(text: '2.0');
  String _priority = 'media';
  String _status = 'Sin empezar';
  String? _hourError;

  @override
  void initState() {
    super.initState();
    _selectedMember = widget.initialMember ?? widget.members.first;
  }

  @override
  void dispose() {
    _titleController.dispose();
    _hoursController.dispose();
    super.dispose();
  }

  void _submit() {
    final title = _titleController.text.trim();
    final hours = double.tryParse(
      _hoursController.text.trim().replaceAll(',', '.'),
    );
    if (title.isEmpty) return;
    if (hours == null || hours <= 0 || hours > 99) {
      setState(() => _hourError = 'Horas debe ser mayor a 0.');
      return;
    }
    Navigator.pop(
      context,
      _TaskDraft(
        member: _selectedMember,
        task: title,
        priority: _priority,
        status: _status,
        estimate: hours,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppColors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.zero,
        side: BorderSide(
          color: AppColors.border,
          width: AppDimens.borderWidthThick,
        ),
      ),
      title: const Text(
        'NUEVA TAREA',
        style: TextStyle(
          fontWeight: FontWeight.w900,
          fontSize: 13,
          color: AppColors.text,
        ),
      ),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _FieldLabel('INTEGRANTE'),
            const SizedBox(height: 4),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
              decoration: BoxDecoration(
                color: AppColors.bg,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(AppDimens.radius),
              ),
              child: DropdownButtonHideUnderline(
                child: DropdownButton<String>(
                  value: _selectedMember,
                  isExpanded: true,
                  isDense: true,
                  icon: const Icon(Icons.expand_more_rounded),
                  style: const TextStyle(
                    fontWeight: FontWeight.w800,
                    fontSize: 12,
                    color: AppColors.text,
                  ),
                  items: widget.members
                      .map(
                        (member) => DropdownMenuItem<String>(
                          value: member,
                          child: Text(member, overflow: TextOverflow.ellipsis),
                        ),
                      )
                      .toList(),
                  onChanged: (value) {
                    if (value != null) {
                      setState(() => _selectedMember = value);
                    }
                  },
                ),
              ),
            ),
            const SizedBox(height: 12),
            _FieldLabel('TÍTULO'),
            const SizedBox(height: 4),
            TextField(
              controller: _titleController,
              autofocus: true,
              style: const TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 13,
                color: AppColors.text,
              ),
              decoration: appInputDecoration('Ej: Endpoint POST /notes'),
            ),
            const SizedBox(height: 12),
            _FieldLabel('PRIORIDAD'),
            const SizedBox(height: 4),
            _SegmentedPicker(
              options: const ['alta', 'media', 'baja'],
              value: _priority,
              onChanged: (v) => setState(() => _priority = v),
            ),
            const SizedBox(height: 12),
            _FieldLabel('HORAS ESTIMADAS'),
            const SizedBox(height: 4),
            TextField(
              controller: _hoursController,
              keyboardType: const TextInputType.numberWithOptions(
                decimal: true,
              ),
              inputFormatters: [FilteringTextInputFormatter.allow(_hoursRegex)],
              style: const TextStyle(
                fontWeight: FontWeight.w800,
                fontSize: 13,
                color: AppColors.text,
              ),
              decoration: appInputDecoration(
                'Ej: 4.0',
              ).copyWith(errorText: _hourError),
            ),
            const SizedBox(height: 12),
            _FieldLabel('ESTADO'),
            const SizedBox(height: 4),
            _SegmentedPicker(
              options: const ['Sin empezar', 'En proceso', 'Listo'],
              value: _status,
              onChanged: (v) => setState(() => _status = v),
            ),
          ],
        ),
      ),
      actionsPadding: const EdgeInsets.fromLTRB(18, 0, 18, 14),
      actions: [
        NeobrutalistButton(
          label: 'Cancelar',
          variant: NeobrutalistButtonVariant.secondary,
          onPressed: () => Navigator.pop(context),
        ),
        NeobrutalistButton(
          label: 'Agregar',
          icon: Icons.add_rounded,
          variant: NeobrutalistButtonVariant.accent,
          onPressed: _submit,
        ),
      ],
    );
  }
}

class _NewSprintDialog extends StatefulWidget {
  const _NewSprintDialog({
    required this.nameController,
    required this.goalController,
  });

  final TextEditingController nameController;
  final TextEditingController goalController;

  @override
  State<_NewSprintDialog> createState() => _NewSprintDialogState();
}

class _NewSprintDialogState extends State<_NewSprintDialog> {
  String? _nameError;

  void _submit() {
    if (widget.nameController.text.trim().isEmpty) {
      setState(() => _nameError = 'El nombre es obligatorio.');
      return;
    }
    Navigator.pop(context);
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppColors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.zero,
        side: BorderSide(
          color: AppColors.border,
          width: AppDimens.borderWidthThick,
        ),
      ),
      title: const Text(
        'NUEVO SPRINT',
        style: TextStyle(
          fontWeight: FontWeight.w900,
          fontSize: 13,
          color: AppColors.text,
        ),
      ),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _FieldLabel('NOMBRE'),
            const SizedBox(height: 4),
            TextField(
              controller: widget.nameController,
              autofocus: true,
              style: const TextStyle(
                fontWeight: FontWeight.w800,
                fontSize: 13,
                color: AppColors.text,
              ),
              decoration: appInputDecoration(
                'Ej: Sprint 4',
              ).copyWith(errorText: _nameError),
            ),
            const SizedBox(height: 12),
            _FieldLabel('OBJETIVO (OPCIONAL)'),
            const SizedBox(height: 4),
            TextField(
              controller: widget.goalController,
              style: const TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 13,
                color: AppColors.text,
              ),
              decoration: appInputDecoration('Ej: Terminar módulo de notas'),
            ),
          ],
        ),
      ),
      actionsPadding: const EdgeInsets.fromLTRB(18, 0, 18, 14),
      actions: [
        NeobrutalistButton(
          label: 'Cancelar',
          variant: NeobrutalistButtonVariant.secondary,
          onPressed: () => Navigator.pop(context),
        ),
        NeobrutalistButton(
          label: 'Crear',
          icon: Icons.check_rounded,
          variant: NeobrutalistButtonVariant.accent,
          onPressed: _submit,
        ),
      ],
    );
  }
}

class _ImputeHoursDialog extends StatefulWidget {
  const _ImputeHoursDialog({
    required this.taskTitle,
    required this.day,
    required this.initial,
    required this.estimate,
  });

  final String taskTitle;
  final String day;
  final double initial;
  final double estimate;

  @override
  State<_ImputeHoursDialog> createState() => _ImputeHoursDialogState();
}

class _ImputeHoursDialogState extends State<_ImputeHoursDialog> {
  late final TextEditingController _controller = TextEditingController(
    text: widget.initial.toStringAsFixed(1),
  );

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _quick(double hours) {
    setState(() => _controller.text = hours.toStringAsFixed(1));
  }

  void _submit() {
    final hours = double.tryParse(_controller.text.trim().replaceAll(',', '.'));
    if (hours == null || hours < 0) return;
    Navigator.pop(context, hours.clamp(0.0, 999.0));
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppColors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.zero,
        side: BorderSide(
          color: AppColors.border,
          width: AppDimens.borderWidthThick,
        ),
      ),
      title: const Text(
        'IMPUTAR HORAS REALES',
        style: TextStyle(
          fontWeight: FontWeight.w900,
          fontSize: 13,
          color: AppColors.text,
        ),
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            '${widget.taskTitle.toUpperCase()} • ${widget.day.toUpperCase()}',
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w900,
              color: AppColors.muted,
            ),
          ),
          const SizedBox(height: 10),
          TextField(
            controller: _controller,
            autofocus: true,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            inputFormatters: [FilteringTextInputFormatter.allow(_hoursRegex)],
            style: const TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 15,
              color: AppColors.text,
            ),
            decoration: appInputDecoration('Horas'),
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              _QuickHourChip(label: '0', onTap: () => _quick(0)),
              _QuickHourChip(label: '0.5', onTap: () => _quick(0.5)),
              _QuickHourChip(label: '1', onTap: () => _quick(1)),
              _QuickHourChip(label: '2', onTap: () => _quick(2)),
              _QuickHourChip(label: '4', onTap: () => _quick(4)),
            ],
          ),
        ],
      ),
      actionsPadding: const EdgeInsets.fromLTRB(18, 0, 18, 14),
      actions: [
        NeobrutalistButton(
          label: 'Cancelar',
          variant: NeobrutalistButtonVariant.secondary,
          onPressed: () => Navigator.pop(context),
        ),
        NeobrutalistButton(
          label: 'Guardar',
          icon: Icons.check_rounded,
          variant: NeobrutalistButtonVariant.accent,
          onPressed: _submit,
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// WIDGETS DE LA PLANILLA
// ---------------------------------------------------------------------------

class _DayCell extends StatelessWidget {
  const _DayCell({required this.value, required this.onTap});

  final double value;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final hasHours = value > 0;
    return ClickCursor(
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: Container(
          height: 32,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: hasHours ? AppColors.accentYellow : AppColors.surface,
            border: Border.all(color: AppColors.border, width: 1.5),
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text(
                '${value.toStringAsFixed(1)}h',
                style: TextStyle(
                  fontWeight: FontWeight.w800,
                  fontSize: 10,
                  color: hasHours ? AppColors.text : AppColors.muted,
                ),
              ),
              const SizedBox(width: 3),
              Icon(
                Icons.edit_rounded,
                size: 10,
                color: hasHours ? AppColors.text : AppColors.muted,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _HeaderCellFixed extends StatelessWidget {
  final String text;
  final double width;
  final Color bg;
  const _HeaderCellFixed(this.text, this.width, {this.bg = AppColors.surface});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: width,
      height: 28,
      alignment: Alignment.center,
      padding: const EdgeInsets.symmetric(horizontal: 4),
      decoration: BoxDecoration(color: bg),
      child: Text(
        text,
        textAlign: TextAlign.center,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(
          fontWeight: FontWeight.w900,
          fontSize: 9,
          letterSpacing: 0.4,
          color: AppColors.text,
        ),
      ),
    );
  }
}

class _LegendBar extends StatelessWidget {
  const _LegendBar({required this.items});

  final List<_LegendChip> items;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 2),
        boxShadow: AppShadows.card,
      ),
      child: Wrap(spacing: 10, runSpacing: 6, children: items),
    );
  }
}

class _LegendChip extends StatelessWidget {
  final Color color;
  final String label;
  const _LegendChip({required this.color, required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        label,
        style: const TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );
  }
}

class _PriorityChip extends StatelessWidget {
  final String priority;
  final ValueChanged<String>? onChanged;
  const _PriorityChip({required this.priority, this.onChanged});

  Color get _bg {
    switch (priority) {
      case 'alta':
        return AppColors.errorDeep;
      case 'media':
        return AppColors.accentYellow;
      default:
        return AppColors.surfaceLow;
    }
  }

  Color get _fg => priority == 'alta' ? AppColors.surface : AppColors.text;

  @override
  Widget build(BuildContext context) {
    final chip = Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
      decoration: BoxDecoration(
        color: _bg,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        priority.toUpperCase(),
        style: TextStyle(fontSize: 9, fontWeight: FontWeight.w900, color: _fg),
        textAlign: TextAlign.center,
      ),
    );

    if (onChanged == null) return chip;

    return PopupMenuButton<String>(
      onSelected: onChanged,
      itemBuilder: (context) => [
        for (final p in const ['alta', 'media', 'baja'])
          PopupMenuItem<String>(
            value: p,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 10,
                  height: 10,
                  decoration: BoxDecoration(
                    color: switch (p) {
                      'alta' => AppColors.errorDeep,
                      'media' => AppColors.accentYellow,
                      _ => AppColors.surfaceLow,
                    },
                    border: Border.all(color: AppColors.border, width: 1),
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  p.toUpperCase(),
                  style: const TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 11,
                    color: AppColors.text,
                  ),
                ),
              ],
            ),
          ),
      ],
      child: chip,
    );
  }
}

class _StatusChip extends StatelessWidget {
  final String status;
  final Color color;
  final ValueChanged<String>? onChanged;
  const _StatusChip({
    required this.status,
    required this.color,
    this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final chip = Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        status.toUpperCase(),
        textAlign: TextAlign.center,
        style: const TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );

    if (onChanged == null) return chip;

    return PopupMenuButton<String>(
      onSelected: onChanged,
      itemBuilder: (context) => [
        for (final s in const [
          'Sin empezar',
          'En proceso',
          'Pendiente',
          'Listo',
        ])
          PopupMenuItem<String>(
            value: s,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 10,
                  height: 10,
                  decoration: BoxDecoration(
                    color: _statusChipColor(s),
                    border: Border.all(color: AppColors.border, width: 1),
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  s.toUpperCase(),
                  style: const TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 11,
                    color: AppColors.text,
                  ),
                ),
              ],
            ),
          ),
      ],
      child: chip,
    );
  }
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w900,
        color: AppColors.muted,
        letterSpacing: 0.5,
      ),
    );
  }
}

class _SegmentedPicker extends StatelessWidget {
  const _SegmentedPicker({
    required this.options,
    required this.value,
    required this.onChanged,
  });

  final List<String> options;
  final String value;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 6,
      runSpacing: 6,
      children: [
        for (final option in options)
          GestureDetector(
            onTap: () => onChanged(option),
            child: AnimatedContainer(
              duration: AppMotion.fast,
              curve: AppMotion.standard,
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
              decoration: BoxDecoration(
                color: value == option
                    ? AppColors.accentBlueDeep
                    : AppColors.bg,
                border: Border.all(color: AppColors.border, width: 1.5),
                borderRadius: BorderRadius.circular(AppDimens.radius),
                boxShadow: value == option
                    ? AppShadows.badge
                    : const <BoxShadow>[],
              ),
              child: Text(
                option.toUpperCase(),
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: value == option ? AppColors.surface : AppColors.text,
                ),
              ),
            ),
          ),
      ],
    );
  }
}

class _QuickHourChip extends StatelessWidget {
  const _QuickHourChip({required this.label, required this.onTap});

  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ClickCursor(
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          width: 44,
          padding: const EdgeInsets.symmetric(vertical: 8),
          decoration: BoxDecoration(
            color: AppColors.surfaceLow,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.badge,
          ),
          child: Text(
            label,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// TARJETA DE MÉTRICAS + BURNDOWN
// ---------------------------------------------------------------------------

class _BurndownChart extends StatelessWidget {
  const _BurndownChart({
    required this.dayLabels,
    required this.cumulativeUsed,
    required this.totalEstimate,
    required this.achievements,
    required this.totalEst,
    required this.totalUsed,
    required this.totalRemaining,
    required this.progressPercent,
  });

  final List<String> dayLabels;
  final List<double> cumulativeUsed;
  final double totalEstimate;
  final Map<String, String> achievements;
  final double totalEst;
  final double totalUsed;
  final double totalRemaining;
  final double progressPercent;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 2),
        boxShadow: AppShadows.card,
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                _MetricTile(
                  label: 'CONFIGURACIÓN',
                  value:
                      '${achievements['Sprint']} • ${achievements['Días']} días',
                  bg: AppColors.accentYellow,
                ),
                _MetricTile(
                  label: 'HORAS ESTIMADAS',
                  value: '${totalEst.toStringAsFixed(1)}h',
                  bg: AppColors.surfaceLow,
                ),
                _MetricTile(
                  label: 'HORAS REALES IMPUTADAS',
                  value: '${totalUsed.toStringAsFixed(1)}h',
                  bg: const Color(0xFFD6E3FF),
                ),
                _MetricTile(
                  label: 'HORAS RESTANTES',
                  value: '${totalRemaining.toStringAsFixed(1)}h',
                  bg: AppColors.subjectPeach,
                ),
              ],
            ),
            const SizedBox(height: 10),
            if (achievements.values.any((v) => v.isNotEmpty)) ...[
              Text(
                'OBJETIVO: ${achievements['Objetivo']}',
                style: const TextStyle(
                  fontSize: 10.5,
                  fontWeight: FontWeight.w800,
                  color: AppColors.muted,
                ),
              ),
              const SizedBox(height: 8),
            ],
            _buildProgressRow(),
            const SizedBox(height: 12),
            _buildChartSection(),
          ],
        ),
      ),
    );
  }

  Widget _buildProgressRow() {
    final pct = progressPercent.round();
    return Row(
      children: [
        const Text(
          'COMPLETADO',
          style: TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w900,
            color: AppColors.muted,
          ),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: LayoutBuilder(
            builder: (context, constraints) {
              return Stack(
                children: [
                  Container(
                    height: 16,
                    decoration: BoxDecoration(
                      color: AppColors.bg,
                      border: Border.all(color: AppColors.border, width: 2),
                      borderRadius: BorderRadius.circular(AppDimens.radius),
                    ),
                  ),
                  FractionallySizedBox(
                    alignment: Alignment.centerLeft,
                    widthFactor: progressPercent / 100,
                    child: Container(
                      height: 16,
                      decoration: BoxDecoration(
                        color: AppColors.accentYellow,
                        border: Border.all(color: AppColors.border, width: 2),
                        borderRadius: BorderRadius.circular(AppDimens.radius),
                      ),
                    ),
                  ),
                ],
              );
            },
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 42,
          child: Text(
            '$pct%',
            textAlign: TextAlign.end,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildChartSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Expanded(
              child: Text(
                'BURNDOWN • ESTIMADO VS AVANCE',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: AppColors.muted,
                ),
              ),
            ),
            const SizedBox(width: 8),
            _MiniLegend(color: AppColors.accentBlue, label: 'AVANCE'),
            const SizedBox(width: 8),
            _MiniLegend(color: AppColors.muted, label: 'IDEAL'),
          ],
        ),
        const SizedBox(height: 6),
        SizedBox(
          height: 110,
          child: CustomPaint(
            size: Size.infinite,
            painter: _BurndownPainter(
              dayLabels: dayLabels,
              cumulativeUsed: cumulativeUsed,
              totalEstimate: totalEstimate,
            ),
          ),
        ),
      ],
    );
  }
}

class _MiniLegend extends StatelessWidget {
  const _MiniLegend({required this.color, required this.label});

  final Color color;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(width: 14, height: 6, color: color),
        const SizedBox(width: 4),
        Text(
          label,
          style: const TextStyle(
            fontSize: 9,
            fontWeight: FontWeight.w900,
            color: AppColors.muted,
          ),
        ),
      ],
    );
  }
}

class _MetricTile extends StatelessWidget {
  const _MetricTile({
    required this.label,
    required this.value,
    required this.bg,
  });

  final String label;
  final String value;
  final Color bg;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 150,
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
      decoration: BoxDecoration(
        color: bg,
        border: Border.all(color: AppColors.border, width: 1.5),
        boxShadow: AppShadows.badge,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w900,
              color: AppColors.muted,
              letterSpacing: 0.4,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ],
      ),
    );
  }
}

class _BurndownPainter extends CustomPainter {
  const _BurndownPainter({
    required this.dayLabels,
    required this.cumulativeUsed,
    required this.totalEstimate,
  });

  final List<String> dayLabels;
  final List<double> cumulativeUsed;
  final double totalEstimate;

  static const double _leftPad = 32;
  static const double _rightPad = 10;
  static const double _topPad = 10;
  static const double _bottomPad = 18;

  @override
  void paint(Canvas canvas, Size size) {
    final chartW = size.width - _leftPad - _rightPad;
    final chartH = size.height - _topPad - _bottomPad;
    if (chartW <= 0 || chartH <= 0) return;

    final maxValue = math.max(totalEstimate, 1.0);
    final n = dayLabels.length;
    if (n == 0) return;

    double x(int i) => _leftPad + (n == 1 ? 0.0 : chartW * i / (n - 1));
    double y(double v) => _topPad + chartH * (1 - v / maxValue);

    final gridPaint = Paint()
      ..color = AppColors.border
      ..strokeWidth = 1;

    // Ejes.
    canvas.drawLine(
      Offset(_leftPad, _topPad),
      Offset(_leftPad, _topPad + chartH),
      gridPaint,
    );
    canvas.drawLine(
      Offset(_leftPad, _topPad + chartH),
      Offset(_leftPad + chartW, _topPad + chartH),
      gridPaint,
    );

    // Líneas de referencia horizontales (0, media, máximo).
    for (final v in [0.0, maxValue / 2, maxValue]) {
      final yy = y(v);
      canvas.drawLine(
        Offset(_leftPad, yy),
        Offset(_leftPad + chartW, yy),
        Paint()
          ..color = AppColors.muted.withValues(alpha: 0.4)
          ..strokeWidth = 1,
      );
      _paintText(
        canvas,
        '${v.round()}', // ignore: avoid_print
        Offset(1, yy - 5),
        size: 8,
        color: AppColors.muted,
      );
    }

    // Línea ideal: de totalEstimate a 0 en línea recta.
    final idealPath = Path()
      ..moveTo(x(0), y(totalEstimate))
      ..lineTo(x(n - 1), y(0));
    canvas.drawPath(
      idealPath,
      Paint()
        ..color = AppColors.muted
        ..strokeWidth = 2
        ..style = PaintingStyle.stroke,
    );

    // Línea real: resta acumulada por día.
    final realPath = Path()..moveTo(x(0), y(totalEstimate));
    for (var i = 0; i < n; i++) {
      final remaining =
          (totalEstimate - (i < cumulativeUsed.length ? cumulativeUsed[i] : 0))
              .clamp(0.0, maxValue);
      realPath.lineTo(x(i), y(remaining));
    }
    canvas.drawPath(
      realPath,
      Paint()
        ..color = AppColors.accentBlue
        ..strokeWidth = 3
        ..style = PaintingStyle.stroke
        ..strokeCap = StrokeCap.round,
    );

    // Puntos reales + etiquetas de días.
    for (var i = 0; i < n; i++) {
      final remaining =
          (totalEstimate - (i < cumulativeUsed.length ? cumulativeUsed[i] : 0))
              .clamp(0.0, maxValue);
      final px = x(i);
      final py = y(remaining);
      canvas.drawCircle(
        Offset(px, py),
        3.5,
        Paint()..color = AppColors.accentBlue,
      );
      canvas.drawCircle(
        Offset(px, py),
        3.5,
        Paint()
          ..color = AppColors.border
          ..strokeWidth = 1
          ..style = PaintingStyle.stroke,
      );
      _paintText(
        canvas,
        dayLabels[i],
        Offset(px - 12, _topPad + chartH + 4),
        size: 8,
        color: AppColors.text,
      );
    }
  }

  void _paintText(
    Canvas canvas,
    String text,
    Offset offset, {
    required double size,
    required Color color,
  }) {
    final painter = TextPainter(
      text: TextSpan(
        text: text,
        style: TextStyle(
          fontSize: size,
          fontWeight: FontWeight.w800,
          color: color,
        ),
      ),
      textDirection: TextDirection.ltr,
    )..layout();
    painter.paint(canvas, offset);
  }

  @override
  bool shouldRepaint(covariant _BurndownPainter oldDelegate) {
    return oldDelegate.totalEstimate != totalEstimate ||
        oldDelegate.cumulativeUsed.length != cumulativeUsed.length ||
        oldDelegate.dayLabels.length != dayLabels.length;
  }
}

final RegExp _hoursRegex = RegExp(r'^\d{0,3}([.,]\d{0,2})?$');

Color _statusChipColor(String status) {
  switch (status) {
    case 'En proceso':
      return AppColors.accentYellow;
    case 'Sin empezar':
      return AppColors.pendingLight;
    case 'Listo':
      return AppColors.success;
    case 'Pendiente':
      return AppColors.subjectPeach;
    default:
      return AppColors.surfaceLow;
  }
}
