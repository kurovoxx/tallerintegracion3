import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../core/common_widgets.dart';
import '../../core/services/courses_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'attendance_screen.dart';

const String kSubjectApproved = 'approved';
const String kSubjectInProgress = 'in_progress';
const String kSubjectFailed = 'failed';
const String kSubjectPending = 'pending';

String _formatGrade(num raw) {
  final v = raw >= 10 ? raw / 10 : raw.toDouble();
  return v.toStringAsFixed(1);
}

const List<String> _kWeekDays = ['Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb'];

const List<List<String>> _kScheduleBlocks = [
  ['08:15', '09:45'],
  ['10:00', '11:30'],
  ['11:45', '13:15'],
  ['14:00', '15:30'],
  ['15:45', '17:15'],
  ['17:30', '19:00'],
];

String _blockLabel(List<String> block) => '${block[0]}–${block[1]}';

enum _CourseDetailTab { unidades, materiales, evaluaciones }

class CoursesScreen extends StatefulWidget {
  const CoursesScreen({super.key});

  @override
  State<CoursesScreen> createState() => _CoursesScreenState();
}

class _CoursesScreenState extends State<CoursesScreen> {
  bool _isLoading = true;
  bool _hasError = false;

  String _searchQuery = '';
  String _statusFilter = 'Todos los estados'; // Todos | Cursando | Pendiente | Finalizado
  String? _selectedCourseId;
  _CourseDetailTab _detailTab = _CourseDetailTab.unidades;

  List<Map<String, dynamic>> get _courses => CoursesService.instance.courses;

  @override
  void initState() {
    super.initState();
    CoursesService.instance.addListener(_handleCoursesChanged);
    _loadCourses();
  }

