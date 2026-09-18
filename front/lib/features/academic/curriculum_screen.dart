import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';

const String kSubjectApproved = 'approved';
const String kSubjectInProgress = 'in_progress';
const String kSubjectFailed = 'failed';
const String kSubjectPending = 'pending';

class CurriculumScreen extends StatefulWidget {
  const CurriculumScreen({super.key});

  @override
  State<CurriculumScreen> createState() => _CurriculumScreenState();
}

class _CurriculumScreenState extends State<CurriculumScreen> {
  bool _isLoading = true;
  bool _hasError = false;

  final ScrollController _horizontalScrollController = ScrollController();

  // semestre -> ramos
  Map<int, List<Map<String, dynamic>>> _semesters = {};
  // snapshot para limpiar
  Map<int, List<Map<String, dynamic>>>? _snapshot;

  @override
  void initState() {
    super.initState();
    _loadCurriculum();
  }

  @override
  void dispose() {
    _horizontalScrollController.dispose();
    super.dispose();
  }

  Future<void> _loadCurriculum() async {
    setState(() {
      _isLoading = true;
      _hasError = false;
    });
    try {
      await Future.delayed(const Duration(milliseconds: 620));
      _semesters = {
        1: [
          {'id': 'info1001', 'code': 'INFO1001', 'name': 'Programación I', 'credits': 6, 'status': kSubjectApproved, 'requisite': null},
          {'id': 'mat1001', 'code': 'MAT1001', 'name': 'Matemática I', 'credits': 6, 'status': kSubjectApproved, 'requisite': null},
          {'id': 'fis1000', 'code': 'FIS1000', 'name': 'Física General I', 'credits': 5, 'status': kSubjectApproved, 'requisite': null},
          {'id': 'com1000', 'code': 'COM1000', 'name': 'Comunicación Efectiva', 'credits': 3, 'status': kSubjectApproved, 'requisite': null},
        ],
        2: [
          {'id': 'info1002', 'code': 'CC1001', 'name': 'Programación II', 'credits': 6, 'status': kSubjectPending, 'requisite': 'CC1001', 'requisite_label': 'Req: CC1001'},
          {'id': 'mat1002', 'code': 'MAT1002', 'name': 'Matemática II', 'credits': 6, 'status': kSubjectInProgress, 'requisite': 'MAT1001'},
          {'id': 'taller1', 'code': 'INFO1005', 'name': 'Taller de Integración 1', 'credits': 4, 'status': kSubjectFailed, 'requisite': 'MA1101'},
        ],
        3: [
          {'id': 'info2001', 'code': 'INFO2001', 'name': 'Estructuras de Datos', 'credits': 6, 'status': kSubjectInProgress, 'requisite': 'CC1001'},
          {'id': 'arq2001', 'code': 'INF210', 'name': 'Arquitectura de Hardware', 'credits': 5, 'status': kSubjectPending, 'requisite': null},
        ],
        4: [
          {'id': 'bd2001', 'code': 'INF220', 'name': 'Bases de Datos', 'credits': 5, 'status': kSubjectPending, 'requisite': 'INFO2001'},
        ],
        5: [],
        6: [
          {'id': 'calc3', 'code': 'INF-1111', 'name': 'Cálculo III', 'credits': 5, 'status': kSubjectInProgress, 'requisite': 'MA1101'},
          {'id': 'taller3', 'code': 'INF-360', 'name': 'Taller de Integración III', 'credits': 4, 'status': kSubjectPending, 'requisite': 'INF-200'},
          {'id': 'seginf', 'code': 'INF-350', 'name': 'Seguridad Informática', 'credits': 4, 'status': kSubjectFailed, 'requisite': 'INF-330'},
          {'id': 'redes', 'code': 'INF-330', 'name': 'Redes de Computadores', 'credits': 5, 'status': kSubjectApproved, 'requisite': 'INF-100'},
        ],
      };
      // snapshot deep copy
      _snapshot = _deepCopy(_semesters);
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

  Map<int, List<Map<String, dynamic>>> _deepCopy(Map<int, List<Map<String, dynamic>>> src) {
    final out = <int, List<Map<String, dynamic>>>{};
    src.forEach((k, v) {
      out[k] = v.map((e) => Map<String, dynamic>.from(e)).toList();
    });
    return out;
  }

  int _creditsForSemester(int sem) {
    final list = _semesters[sem] ?? [];
    return list.fold<int>(0, (sum, r) => sum + (r['credits'] as int));
  }

  int get _totalCreditsApproved {
    int total = 0;
    _semesters.forEach((_, list) {
      for (final r in list) {
        if (r['status'] == kSubjectApproved) total += r['credits'] as int;
      }
    });
    return total;
  }

  int get _totalCreditsAll {
    int total = 0;
    _semesters.forEach((_, list) {
      for (final r in list) total += r['credits'] as int;
    });
    return total;
  }

  void _handleClearAll() async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: AppColors.surface,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius), side: const BorderSide(color: AppColors.border, width: 3)),
        title: const Text('LIMPIAR MALLA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
        content: const Text('¿Seguro que quieres limpiar todos los ramos? Esta acción dejará los semestres vacíos.', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.muted)),
        actions: [
          TextButton(onPressed: () => Navigator.of(ctx).pop(false), child: const Text('CANCELAR', style: TextStyle(fontWeight: FontWeight.w800, color: AppColors.muted))),
          ElevatedButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            style: ElevatedButton.styleFrom(backgroundColor: AppColors.error, foregroundColor: Colors.white, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius))),
            child: const Text('LIMPIAR TODO', style: TextStyle(fontWeight: FontWeight.w900)),
          ),
        ],
      ),
    );
    if (confirm == true) {
      setState(() {
        for (final k in _semesters.keys.toList()) {
          _semesters[k] = [];
        }
      });
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Malla limpiada'), backgroundColor: AppColors.border));
    }
  }

  void _handleRestoreSnapshot() {
    if (_snapshot == null) return;
    setState(() => _semesters = _deepCopy(_snapshot!));
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Malla restaurada'), backgroundColor: AppColors.border));
  }

  void _handleSave() {
    _snapshot = _deepCopy(_semesters);
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Malla guardada correctamente'), backgroundColor: AppColors.border));
  }

  Future<void> _showAddSubjectDialog(int semester) async {
    final result = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (_) => _AddSubjectDialog(semester: semester),
    );

    if (result != null) {
      setState(() {
        final list = _semesters[semester] ?? [];
        list.add({
          'id': 'ramo-${DateTime.now().microsecondsSinceEpoch}',
          'code': result['code'],
          'name': result['name'],
          'credits': result['credits'],
          'status': result['status'],
          'requisite': null,
        });
        _semesters[semester] = list;
      });
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Ramo "${result['name']}" añadido a Semestre $semester'), backgroundColor: AppColors.border));
    }
  }

  void _removeSubject(int semester, String id) {
    setState(() => _semesters[semester] = (_semesters[semester] ?? []).where((r) => r['id'] != id).toList());
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Ramo eliminado'), backgroundColor: AppColors.border));
  }

  void _cycleStatus(int semester, String id) {
    setState(() {
      final list = _semesters[semester] ?? [];
      final idx = list.indexWhere((r) => r['id'] == id);
      if (idx == -1) return;
      final current = list[idx]['status'] as String;
      String next;
      switch (current) {
        case kSubjectApproved:
          next = kSubjectInProgress;
          break;
        case kSubjectInProgress:
          next = kSubjectPending;
          break;
        case kSubjectPending:
          next = kSubjectFailed;
          break;
        case kSubjectFailed:
        default:
          next = kSubjectApproved;
      }
      list[idx]['status'] = next;
    });
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
                padding: EdgeInsets.symmetric(horizontal: isDesktop ? 32 : 16, vertical: 20),
                child: Center(
                  child: ConstrainedBox(
                    constraints: BoxConstraints(maxWidth: isDesktop ? 1400 : 640),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _buildHeader(canPop, isDesktop),
                        const SizedBox(height: 18),
                        if (_isLoading)
                          _buildLoadingCard()
                        else
                          _buildMallaContent(isDesktop),
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
      Text('MI MALLA CURRICULAR', style: TextStyle(color: AppColors.text, fontWeight: FontWeight.w900, fontSize: isDesktop ? 28 : 22, letterSpacing: -0.6)),
      const SizedBox(height: 4),
      const Text('Planifica y visualiza tu progreso académico.', style: TextStyle(color: AppColors.muted, fontWeight: FontWeight.w700, fontSize: 13)),
      const SizedBox(height: 6),
      Wrap(spacing: 8, runSpacing: 6, children: [
        Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4), decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text('CRÉDITOS APROBADOS: $_totalCreditsApproved', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text))),
        Container(padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4), decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Text('TOTAL MALLA: $_totalCreditsAll créditos', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.muted))),
      ]),
    ]);

    final actions = Wrap(
      spacing: 10,
      runSpacing: 10,
      children: [
        OutlinedButton.icon(
          onPressed: _handleClearAll,
          style: OutlinedButton.styleFrom(side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12), backgroundColor: AppColors.surface),
          icon: const Icon(Icons.delete_sweep_rounded, size: 18, color: AppColors.text),
          label: const Text('LIMPIAR TODO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text)),
        ),
        ElevatedButton.icon(
          onPressed: _handleSave,
          style: ElevatedButton.styleFrom(backgroundColor: AppColors.border, foregroundColor: Colors.white, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius)), elevation: 0, padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12)),
          icon: const Icon(Icons.save_rounded, size: 18),
          label: const Text('GUARDAR MALLA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
        ),
        if (_snapshot != null)
          InkWell(
            onTap: _handleRestoreSnapshot,
            borderRadius: BorderRadius.circular(AppDimens.radius),
            child: Container(padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8), decoration: BoxDecoration(border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(AppDimens.radius)), child: const Text('RESTAURAR', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.muted))),
          ),
      ],
    );

    if (isDesktop) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            if (canPop) ...[
              _SquareIconButton(icon: Icons.arrow_back_rounded, tooltip: 'Volver', onPressed: () => Navigator.of(context).pop()),
              const SizedBox(width: 12),
            ],
            Expanded(child: titleBlock),
            const SizedBox(width: 16),
            actions,
          ]),
          const SizedBox(height: 14),
          Container(height: 4, decoration: BoxDecoration(color: AppColors.border, borderRadius: BorderRadius.circular(2))),
        ],
      );
    }

    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        if (canPop) _SquareIconButton(icon: Icons.arrow_back_rounded, tooltip: 'Volver', onPressed: () => Navigator.of(context).pop()),
        if (canPop) const SizedBox(width: 10),
        Expanded(child: titleBlock),
        _SquareIconButton(icon: Icons.refresh_rounded, tooltip: 'Recargar', onPressed: _loadCurriculum),
      ]),
      const SizedBox(height: 14),
      actions,
      const SizedBox(height: 14),
      Container(height: 4, decoration: BoxDecoration(color: AppColors.border, borderRadius: BorderRadius.circular(2))),
    ]);
  }

  Widget _buildMallaContent(bool isDesktop) {
    // Ordena semestres numéricamente
    final semesterKeys = _semesters.keys.toList()..sort();
    // Asegura que al menos se muestren semestres 1..8 aunque algunos vacíos
    for (int i = 1; i <= 8; i++) {
      _semesters.putIfAbsent(i, () => []);
      if (!semesterKeys.contains(i)) semesterKeys.add(i);
    }
    semesterKeys.sort();

    final isMobile = MediaQuery.of(context).size.width < 700;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: isMobile ? 480 : 520,
          child: Scrollbar(
            controller: _horizontalScrollController,
            thumbVisibility: true,
            child: ScrollConfiguration(
              behavior: ScrollConfiguration.of(context).copyWith(
                dragDevices: {
                  PointerDeviceKind.touch,
                  PointerDeviceKind.mouse,
                  PointerDeviceKind.trackpad,
                },
              ),
              child: ListView.separated(
                controller: _horizontalScrollController,
                scrollDirection: Axis.horizontal,
                padding: const EdgeInsets.only(bottom: 12, top: 4, right: 4),
                itemCount: semesterKeys.length,
                separatorBuilder: (_, __) => const SizedBox(width: 16),
                itemBuilder: (context, idx) {
                  final sem = semesterKeys[idx];
                  final ramos = _semesters[sem] ?? [];
                  return _SemesterColumn(
                    semester: sem,
                    credits: _creditsForSemester(sem),
                    ramos: ramos,
                    onAdd: () => _showAddSubjectDialog(sem),
                    onRemove: (id) => _removeSubject(sem, id),
                    onCycleStatus: (id) => _cycleStatus(sem, id),
                  );
                },
              ),
            ),
          ),
        ),
        const SizedBox(height: 10),
        Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)),
          child: Wrap(
            spacing: 14,
            runSpacing: 8,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              Row(mainAxisSize: MainAxisSize.min, children: [
                Container(width: 10, height: 10, decoration: const BoxDecoration(color: AppColors.accentYellow, shape: BoxShape.circle, border: Border.fromBorderSide(BorderSide(color: AppColors.border, width: 1)))),
                const SizedBox(width: 8),
                const Text('Aprobado', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.text)),
              ]),
              Row(mainAxisSize: MainAxisSize.min, children: [
                Container(width: 10, height: 10, decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 1), shape: BoxShape.circle)),
                const SizedBox(width: 8),
                const Text('En Curso / Pendiente', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.text)),
              ]),
              Row(mainAxisSize: MainAxisSize.min, children: [
                Container(width: 10, height: 10, decoration: const BoxDecoration(color: Color(0xFFFFDAD6), shape: BoxShape.circle, border: Border.fromBorderSide(BorderSide(color: AppColors.border, width: 1)))),
                const SizedBox(width: 8),
                const Text('Reprobado', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.text)),
              ]),
              const Text('Tap en ícono para cambiar estado', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 10, color: AppColors.muted)),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildLoadingCard() {
    return Container(height: 420, decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]), child: const Center(child: CircularProgressIndicator(color: AppColors.text)));
  }

  Widget _buildErrorState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Icon(Icons.error_outline_rounded, size: 48, color: AppColors.text),
          const SizedBox(height: 12),
          const Text('NO SE PUDO CARGAR LA MALLA', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
          const SizedBox(height: 8),
          const Text('Verifica tu conexión e intenta nuevamente.', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.muted)),
          const SizedBox(height: 18),
          SizedBox(width: 200, child: SubmitButton(text: 'REINTENTAR', onPressed: _loadCurriculum)),
        ]),
      ),
    );
  }
}

