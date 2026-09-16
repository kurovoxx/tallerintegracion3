import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import 'attendance_screen.dart';

const String kSubjectApproved = 'approved';
const String kSubjectInProgress = 'in_progress';
const String kSubjectFailed = 'failed';
const String kSubjectPending = 'pending';

String _formatGrade(num raw) {
  final v = raw >= 10 ? raw / 10 : raw.toDouble();
  return v.toStringAsFixed(1);
}

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

  List<Map<String, dynamic>> _courses = [];

  @override
  void initState() {
    super.initState();
    _loadCourses();
  }

  Future<void> _loadCourses() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
    });
    try {
      await Future.delayed(const Duration(milliseconds: 620));
      _courses = [
        {
          'id': 'calc-3',
          'code': 'INF-1111',
          'name': 'Cálculo III',
          'credits': 5,
          'status': kSubjectInProgress,
          'professor': 'Prof. 1',
          'schedule': 'Lun / Mié · 10:00–11:30',
          'room': 'CJP11-204',
          'requisite': 'REQ: MA1101',
          'requisite_alert': false,
          'progress': 68,
          'semester': 'Semestre 6',
          'icon': Icons.menu_book_rounded,
          'units': [
            {
              'title': 'Unidad 1: Funciones de Varias Variables y Derivadas Parciales',
              'badge': 'completado',
              'items': [
                {'title': 'Dominio, Rango y Graficación de Funciones de Varias Variables', 'checked': true, 'tag': 'Clase 1 – 4'},
                {'title': 'Límites y Continuidad en Rⁿ', 'checked': true, 'tag': 'Clase 5 – 7'},
              ]
            },
            {
              'title': 'Unidad 2: Integrales Múltiples y Cambio de Variables',
              'badge': 'en curso',
              'items': [
                {'title': 'Integrales Dobles sobre Regiones Generales', 'checked': true, 'tag': 'Clase 11 – 13'},
                {'title': 'Transformación a Coordenadas Polares, Cilíndricas y Esféricas', 'checked': false, 'tag': 'Clase 14 – 17'},
              ]
            },
            {
              'title': 'Unidad 3: Cálculo Vectorial y Teoremas Fundamentales',
              'badge': 'próximo',
              'items': [
                {'title': 'Campos Vectoriales, Divergencia y Rotacional', 'checked': false, 'tag': 'Clase 21 – 24'},
              ]
            },
          ],
          'materials': [
            {'name': 'Guía 1 - Derivadas Parciales.pdf', 'meta': 'PDF · 2.4 MB', 'icon': Icons.picture_as_pdf_rounded},
            {'name': 'Diapositivas - Integrales Dobles.pdf', 'meta': 'PDF · 5.1 MB', 'icon': Icons.picture_as_pdf_rounded},
            {'name': 'Solucionario Certamen 1 2025.pdf', 'meta': 'PDF · 1.8 MB', 'icon': Icons.picture_as_pdf_rounded},
          ],
          'grades': [
            {'name': 'Certamen 1 (25%) · Unidad 1', 'weight': 0.25, 'score': 62, 'status': 'graded'},
            {'name': 'Certamen 2 (35%) · Unidad 2', 'weight': 0.35, 'score': null, 'status': 'pending', 'date': '15 nov'},
            {'name': 'Tareas Prácticas y Talleres (40%)', 'weight': 0.40, 'score': 68, 'status': 'graded'},
          ],
        },
        {
          'id': 'taller-3',
          'code': 'INF-360',
          'name': 'Taller de Integración III',
          'credits': 4,
          'status': kSubjectPending,
          'professor': 'Prof. 2',
          'schedule': 'Mar / Jue · 08:30–10:00',
          'room': 'CJP11-102',
          'requisite': 'REQ: INF-200',
          'requisite_alert': false,
          'progress': 42,
          'semester': 'Semestre 6',
          'icon': Icons.computer_rounded,
          'units': [
            {
              'title': 'Unidad 1: Levantamiento de Requerimientos',
              'badge': 'en curso',
              'items': [
                {'title': 'Entrevistas y especificación funcional', 'checked': true, 'tag': 'Clase 1 – 3'},
                {'title': 'Historias de usuario y criterios de aceptación', 'checked': false, 'tag': 'Clase 4 – 6'},
              ]
            },
            {
              'title': 'Unidad 2: Diseño y Prototipado',
              'badge': 'próximo',
              'items': [
                {'title': 'Wireframes y PMN en Tailwind', 'checked': false, 'tag': 'Clase 7 – 10'},
              ]
            },
          ],
          'materials': [
            {'name': 'PMN - Especificación Sigma Academy.pdf', 'meta': 'PDF · 3.2 MB', 'icon': Icons.description_rounded},
            {'name': 'Guía Flutter Neo-Brutalismo.pdf', 'meta': 'PDF · 1.1 MB', 'icon': Icons.description_rounded},
          ],
          'grades': [
            {'name': 'Avance 1 - PMN (20%)', 'weight': 0.20, 'score': 58, 'status': 'graded'},
            {'name': 'Sprint 1 (30%)', 'weight': 0.30, 'score': null, 'status': 'pending'},
            {'name': 'Entrega Final (50%)', 'weight': 0.50, 'score': null, 'status': 'pending'},
          ],
        },
        {
          'id': 'seg-inf',
          'code': 'INF-350',
          'name': 'Seguridad Informática',
          'credits': 4,
          'status': kSubjectFailed,
          'professor': 'Prof. de la Vega',
          'schedule': 'Vie · 14:00–17:00',
          'room': 'CJP11-102',
          'requisite': 'REQ: INF-330',
          'requisite_alert': true,
          'progress': 25,
          'semester': 'Semestre 6',
          'icon': Icons.lock_rounded,
          'units': [
            {
              'title': 'Unidad 1: Criptografía',
              'badge': 'próximo',
              'items': [
                {'title': 'Cifrado simétrico y asimétrico', 'checked': false, 'tag': 'Clase 1 – 5'},
              ]
            },
          ],
          'materials': [
            {'name': 'Apunte Criptografía.pdf', 'meta': 'PDF · 4.0 MB', 'icon': Icons.picture_as_pdf_rounded},
          ],
          'grades': [
            {'name': 'Certamen 1 (40%)', 'weight': 0.40, 'score': 35, 'status': 'graded'},
            {'name': 'Laboratorio (60%)', 'weight': 0.60, 'score': null, 'status': 'pending'},
          ],
        },
        {
          'id': 'redes',
          'code': 'INF-330',
          'name': 'Redes de Computadores',
          'credits': 5,
          'status': kSubjectApproved,
          'professor': 'Profesora Morales',
          'schedule': 'Lun / Jue · 11:30–13:00',
          'room': 'CJP11-101',
          'requisite': 'REQ: INF-100',
          'requisite_alert': false,
          'progress': 75,
          'semester': 'Semestre 6',
          'icon': Icons.public_rounded,
          'units': [
            {
              'title': 'Unidad 1: Modelo OSI y TCP/IP',
              'badge': 'completado',
              'items': [
                {'title': 'Capas y encapsulamiento', 'checked': true, 'tag': 'Clase 1 – 4'},
              ]
            },
            {
              'title': 'Unidad 2: Enrutamiento',
              'badge': 'completado',
              'items': [
                {'title': 'Algoritmos de enrutamiento', 'checked': true, 'tag': 'Clase 5 – 8'},
              ]
            },
          ],
          'materials': [
            {'name': 'Guía Laboratorio Redes.pdf', 'meta': 'PDF · 2.0 MB', 'icon': Icons.description_rounded},
          ],
          'grades': [
            {'name': 'Certamen 1 (30%)', 'weight': 0.30, 'score': 55, 'status': 'graded'},
            {'name': 'Certamen 2 (30%)', 'weight': 0.30, 'score': 60, 'status': 'graded'},
            {'name': 'Proyecto (40%)', 'weight': 0.40, 'score': 62, 'status': 'graded'},
          ],
        },
      ];
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
    final formKey = GlobalKey<FormState>();
    final nameCtrl = TextEditingController();
    final codeCtrl = TextEditingController();
    final creditsCtrl = TextEditingController(text: '5');
    final professorCtrl = TextEditingController();
    final scheduleCtrl = TextEditingController();
    final roomCtrl = TextEditingController();
    String status = kSubjectInProgress;

    final result = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (ctx) {
        return StatefulBuilder(builder: (ctx2, setD) {
          return AlertDialog(
            backgroundColor: AppColors.surface,
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius), side: const BorderSide(color: AppColors.border, width: 3)),
            title: const Text('CREAR NUEVO CURSO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
            content: SingleChildScrollView(
              child: Form(
                key: formKey,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const AppFieldLabel('NOMBRE DEL RAMO'),
                    const SizedBox(height: 6),
                    TextFormField(
                      controller: nameCtrl,
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
                            controller: codeCtrl,
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
                            controller: creditsCtrl,
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
                    TextFormField(controller: professorCtrl, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text), decoration: appInputDecoration('Prof. Apellido')),
                    const SizedBox(height: 12),
                    Row(children: [
                      Expanded(
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          const AppFieldLabel('HORARIO'),
                          const SizedBox(height: 6),
                          TextFormField(controller: scheduleCtrl, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text), decoration: appInputDecoration('Lun / Mié · 10:00')),
                        ]),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          const AppFieldLabel('SALA'),
                          const SizedBox(height: 6),
                          TextFormField(controller: roomCtrl, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text), decoration: appInputDecoration('CJP11-204')),
                        ]),
                      ),
                    ]),
                    const SizedBox(height: 12),
                    const AppFieldLabel('ESTADO'),
                    const SizedBox(height: 6),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 10),
                      decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
                      child: DropdownButtonHideUnderline(
                        child: DropdownButton<String>(
                          value: status,
                          isExpanded: true,
                          style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text),
                          items: const [
                            DropdownMenuItem(value: kSubjectInProgress, child: Text('Cursando')),
                            DropdownMenuItem(value: kSubjectPending, child: Text('Pendiente')),
                            DropdownMenuItem(value: kSubjectApproved, child: Text('Finalizado · Aprobado')),
                            DropdownMenuItem(value: kSubjectFailed, child: Text('Finalizado · Reprobado')),
                          ],
                          onChanged: (v) {
                            if (v != null) setD(() => status = v);
                          },
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            actions: [
              TextButton(onPressed: () => Navigator.of(ctx2).pop(), child: const Text('CANCELAR', style: TextStyle(fontWeight: FontWeight.w800, color: AppColors.muted))),
              ElevatedButton(
                onPressed: () {
                  if (!formKey.currentState!.validate()) return;
                  Navigator.of(ctx2).pop({
                    'name': nameCtrl.text.trim(),
                    'code': codeCtrl.text.trim().toUpperCase(),
                    'credits': int.tryParse(creditsCtrl.text) ?? 5,
                    'status': status,
                    'professor': professorCtrl.text.trim().isEmpty ? 'Por asignar' : professorCtrl.text.trim(),
                    'schedule': scheduleCtrl.text.trim().isEmpty ? 'Por definir' : scheduleCtrl.text.trim(),
                    'room': roomCtrl.text.trim().isEmpty ? 'Por asignar' : roomCtrl.text.trim(),
                  });
                },
                style: ElevatedButton.styleFrom(backgroundColor: AppColors.accentYellow, foregroundColor: AppColors.text, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius))),
                child: const Text('CREAR RAMO', style: TextStyle(fontWeight: FontWeight.w900)),
              ),
            ],
          );
        });
      },
    );

    nameCtrl.dispose();
    codeCtrl.dispose();
    creditsCtrl.dispose();
    professorCtrl.dispose();
    scheduleCtrl.dispose();
    roomCtrl.dispose();

    if (result != null) {
      setState(() {
        _courses.insert(0, {
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
      });
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Curso creado correctamente'), backgroundColor: AppColors.border));
    }
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
        decoration: BoxDecoration(color: cardBg, border: Border.all(color: AppColors.border, width: 4), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Container(width: 44, height: 44, alignment: Alignment.center, decoration: BoxDecoration(color: isAlert ? Colors.white : AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)), child: Icon(course['icon'] as IconData, size: 20, color: isAlert ? AppColors.error : AppColors.text)),
              const SizedBox(width: 10),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text((course['name'] as String).toUpperCase(), style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13.5, color: isAlert ? AppColors.error : AppColors.text, height: 1.1)),
                  const SizedBox(height: 2),
                  Text('${course['code']} · ${course['credits']} créditos', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: isAlert ? AppColors.error.withOpacity(0.85) : AppColors.muted)),
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