  void _handleCoursesChanged() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    CoursesService.instance.removeListener(_handleCoursesChanged);
    super.dispose();
  }

  Future<void> _loadCourses() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
    });
    try {
      await Future.delayed(const Duration(milliseconds: 620));
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

  Map<String, dynamic>? get _selectedCourse {
    if (_selectedCourseId == null) return null;
    try {
      return _courses.firstWhere((c) => c['id'] == _selectedCourseId);
    } catch (_) {
      return null;
    }
  }

  List<Map<String, dynamic>> get _filteredCourses {
    return _courses.where((c) {
      final q = _searchQuery.trim().toLowerCase();
      final matchesSearch = q.isEmpty || (c['name'] as String).toLowerCase().contains(q) || (c['code'] as String).toLowerCase().contains(q) || (c['professor'] as String).toLowerCase().contains(q);
      if (!matchesSearch) return false;
      switch (_statusFilter) {
        case 'Cursando':
          return c['status'] == kSubjectInProgress;
        case 'Pendiente':
          return c['status'] == kSubjectPending;
        case 'Finalizado':
          return c['status'] == kSubjectApproved || c['status'] == kSubjectFailed;
        default:
          return true;
      }
    }).toList();
  }

  void _openDetail(String id) {
    setState(() {
      _selectedCourseId = id;
      _detailTab = _CourseDetailTab.unidades;
    });
  }

  void _backToList() {
    setState(() => _selectedCourseId = null);
  }

  Future<void> _showCreateCourseDialog() async {
    final result = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (_) => const _CreateCourseDialog(),
    );

    if (result == null || !mounted) return;

    CoursesService.instance.addCourse({
      'id': 'course-${DateTime.now().microsecondsSinceEpoch}',
      'code': result['code'],
      'name': result['name'],
      'credits': result['credits'],
      'status': result['status'],
      'professor': result['professor'],
      'schedule': result['schedule'],
      'room': result['room'],
      'requisite': 'REQ: —',
      'requisite_alert': false,
      'progress': result['status'] == kSubjectPending ? 0 : 15,
      'semester': 'Semestre 6',
      'icon': Icons.book_rounded,
      'units': [
        {
          'title': 'Unidad 1: Introducción',
          'badge': 'próximo',
          'items': [
            {'title': 'Contenido inicial por definir', 'checked': false, 'tag': 'Clase 1 – 2'},
          ]
        },
      ],
      'materials': [
        {'name': 'Sin materiales aún', 'meta': 'Añade PDFs desde el detalle', 'icon': Icons.description_rounded},
      ],
      'grades': [
        {'name': 'Evaluación 1 (100%)', 'weight': 1.0, 'score': null, 'status': 'pending'},
      ],
    });

    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Curso creado correctamente'),
        backgroundColor: AppColors.border,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    final canPop = Navigator.of(context).canPop();

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: _hasError
            ? _buildErrorState()
            : SingleChildScrollView(
                padding: EdgeInsets.symmetric(horizontal: isDesktop ? 48 : 16, vertical: 20),
                child: Center(
                  child: ConstrainedBox(
                    constraints: BoxConstraints(maxWidth: isDesktop ? 1320 : 640),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _buildHeader(canPop, isDesktop),
                        const SizedBox(height: 18),
                        if (_isLoading)
                          _buildLoadingCard()
                        else if (_selectedCourseId != null)
                          _buildDetailView(isDesktop)
                        else
                          _buildListView(isDesktop),
                      ],
                    ),
                  ),
                ),
              ),
      ),
    );
  }

  Widget _buildHeader(bool canPop, bool isDesktop) {
    final titleBlock = Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(_selectedCourseId != null ? 'DETALLE DEL CURSO' : 'MIS CURSOS', style: TextStyle(color: AppColors.text, fontWeight: FontWeight.w900, fontSize: isDesktop ? 28 : 22, letterSpacing: -0.6)),
      const SizedBox(height: 3),
      Text(_selectedCourseId != null ? 'Información, contenidos y evaluaciones del ramo' : 'Planifica y visualiza tu progreso académico · Semestre 6', style: const TextStyle(color: AppColors.muted, fontWeight: FontWeight.w700, fontSize: 12.5)),
    ]);

    final actions = Row(mainAxisSize: MainAxisSize.min, children: [
      if (_selectedCourseId == null)
        ElevatedButton.icon(
          onPressed: _showCreateCourseDialog,
          style: ElevatedButton.styleFrom(backgroundColor: AppColors.border, foregroundColor: Colors.white, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), elevation: 0, shadowColor: AppColors.border, padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12)),
          icon: const Icon(Icons.add_rounded, size: 18),
          label: const Text('AGREGAR CURSO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
        )
      else
        _SquareIconButton(icon: Icons.refresh_rounded, tooltip: 'Actualizar', onPressed: _loadCourses),
    ]);

    if (isDesktop) {
      return Row(crossAxisAlignment: CrossAxisAlignment.center, children: [
        if (canPop || _selectedCourseId != null) ...[
          _SquareIconButton(
              icon: Icons.arrow_back_rounded,
              tooltip: 'Volver',
              onPressed: () {
                if (_selectedCourseId != null) {
                  _backToList();
                } else if (canPop) {
                  Navigator.of(context).pop();
                }
              }),
          const SizedBox(width: 12),
        ],
        Expanded(child: titleBlock),
        const SizedBox(width: 12),
        actions,
      ]);
    }

    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        if (canPop || _selectedCourseId != null)
          _SquareIconButton(
              icon: Icons.arrow_back_rounded,
              tooltip: 'Volver',
              onPressed: () {
                if (_selectedCourseId != null) {
                  _backToList();
                } else if (canPop) {
                  Navigator.of(context).pop();
                }
              }),
        if (canPop || _selectedCourseId != null) const SizedBox(width: 10),
        Expanded(child: titleBlock),
      ]),
      const SizedBox(height: 14),
      actions,
    ]);
  }

  // ---------------------------------------------------------------------
  // LISTADO
  // ---------------------------------------------------------------------
  Widget _buildListView(bool isDesktop) {
    final filtered = _filteredCourses;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          height: 4,
          decoration: BoxDecoration(color: AppColors.border, borderRadius: BorderRadius.circular(2)),
        ),
        const SizedBox(height: 14),
        // Buscador + filtro
        Wrap(
          spacing: 12,
          runSpacing: 12,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            SizedBox(
              width: isDesktop ? 420 : double.infinity,
              child: TextField(
                onChanged: (v) => setState(() => _searchQuery = v),
                style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                decoration: appInputDecoration('Buscar curso en el semestre...').copyWith(
                  prefixIcon: const Icon(Icons.search_rounded, color: AppColors.muted, size: 20),
                  filled: true,
                  fillColor: AppColors.surface,
                ),
              ),
            ),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
              child: DropdownButtonHideUnderline(
                child: DropdownButton<String>(
                  value: _statusFilter,
                  icon: const Icon(Icons.expand_more_rounded, size: 20, color: AppColors.text),
                  style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text),
                  items: const [
                    DropdownMenuItem(value: 'Todos los estados', child: Text('Todos los estados')),
                    DropdownMenuItem(value: 'Cursando', child: Text('Cursando')),
                    DropdownMenuItem(value: 'Pendiente', child: Text('Pendiente')),
                    DropdownMenuItem(value: 'Finalizado', child: Text('Finalizado')),
                  ],
                  onChanged: (v) {
                    if (v != null) setState(() => _statusFilter = v);
                  },
                ),
              ),
            ),
            if (_searchQuery.isNotEmpty || _statusFilter != 'Todos los estados')
              ActionChip(
                label: const Text('LIMPIAR FILTROS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text)),
                backgroundColor: AppColors.accentYellow,
                side: const BorderSide(color: AppColors.border, width: 2),
                onPressed: () => setState(() {
                  _searchQuery = '';
                  _statusFilter = 'Todos los estados';
                }),
              ),
          ],
        ),
        const SizedBox(height: 18),
        if (filtered.isEmpty)
          Container(
            padding: const EdgeInsets.all(28),
            decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
            child: Column(children: [
              const Icon(Icons.search_off_rounded, size: 36, color: AppColors.muted),
              const SizedBox(height: 10),
              const Text('SIN RESULTADOS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
              const SizedBox(height: 6),
              Text('No hay cursos para "$_statusFilter"${_searchQuery.isNotEmpty ? ' con "$_searchQuery"' : ''}', textAlign: TextAlign.center, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12, color: AppColors.muted)),
            ]),
          )
        else
          LayoutBuilder(builder: (context, c) {
            final cols = isDesktop ? (c.maxWidth > 900 ? 2 : 2) : 1;
            final gap = 14.0 * (cols - 1);
            final w = cols == 1 ? c.maxWidth : (c.maxWidth - gap) / cols;
            return Wrap(
              spacing: 14,
              runSpacing: 14,
              children: filtered.map((course) => SizedBox(width: w, child: _CourseCard(course: course, onTap: () => _openDetail(course['id'] as String)))).toList(),
            );
          }),
      ],
    );
  }

  // ---------------------------------------------------------------------
  // DETALLE
  // ---------------------------------------------------------------------
  Widget _buildDetailView(bool isDesktop) {
    final course = _selectedCourse!;
    final progress = course['progress'] as int;
    final status = course['status'] as String;
    final statusLabel = _statusLabel(status);
    final statusColor = _statusColor(status);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        InkWell(
          onTap: _backToList,
          borderRadius: BorderRadius.circular(AppDimens.radius),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
            child: Row(mainAxisSize: MainAxisSize.min, children: const [Icon(Icons.arrow_back_rounded, size: 16, color: AppColors.text), SizedBox(width: 6), Text('VOLVER A MIS CURSOS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text))]),
          ),
        ),
        const SizedBox(height: 16),
        // Header detalle (replica pmn view-curso-detalle)
        Container(
          padding: EdgeInsets.all(isDesktop ? 22 : 16),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 4), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(6, 6), blurRadius: 0)]),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Expanded(
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text('${course['code']} · ${course['credits']} CRÉDITOS', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.accentBlue, letterSpacing: 0.4)),
                    const SizedBox(height: 4),
                    Text((course['name'] as String).toUpperCase(), style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 20, color: AppColors.text, letterSpacing: -0.4)),
                  ]),
                ),
                const SizedBox(width: 12),
                Container(padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6), decoration: BoxDecoration(color: statusColor, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(4)), child: Text(statusLabel, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: status == kSubjectFailed ? Colors.white : AppColors.text))),
              ]),
              const SizedBox(height: 16),
              Container(
                padding: const EdgeInsets.all(14),
                decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                child: Wrap(
                  spacing: 16,
                  runSpacing: 12,
                  children: [
                    _DetailMeta(label: 'PROFESOR / DOCENTE', value: course['professor'] as String),
                    _DetailMeta(label: 'HORARIOS DE CLASE', value: course['schedule'] as String),
                    _DetailMeta(label: 'SALA / AULA', value: course['room'] as String),
                    _DetailMeta(label: 'PROGRESO DEL CURSO', value: '$progress% completado'),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [const Text('AVANCE GENERAL DEL PROGRAMA', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.muted)), Text('$progress%', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text))]),
              const SizedBox(height: 6),
              Container(height: 12, decoration: BoxDecoration(border: Border.all(color: AppColors.border, width: 2)), child: FractionallySizedBox(alignment: Alignment.centerLeft, widthFactor: progress / 100, child: Container(color: AppColors.accentYellow))),
            ],
          ),
        ),
        const SizedBox(height: 16),
        // Tabs
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            _CourseTabButton(icon: Icons.menu_book_rounded, label: 'Unidades y Contenidos', active: _detailTab == _CourseDetailTab.unidades, onTap: () => setState(() => _detailTab = _CourseDetailTab.unidades)),
            _CourseTabButton(icon: Icons.description_rounded, label: 'Materiales y Apuntes', active: _detailTab == _CourseDetailTab.materiales, onTap: () => setState(() => _detailTab = _CourseDetailTab.materiales)),
            _CourseTabButton(icon: Icons.bar_chart_rounded, label: 'Evaluaciones y Notas', active: _detailTab == _CourseDetailTab.evaluaciones, onTap: () => setState(() => _detailTab = _CourseDetailTab.evaluaciones)),
            _CourseTabButton(icon: Icons.article_rounded, label: 'Apuntes', active: false, onTap: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Módulo de apuntes próximamente'), backgroundColor: AppColors.border))),
            _CourseTabButton(
              icon: Icons.fact_check_rounded,
              label: 'Ver Asistencia',
              active: false,
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => AttendanceScreen(initialSubjectId: course['id'] as String),
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        if (_detailTab == _CourseDetailTab.unidades) _buildUnidadesTab(course),
        if (_detailTab == _CourseDetailTab.materiales) _buildMaterialesTab(course),
        if (_detailTab == _CourseDetailTab.evaluaciones) _buildEvaluacionesTab(course),
      ],
    );
  }

  Widget _buildUnidadesTab(Map<String, dynamic> course) {
    final units = course['units'] as List;
    return Column(
      children: units.map<Widget>((u) {
        final badge = u['badge'] as String;
        final items = u['items'] as List;
        Color badgeColor;
        Color badgeText;
        switch (badge) {
          case 'completado':
            badgeColor = const Color(0xFFDCFCE7);
            badgeText = const Color(0xFF166534);
            break;
          case 'en curso':
            badgeColor = const Color(0xFFD6E3FF);
            badgeText = AppColors.accentBlue;
            break;
          case 'próximo':
          default:
            badgeColor = AppColors.accentBlue;
            badgeText = Colors.white;
        }
        return Container(
          margin: const EdgeInsets.only(bottom: 12),
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)]),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(children: [
                Expanded(child: Text((u['title'] as String).toUpperCase(), style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12.5, color: AppColors.text))),
                const SizedBox(width: 10),
                Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4), decoration: BoxDecoration(color: badgeColor, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text(badge, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: badgeText))),
              ]),
              const Divider(color: AppColors.border, thickness: 2, height: 18),
              ...items.asMap().entries.map((entry) {
                final idx = entry.key;
                final it = entry.value as Map;
                final checked = it['checked'] as bool;
                return Container(
                  padding: const EdgeInsets.symmetric(vertical: 10),
                  decoration: BoxDecoration(border: idx != items.length - 1 ? const Border(bottom: BorderSide(color: AppColors.border, width: 1, style: BorderStyle.solid)) : null),
                  child: Row(
                    children: [
                      Checkbox(
                        value: checked,
                        activeColor: AppColors.accentYellow,
                        checkColor: AppColors.text,
                        side: const BorderSide(color: AppColors.border, width: 2),
                        onChanged: (v) {
                          setState(() => it['checked'] = v ?? false);
                        },
                      ),
                      const SizedBox(width: 6),
                      Expanded(child: Text(it['title'] as String, style: TextStyle(fontWeight: FontWeight.w800, fontSize: 12.5, color: checked ? AppColors.text : AppColors.muted))),
                      const SizedBox(width: 8),
                      Container(padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3), decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text(it['tag'] as String, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 9, color: AppColors.text))),
                    ],
                  ),
                );
              }),
            ],
          ),
        );
      }).toList(),
    );
  }

  Widget _buildMaterialesTab(Map<String, dynamic> course) {
    final materials = course['materials'] as List;
    return LayoutBuilder(builder: (context, c) {
      final cols = c.maxWidth > 600 ? 3 : 1;
      final gap = 12.0 * (cols - 1);
      final w = cols == 1 ? c.maxWidth : (c.maxWidth - gap) / cols;
      return Wrap(
        spacing: 12,
        runSpacing: 12,
        children: materials.map<Widget>((m) {
          return SizedBox(
            width: w,
            child: Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
              child: Row(children: [
                Container(width: 40, height: 40, alignment: Alignment.center, decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)), child: Icon(m['icon'] as IconData, size: 20, color: AppColors.text)),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(m['name'] as String, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 12.5, color: AppColors.text)),
                    const SizedBox(height: 2),
                    Text(m['meta'] as String, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
                  ]),
                ),
              ]),
            ),
          );
        }).toList(),
      );
    });
  }

  Widget _buildEvaluacionesTab(Map<String, dynamic> course) {
    final grades = course['grades'] as List;
    return Column(
      children: grades.map<Widget>((g) {
        final status = g['status'] as String;
        final score = g['score'] as int?;
        final weight = g['weight'] as double;
        final isGraded = status == 'graded';
        final isPending = status == 'pending';
        final isExcused = status == 'excused';

        String badgeLabel;
        Color badgeColor;
        Color badgeText;
        if (isGraded) {
          badgeLabel = 'evaluado';
          badgeColor = const Color(0xFFDCFCE7);
          badgeText = const Color(0xFF166534);
        } else if (isPending) {
          if (g['date'] != null) {
            badgeLabel = 'próximo · ${g['date']}';
            badgeColor = AppColors.accentBlue;
            badgeText = Colors.white;
          } else {
            badgeLabel = 'pendiente';
            badgeColor = AppColors.accentYellow;
            badgeText = AppColors.text;
          }
        } else if (isExcused) {
          badgeLabel = 'eximido';
          badgeColor = AppColors.accentBlue;
          badgeText = Colors.white;
        } else {
          badgeLabel = 'sin registro';
          badgeColor = AppColors.muted;
          badgeText = Colors.white;
        }

        return Container(
          margin: const EdgeInsets.only(bottom: 10),
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
          child: Row(children: [
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3), decoration: BoxDecoration(color: badgeColor, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text(badgeLabel, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: badgeText))),
                const SizedBox(height: 8),
                Text(g['name'] as String, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text)),
                const SizedBox(height: 2),
                Text('Ponderación ${(weight * 100).toStringAsFixed(0)}%', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
              ]),
            ),
            const SizedBox(width: 12),
            Text(score != null ? _formatGrade(score) : '–', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 24, color: score != null ? AppColors.text : AppColors.muted)),
          ]),
        );
      }).toList(),
    );
  }

  String _statusLabel(String s) {
    switch (s) {
      case kSubjectInProgress:
        return 'CURSANDO';
      case kSubjectPending:
        return 'PENDIENTE';
      case kSubjectApproved:
        return 'FINALIZADO';
      case kSubjectFailed:
        return 'ALERTA REQUISITO';
      default:
        return s.toUpperCase();
    }
  }

  Color _statusColor(String s) {
    switch (s) {
      case kSubjectInProgress:
        return const Color(0xFFDCFCE7);
      case kSubjectPending:
        return AppColors.accentYellow;
      case kSubjectApproved:
        return const Color(0xFFDCFCE7);
      case kSubjectFailed:
        return AppColors.error;
      default:
        return AppColors.bg;
    }
  }

  Widget _buildLoadingCard() {
    return Container(height: 260, decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]), child: const Center(child: CircularProgressIndicator(color: AppColors.text)));
  }

  Widget _buildErrorState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Icon(Icons.error_outline_rounded, size: 48, color: AppColors.text),
          const SizedBox(height: 12),
          const Text('NO SE PUDIERON CARGAR TUS CURSOS', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
          const SizedBox(height: 8),
          const Text('Intenta nuevamente en unos segundos.', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.muted)),
          const SizedBox(height: 18),
          SizedBox(width: 200, child: SubmitButton(text: 'REINTENTAR', onPressed: _loadCourses)),
        ]),
      ),
    );
  }
}