class _AddSubjectDialog extends StatefulWidget {
  final int semester;
  const _AddSubjectDialog({required this.semester});

  @override
  State<_AddSubjectDialog> createState() => _AddSubjectDialogState();
}

class _AddSubjectDialogState extends State<_AddSubjectDialog> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _nameCtrl;
  late final TextEditingController _codeCtrl;
  late final TextEditingController _creditsCtrl;
  String _status = kSubjectPending;

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController();
    _codeCtrl = TextEditingController();
    _creditsCtrl = TextEditingController(text: '5');
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _codeCtrl.dispose();
    _creditsCtrl.dispose();
    super.dispose();
  }

  void _submit() {
    if (!_formKey.currentState!.validate()) return;
    Navigator.of(context).pop({
      'name': _nameCtrl.text.trim(),
      'code': _codeCtrl.text.trim().toUpperCase(),
      'credits': int.tryParse(_creditsCtrl.text) ?? 5,
      'status': _status,
    });
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppColors.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius), side: const BorderSide(color: AppColors.border, width: 3)),
      title: Text('AÑADIR RAMO · SEMESTRE ${widget.semester}', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
      content: SingleChildScrollView(
        child: Form(
          key: _formKey,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const AppFieldLabel('NOMBRE DE LA ASIGNATURA'),
              const SizedBox(height: 6),
              TextFormField(
                controller: _nameCtrl,
                style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                decoration: appInputDecoration('Ej. Programación III'),
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
                      decoration: appInputDecoration('INFO1001'),
                      inputFormatters: [LengthLimitingTextInputFormatter(20)],
                      validator: (v) {
                        if (v == null || v.trim().isEmpty) return 'Requerido';
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
                        if (n < 1 || n > 10) return '1-10';
                        return null;
                      },
                    ),
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
                    value: _status,
                    isExpanded: true,
                    style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text),
                    items: const [
                      DropdownMenuItem(value: kSubjectApproved, child: Text('Aprobado')),
                      DropdownMenuItem(value: kSubjectInProgress, child: Text('En Curso')),
                      DropdownMenuItem(value: kSubjectPending, child: Text('Pendiente')),
                      DropdownMenuItem(value: kSubjectFailed, child: Text('Reprobado')),
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
        TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('CANCELAR', style: TextStyle(fontWeight: FontWeight.w800, color: AppColors.muted))),
        ElevatedButton(
          onPressed: _submit,
          style: ElevatedButton.styleFrom(backgroundColor: AppColors.accentYellow, foregroundColor: AppColors.text, side: const BorderSide(color: AppColors.border, width: 2), shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppDimens.radius))),
          child: const Text('AÑADIR', style: TextStyle(fontWeight: FontWeight.w900)),
        ),
      ],
    );
  }
}