// ---------------------------------------------------------------------
// Widgets locales
// ---------------------------------------------------------------------
class _CourseCard extends StatelessWidget {
  final Map<String, dynamic> course;
  final VoidCallback onTap;
  const _CourseCard({required this.course, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final status = course['status'] as String;
    final isAlert = course['requisite_alert'] == true;
    final progress = course['progress'] as int;

    Color cardBg = AppColors.surface;
    if (isAlert) cardBg = const Color(0xFFFFDAD6);

    String badgeText;
    Color badgeBg;
    Color badgeTextColor;
    switch (status) {
      case kSubjectInProgress:
        badgeText = 'CURSANDO';
        badgeBg = const Color(0xFFDCFCE7);
        badgeTextColor = const Color(0xFF166534);
        break;
      case kSubjectPending:
        badgeText = 'PENDIENTE';
        badgeBg = AppColors.accentYellow;
        badgeTextColor = AppColors.text;
        break;
      case kSubjectApproved:
        badgeText = 'AL DÍA';
        badgeBg = const Color(0xFFDCFCE7);
        badgeTextColor = const Color(0xFF166534);
        break;
      case kSubjectFailed:
        badgeText = 'ALERTA REQUISITO';
        badgeBg = Colors.white;
        badgeTextColor = AppColors.error;
        break;
      default:
        badgeText = status.toUpperCase();
        badgeBg = AppColors.bg;
        badgeTextColor = AppColors.text;
    }

    return InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(color: cardBg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text((course['name'] as String).toUpperCase(), style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: isAlert ? AppColors.error : AppColors.text, height: 1.15)),
                  const SizedBox(height: 3),
                  Text('${course['code']} · ${course['credits']} créditos', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: isAlert ? AppColors.error.withValues(alpha: 0.85) : AppColors.muted)),
                ]),
              ),
              const SizedBox(width: 8),
              Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4), decoration: BoxDecoration(color: badgeBg, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text(badgeText, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 9, color: badgeTextColor))),
            ]),
            const SizedBox(height: 12),
            Wrap(spacing: 12, runSpacing: 6, children: [
              _InfoLine(label: 'Profesor:', value: course['professor'] as String),
              _InfoLine(label: 'Horario:', value: course['schedule'] as String),
              _InfoLine(label: 'Sala:', value: course['room'] as String),
              Row(mainAxisSize: MainAxisSize.min, children: [
                const Text('Requisito:', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text)),
                const SizedBox(width: 6),
                Container(padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3), decoration: BoxDecoration(color: AppColors.accentBlue, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text(course['requisite'] as String, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 9, color: Colors.white))),
              ]),
            ]),
            const SizedBox(height: 14),
            Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [Text('AVANCE', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: isAlert ? AppColors.error : AppColors.muted)), Text('$progress%', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: isAlert ? AppColors.error : AppColors.text))]),
            const SizedBox(height: 6),
            Container(height: 10, decoration: BoxDecoration(border: Border.all(color: AppColors.border, width: 2), color: isAlert ? Colors.white : null), child: FractionallySizedBox(alignment: Alignment.centerLeft, widthFactor: progress / 100, child: Container(color: isAlert ? AppColors.error : AppColors.accentYellow))),
          ],
        ),
      ),
    );
  }
}

class _InfoLine extends StatelessWidget {
  final String label;
  final String value;
  const _InfoLine({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return RichText(
      text: TextSpan(children: [
        TextSpan(text: '$label ', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text)),
        TextSpan(text: value, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
      ]),
    );
  }
}

class _DetailMeta extends StatelessWidget {
  final String label;
  final String value;
  const _DetailMeta({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 160,
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(label, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.muted)),
        const SizedBox(height: 2),
        Text(value, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 12.5, color: AppColors.text)),
      ]),
    );
  }
}

class _CourseTabButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool active;
  final VoidCallback onTap;
  const _CourseTabButton({required this.icon, required this.label, required this.active, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(color: active ? AppColors.border : AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: active ? const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)] : null),
        child: Row(mainAxisSize: MainAxisSize.min, children: [Icon(icon, size: 16, color: active ? Colors.white : AppColors.text), const SizedBox(width: 6), Text(label.toUpperCase(), style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: active ? Colors.white : AppColors.text))]),
      ),
    );
  }
}

/// Diálogo "CREAR NUEVO CURSO" como StatefulWidget dedicado. Los
/// TextEditingController viven en su State (initState) y se liberan en
/// dispose(), momento en que la ruta del diálogo ya desmontó sus widgets.
/// Esto evita el assert de ChangeNotifier '_dependents.isEmpty' que ocurría
/// al disponer los controladores justo después del pop del showDialog.
class _CreateCourseDialog extends StatefulWidget {
  const _CreateCourseDialog();