class _SemesterColumn extends StatelessWidget {
  final int semester;
  final int credits;
  final List<Map<String, dynamic>> ramos;
  final VoidCallback onAdd;
  final ValueChanged<String> onRemove;
  final ValueChanged<String> onCycleStatus;

  const _SemesterColumn({required this.semester, required this.credits, required this.ramos, required this.onAdd, required this.onRemove, required this.onCycleStatus});

  @override
  Widget build(BuildContext context) {
    final isEmpty = ramos.isEmpty;
    return Container(
      width: 300,
      decoration: BoxDecoration(
        color: isEmpty ? AppColors.bg.withOpacity(0.6) : AppColors.surface,
        border: Border.all(color: AppColors.border, width: isEmpty ? 3 : 4, style: isEmpty ? BorderStyle.solid : BorderStyle.solid),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: isEmpty ? null : const [BoxShadow(color: AppColors.border, offset: Offset(6, 6), blurRadius: 0)],
      ),
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Header
          Container(
            padding: const EdgeInsets.only(bottom: 10),
            decoration: BoxDecoration(border: Border(bottom: BorderSide(color: AppColors.border, width: isEmpty ? 3 : 4, style: isEmpty ? BorderStyle.solid : BorderStyle.solid))),
            child: Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
              Text('SEMESTRE $semester', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text, letterSpacing: 0.3)),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(color: AppColors.border, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                child: Text('$credits créditos', style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: Colors.white)),
              ),
            ]),
          ),
          const SizedBox(height: 12),
          Expanded(
            child: isEmpty
                ? Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Container(width: 56, height: 56, decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2, style: BorderStyle.solid), borderRadius: BorderRadius.circular(AppDimens.radius)), child: const Icon(Icons.drag_indicator_rounded, size: 28, color: AppColors.muted)),
                      const SizedBox(height: 12),
                      const Text('Arrastra ramos aquí o añade nuevos.', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: AppColors.muted)),
                    ],
                  )
                : ListView.separated(
                    itemCount: ramos.length,
                    separatorBuilder: (_, __) => const SizedBox(height: 10),
                    itemBuilder: (context, i) {
                      final r = ramos[i];
                      return _CurriculumCard(ramo: r, onRemove: () => onRemove(r['id'] as String), onCycle: () => onCycleStatus(r['id'] as String));
                    },
                  ),
          ),
          const SizedBox(height: 12),
          InkWell(
            onTap: onAdd,
            borderRadius: BorderRadius.circular(AppDimens.radius),
            child: Container(
              padding: const EdgeInsets.symmetric(vertical: 10),
              decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 2, style: BorderStyle.solid), borderRadius: BorderRadius.circular(AppDimens.radius)),
              child: Row(mainAxisAlignment: MainAxisAlignment.center, children: const [Icon(Icons.add_rounded, size: 16, color: AppColors.text), SizedBox(width: 6), Text('AÑADIR RAMO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text))]),
            ),
          ),
        ],
      ),
    );
  }
}