  @override
  State<_CreateCourseDialog> createState() => _CreateCourseDialogState();
}

class _CreateCourseDialogState extends State<_CreateCourseDialog> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _nameCtrl;
  late final TextEditingController _codeCtrl;
  late final TextEditingController _creditsCtrl;
  late final TextEditingController _professorCtrl;
  late final TextEditingController _scheduleCtrl;
  late final TextEditingController _roomCtrl;
  String _status = kSubjectInProgress;
  // Bloques de horario añadidos, p.ej. 'Lun / Mié · 10:00–11:30'.
  final List<String> _scheduleEntries = [];

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController();
    _codeCtrl = TextEditingController();
    _creditsCtrl = TextEditingController(text: '5');
    _professorCtrl = TextEditingController();
    _scheduleCtrl = TextEditingController();
    _roomCtrl = TextEditingController();
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _codeCtrl.dispose();
    _creditsCtrl.dispose();
    _professorCtrl.dispose();
    _scheduleCtrl.dispose();
    _roomCtrl.dispose();
    super.dispose();
  }

  void _submit() {
    if (!_formKey.currentState!.validate()) return;
    Navigator.of(context).pop({
      'name': _nameCtrl.text.trim(),
      'code': _codeCtrl.text.trim().toUpperCase(),
      'credits': int.tryParse(_creditsCtrl.text) ?? 5,
      'status': _status,
      'professor': _professorCtrl.text.trim().isEmpty ? 'Por asignar' : _professorCtrl.text.trim(),
      'schedule': _scheduleCtrl.text.trim().isEmpty ? 'Por definir' : _scheduleCtrl.text.trim(),
      'room': _roomCtrl.text.trim().isEmpty ? 'Por asignar' : _roomCtrl.text.trim(),
    });
  }

  /// Sincroniza el campo HORARIO con los bloques añadidos.
  void _syncScheduleText() {
    _scheduleCtrl.text = _scheduleEntries.join(' · ');
  }

  void _removeScheduleEntry(int index) {
    setState(() {
      _scheduleEntries.removeAt(index);
      _syncScheduleText();
    });
  }

  /// Abre el popup compacto de horario (días + bloque) que devuelve la
  /// lista final de entradas añadidas.
  Future<void> _openSchedulePicker() async {
    final result = await showDialog<List<String>>(
      context: context,
      builder: (_) => _SchedulePickerDialog(
        initialEntries: List<String>.of(_scheduleEntries),
      ),
    );
    if (result == null) return;
    setState(() {
      _scheduleEntries
        ..clear()
        ..addAll(result);
      _syncScheduleText();
    });
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      type: MaterialType.transparency,
      child: AlertDialog(
        backgroundColor: AppColors.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppDimens.radius),
          side: const BorderSide(color: AppColors.border, width: 3),
        ),
        title: const Text(
          'CREAR NUEVO CURSO',
          style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text),
        ),
        content: SingleChildScrollView(
          child: Form(
            key: _formKey,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const AppFieldLabel('NOMBRE DEL RAMO'),
                const SizedBox(height: 6),
                TextFormField(
                  controller: _nameCtrl,
                  style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14, color: AppColors.text),
                  decoration: appInputDecoration('Sistemas Inteligentes'),
                  validator: (v) {
                    if (v == null || v.trim().isEmpty) return 'Requerido';
                    if (v.trim().length < 3) return 'Mínimo 3 caracteres';
                    return null;
                  },
                ),
                const SizedBox(height: 12),
                Row(children: [
                  Expanded(
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const AppFieldLabel('CÓDIGO'),
                      const SizedBox(height: 6),
                      TextFormField(
                        controller: _codeCtrl,
                        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                        decoration: appInputDecoration('INFO-301'),
                        inputFormatters: [LengthLimitingTextInputFormatter(20)],
                        validator: (v) {
                          if (v == null || v.trim().isEmpty) return 'Requerido';
                          if (!RegExp(r'^[A-Z]{2,5}[- ]?\d{3,4}$').hasMatch(v.trim().toUpperCase())) return 'Ej. INF-301';
                          return null;
                        },
                      ),
                    ]),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const AppFieldLabel('CRÉDITOS'),
                      const SizedBox(height: 6),
                      TextFormField(
                        controller: _creditsCtrl,
                        keyboardType: TextInputType.number,
                        inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(2)],
                        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                        decoration: appInputDecoration('5'),
                        validator: (v) {
                          final n = int.tryParse(v ?? '');
                          if (n == null) return 'Requerido';
                          if (n < 1 || n > 12) return '1-12';
                          return null;
                        },
                      ),
                    ]),
                  ),
                ]),
                const SizedBox(height: 12),
                const AppFieldLabel('PROFESOR'),
                const SizedBox(height: 6),
                TextFormField(
                  controller: _professorCtrl,
                  style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                  decoration: appInputDecoration('Prof. Apellido'),
                ),
                const SizedBox(height: 12),
                // Selector compacto: un botón abre el popup de horario
                // (días + bloque), agregando entradas tipo 'Lun / Mié · 10:00–11:30'
                // que se sincronizan con el campo HORARIO (sigue siendo editable).
                Row(children: [
                  const Text('HORARIO SEMANAL', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text, letterSpacing: 0.5)),
                  const Spacer(),
                  if (_scheduleEntries.isNotEmpty)
                    InkWell(
                      onTap: () {
                        setState(() {
                          _scheduleEntries.clear();
                          _syncScheduleText();
                        });
                      },
                      child: const Text('LIMPIAR', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.error)),
                    ),
                ]),
                const SizedBox(height: 6),
                NeobrutalistButton(
                  label: 'SELECCIONAR HORARIO',
                  icon: Icons.calendar_month_rounded,
                  variant: NeobrutalistButtonVariant.secondary,
                  expand: true,
                  onPressed: _openSchedulePicker,
                ),
                const SizedBox(height: 8),
                if (_scheduleEntries.isEmpty)
                  const Text('Elige días y franjas en el popup para armar el horario.', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 10, color: AppColors.muted))
                else
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      for (var i = 0; i < _scheduleEntries.length; i++)
                        _ScheduleEntryChip(
                          label: _scheduleEntries[i],
                          onDelete: () => _removeScheduleEntry(i),
                        ),
                    ],
                  ),
                const SizedBox(height: 10),
                Row(children: [
                  Expanded(
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const AppFieldLabel('HORARIO'),
                      const SizedBox(height: 6),
                      TextFormField(
                        controller: _scheduleCtrl,
                        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                        decoration: appInputDecoration('Lun / Mié · 10:00'),
                      ),
                    ]),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const AppFieldLabel('SALA'),
                      const SizedBox(height: 6),
                      TextFormField(
                        controller: _roomCtrl,
                        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                        decoration: appInputDecoration('CJP11-204'),
                      ),
                    ]),
                  ),
                ]),
                const SizedBox(height: 12),
                const AppFieldLabel('ESTADO'),
                const SizedBox(height: 6),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 10),
                  decoration: BoxDecoration(
                    color: AppColors.bg,
                    border: Border.all(color: AppColors.border, width: 2),
                    borderRadius: BorderRadius.circular(AppDimens.radius),
                  ),
                  child: DropdownButtonHideUnderline(
                    child: DropdownButton<String>(
                      value: _status,
                      isExpanded: true,
                      style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text),
                      items: const [
                        DropdownMenuItem(value: kSubjectInProgress, child: Text('Cursando')),
                        DropdownMenuItem(value: kSubjectPending, child: Text('Pendiente')),
                        DropdownMenuItem(value: kSubjectApproved, child: Text('Finalizado · Aprobado')),
                        DropdownMenuItem(value: kSubjectFailed, child: Text('Finalizado · Reprobado')),
                      ],
                      onChanged: (v) {
                        if (v != null) setState(() => _status = v);
                      },
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('CANCELAR', style: TextStyle(fontWeight: FontWeight.w800, color: AppColors.muted)),
          ),
          ElevatedButton(
            onPressed: _submit,
            style: ElevatedButton.styleFrom(
              backgroundColor: AppColors.accentYellow,
              foregroundColor: AppColors.text,
              side: const BorderSide(color: AppColors.border, width: 2),
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)),
            ),
            child: const Text('CREAR RAMO', style: TextStyle(fontWeight: FontWeight.w900)),
          ),
        ],
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
    final btn = InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onPressed,
      child: Container(width: 42, height: 42, alignment: Alignment.center, decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]), child: Icon(icon, size: 18, color: AppColors.text)),
    );
    return tooltip != null ? Tooltip(message: tooltip!, child: btn) : btn;
  }
}

/// Popup compacto para armar el horario al crear un curso (estilo Agendar
/// Reunión): chips de días (multi-selección), chips de bloque/hora común,
/// botón 'AÑADIR BLOQUE' y la lista de bloques ya añadidos. Devuelve la
/// lista final de entradas cuando se confirma con 'LISTO'.
class _SchedulePickerDialog extends StatefulWidget {
  final List<String> initialEntries;
  const _SchedulePickerDialog({required this.initialEntries});

  @override
  State<_SchedulePickerDialog> createState() => _SchedulePickerDialogState();
}

class _SchedulePickerDialogState extends State<_SchedulePickerDialog> {
  late final List<String> _added;
  final Set<String> _selectedDays = <String>{};
  String? _selectedBlock;

  @override
  void initState() {
    super.initState();
    _added = List<String>.of(widget.initialEntries);
  }

  /// Vista previa de la entrada en construcción, p.ej. 'Lun / Mié · 10:00–11:30'.
  String get _preview {
    final days = _selectedDays.toList()
      ..sort((a, b) => _kWeekDays.indexOf(a).compareTo(_kWeekDays.indexOf(b)));
    if (days.isEmpty || _selectedBlock == null) return '';
    return '${days.join(' / ')} · $_selectedBlock';
  }

  void _addBlock() {
    final preview = _preview;
    if (preview.isEmpty) return;
    setState(() {
      _added.add(preview);
      _selectedDays.clear();
      _selectedBlock = null;
    });
  }