class _CurriculumCard extends StatelessWidget {
  final Map<String, dynamic> ramo;
  final VoidCallback onRemove;
  final VoidCallback onCycle;
  const _CurriculumCard({required this.ramo, required this.onRemove, required this.onCycle});

  @override
  Widget build(BuildContext context) {
    final status = ramo['status'] as String;
    final isApproved = status == kSubjectApproved;
    final isFailed = status == kSubjectFailed;
    final isPending = status == kSubjectPending || status == kSubjectInProgress;

    Color topColor;
    Color cardBg;
    if (isFailed) {
      topColor = AppColors.error;
      cardBg = const Color(0xFFFFDAD6);
    } else if (isApproved) {
      topColor = AppColors.accentYellow;
      cardBg = AppColors.surface;
    } else {
      topColor = AppColors.muted.withOpacity(0.6);
      cardBg = AppColors.surface;
    }

    IconData statusIcon;
    Color statusIconColor;
    switch (status) {
      case kSubjectApproved:
        statusIcon = Icons.check_box_rounded;
        statusIconColor = AppColors.text;
        break;
      case kSubjectFailed:
        statusIcon = Icons.disabled_by_default_rounded;
        statusIconColor = AppColors.error;
        break;
      case kSubjectInProgress:
        statusIcon = Icons.hourglass_bottom_rounded;
        statusIconColor = AppColors.accentBlue;
        break;
      case kSubjectPending:
      default:
        statusIcon = Icons.check_box_outline_blank_rounded;
        statusIconColor = AppColors.muted;
    }

    final requisite = ramo['requisite'] as String?;
    final requisiteLabel = ramo['requisite_label'] as String? ?? (requisite != null ? 'Req: $requisite' : null);

    return Container(
      decoration: BoxDecoration(color: cardBg, border: Border.all(color: AppColors.border, width: isFailed ? 3 : 2), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(height: 6, decoration: BoxDecoration(color: topColor, borderRadius: const BorderRadius.vertical(top: Radius.circular(AppDimens.radius - 1)), border: const Border(bottom: BorderSide(color: AppColors.border, width: 1)))),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 10, 8, 10),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(ramo['name'] as String, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12.5, color: isFailed ? AppColors.error : AppColors.text, height: 1.1)),
                    const SizedBox(height: 6),
                    Row(children: [
                      Container(padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3), decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Row(mainAxisSize: MainAxisSize.min, children: [const Icon(Icons.sell_rounded, size: 10, color: AppColors.text), const SizedBox(width: 4), Text(ramo['code'] as String, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.text))])),
                      const SizedBox(width: 6),
                      Text('${ramo['credits']} cr.', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 10, color: AppColors.muted)),
                    ]),
                    if (isPending && requisiteLabel != null) ...[
                      const SizedBox(height: 6),
                      Container(padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3), decoration: BoxDecoration(color: AppColors.accentBlue, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Row(mainAxisSize: MainAxisSize.min, children: [const Icon(Icons.link_rounded, size: 10, color: Colors.white), const SizedBox(width: 4), Text(requisiteLabel, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 9, color: Colors.white))])),
                    ],
                    if (isFailed && requisiteLabel != null) ...[
                      const SizedBox(height: 6),
                      Container(padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3), decoration: BoxDecoration(color: AppColors.accentBlue, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)), child: Row(mainAxisSize: MainAxisSize.min, children: [const Icon(Icons.link_rounded, size: 10, color: Colors.white), const SizedBox(width: 4), Text(requisiteLabel, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 9, color: Colors.white))])),
                    ],
                  ]),
                ),
                const SizedBox(width: 8),
                Column(children: [
                  InkWell(onTap: onCycle, borderRadius: BorderRadius.circular(4), child: Icon(statusIcon, size: 22, color: statusIconColor)),
                  const SizedBox(height: 6),
                  InkWell(
                    onTap: onRemove,
                    borderRadius: BorderRadius.circular(4),
                    child: Container(padding: const EdgeInsets.all(4), decoration: BoxDecoration(border: Border.all(color: AppColors.border.withOpacity(0.5), width: 1), borderRadius: BorderRadius.circular(4)), child: const Icon(Icons.close_rounded, size: 12, color: AppColors.muted)),
                  ),
                ]),
              ],
            ),
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