  void _removeAt(int index) {
    setState(() => _added.removeAt(index));
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      type: MaterialType.transparency,
      child: AlertDialog(
        backgroundColor: AppColors.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppDimens.radius),
          side: const BorderSide(color: AppColors.border, width: 3),
        ),
        title: const Text(
          'SELECCIONAR HORARIO',
          style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text),
        ),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text('DÍAS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.muted, letterSpacing: 0.5)),
              const SizedBox(height: 6),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  for (final day in _kWeekDays)
                    _ScheduleDayChip(
                      label: day,
                      selected: _selectedDays.contains(day),
                      onTap: () {
                        setState(() {
                          if (_selectedDays.contains(day)) {
                            _selectedDays.remove(day);
                          } else {
                            _selectedDays.add(day);
                          }
                        });
                      },
                    ),
                ],
              ),
              const SizedBox(height: 14),
              const Text('BLOQUE / HORA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.muted, letterSpacing: 0.5)),
              const SizedBox(height: 6),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  for (final block in _kScheduleBlocks)
                    _ScheduleDayChip(
                      label: _blockLabel(block),
                      selected: _selectedBlock == _blockLabel(block),
                      onTap: () => setState(() => _selectedBlock = _blockLabel(block)),
                    ),
                ],
              ),
              const SizedBox(height: 14),
              if (_preview.isNotEmpty) ...[
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                  decoration: BoxDecoration(
                    color: AppColors.accentYellow,
                    border: Border.all(color: AppColors.border, width: 2),
                    borderRadius: BorderRadius.circular(AppDimens.radius),
                  ),
                  child: Text(_preview, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text)),
                ),
                const SizedBox(height: 8),
              ],
              NeobrutalistButton(
                label: 'AÑADIR BLOQUE',
                icon: Icons.add_rounded,
                variant: NeobrutalistButtonVariant.accent,
                expand: true,
                onPressed: _preview.isEmpty ? null : _addBlock,
              ),
              if (_added.isNotEmpty) ...[
                const SizedBox(height: 14),
                const Text('BLOQUES AÑADIDOS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.muted, letterSpacing: 0.5)),
                const SizedBox(height: 6),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    for (var i = 0; i < _added.length; i++)
                      _ScheduleEntryChip(label: _added[i], onDelete: () => _removeAt(i)),
                  ],
                ),
              ],
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('CANCELAR', style: TextStyle(fontWeight: FontWeight.w800, color: AppColors.muted)),
          ),
          NeobrutalistButton(
            label: 'LISTO',
            icon: Icons.check_rounded,
            variant: NeobrutalistButtonVariant.primary,
            onPressed: () => Navigator.of(context).pop(List<String>.of(_added)),
          ),
        ],
      ),
    );
  }
}

/// Chip con la entrada de horario añadida (p.ej. 'Lun / Mié · 10:00–11:30')
/// y una 'X' para eliminarla.
class _ScheduleEntryChip extends StatelessWidget {
  final String label;
  final VoidCallback onDelete;
  const _ScheduleEntryChip({required this.label, required this.onDelete});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.only(left: 10, right: 4, top: 6, bottom: 6),
      decoration: BoxDecoration(
        color: AppColors.accentYellow,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(label, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.text)),
          const SizedBox(width: 4),
          InkWell(
            onTap: onDelete,
            borderRadius: BorderRadius.circular(4),
            child: const Padding(
              padding: EdgeInsets.all(2),
              child: Icon(Icons.close_rounded, size: 14, color: AppColors.text),
            ),
          ),
        ],
      ),
    );
  }
}

/// Chip sticker de día en el selector semanal: resaltador invertido al
/// seleccionarse, hundimiento mecánico al presionar.
class _ScheduleDayChip extends StatelessWidget {
  final String label;
  final bool selected;
  final VoidCallback onTap;
  const _ScheduleDayChip({required this.label, required this.selected, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: AnimatedContainer(
          duration: AppMotion.press,
          curve: AppMotion.standard,
          transform: Matrix4.translationValues(
            selected ? AppShadows.offsetBadge.dx : 0,
            selected ? AppShadows.offsetBadge.dy : 0,
            0,
          ),
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
          decoration: BoxDecoration(
            color: selected ? AppColors.accentYellow : AppColors.surface,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: selected ? null : AppShadows.badge,
          ),
          child: Text(
            label,
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 10,
              color: selected ? AppColors.text : AppColors.muted,
            ),
          ),
        ),
      ),
    );
  }
}
