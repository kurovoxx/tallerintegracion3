import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import 'academic_api.dart';
import 'academic_models.dart';
import 'grade_calculator_screen.dart';

/// Tab Unidades (UX-05b): checklist simple con check persistente (done),
/// crear/editar/borrar. Sin reorden: el orden lo da position al crear.
class ContentsTab extends StatefulWidget {
  const ContentsTab({super.key, required this.subjectId});

  final String subjectId;

  @override
  State<ContentsTab> createState() => _ContentsTabState();
}

class _ContentsTabState extends State<ContentsTab> {
  final AcademicApi _api = AcademicApi();
  bool _isLoading = true;
  String? _error;
  List<AcademicContent> _items = const [];

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _api.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (mounted) {
      setState(() {
        _isLoading = true;
        _error = null;
      });
    }
    try {
      final items = await _api.listContents(widget.subjectId);
      if (!mounted) return;
      setState(() {
        _items = items;
        _isLoading = false;
      });
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _error = e.message;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _error = 'Error inesperado. Reintenta.';
      });
    }
  }

  Future<void> _showEditDialog({AcademicContent? current}) async {
    final titleCtrl = TextEditingController(text: current?.title ?? '');
    final bodyCtrl = TextEditingController(text: current?.body ?? '');
    final result = await showDialog<Map<String, String?>>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: AppColors.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppDimens.radius),
          side: const BorderSide(color: AppColors.border, width: 3),
        ),
        title: Text(
          current == null ? 'NUEVO CONTENIDO' : 'EDITAR CONTENIDO',
          style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text),
        ),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const AppFieldLabel('TÍTULO'),
              const SizedBox(height: 6),
              TextField(
                controller: titleCtrl,
                style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                decoration: appInputDecoration('Ej. Unidad 1: Introducción'),
              ),
              const SizedBox(height: 12),
              const AppFieldLabel('CUERPO (OPCIONAL)'),
              const SizedBox(height: 6),
              TextField(
                controller: bodyCtrl,
                maxLines: 4,
                style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                decoration: appInputDecoration('Resumen del contenido…'),
              ),
            ],
          ),
        ),
        actions: [
          NeobrutalistButton(
            label: 'Cancelar',
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: () => Navigator.of(ctx).pop(),
          ),
          NeobrutalistButton(
            label: current == null ? 'Crear' : 'Guardar',
            icon: Icons.check_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: () {
              final title = titleCtrl.text.trim();
              if (title.isEmpty) {
                ScaffoldMessenger.of(ctx).showSnackBar(
                  const SnackBar(content: Text('El título es requerido'), backgroundColor: AppColors.border),
                );
                return;
              }
              Navigator.of(ctx).pop({'title': title, 'body': bodyCtrl.text.trim()});
            },
          ),
        ],
      ),
    );
    titleCtrl.dispose();
    bodyCtrl.dispose();
    if (result == null || !mounted) return;
    try {
      if (current == null) {
        final position = _items.isEmpty
            ? 0
            : _items.map((c) => c.position).reduce((a, b) => a > b ? a : b) + 1;
        await _api.createContent(
          subjectId: widget.subjectId,
          title: result['title']!,
          body: result['body'],
          position: position,
        );
      } else {
        await _api.patchContent(current.id, {
          'title': result['title'],
          'body': (result['body'] ?? '').isEmpty ? '' : result['body'],
        });
      }
      await _load();
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('No se pudo guardar: ${e.message}'), backgroundColor: AppColors.border),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No se pudo guardar: error inesperado'), backgroundColor: AppColors.border),
      );
    }
  }

  Future<void> _delete(String id) async {
    try {
      await _api.deleteContent(id);
      await _load();
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('No se pudo eliminar: ${e.message}'), backgroundColor: AppColors.border),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No se pudo eliminar: error inesperado'), backgroundColor: AppColors.border),
      );
    }
  }

  /// Toggle del check persistente (PATCH done). Sin sesión: error visible.
  Future<void> _toggleDone(AcademicContent c, bool value) async {
    try {
      await _api.patchContent(c.id, {'done': value});
      await _load();
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('No se pudo actualizar: ${e.message}'), backgroundColor: AppColors.border),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No se pudo actualizar: error inesperado'), backgroundColor: AppColors.border),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return Container(
        height: 160,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
        child: const CircularProgressIndicator(color: AppColors.text),
      );
    }
    if (_error != null) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(_error!, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.muted)),
            const SizedBox(height: 12),
            NeobrutalistButton(label: 'Reintentar', icon: Icons.refresh_rounded, variant: NeobrutalistButtonVariant.secondary, onPressed: _load),
          ],
        ),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Align(
          alignment: Alignment.centerRight,
          child: NeobrutalistButton(
            label: 'Agregar contenido',
            icon: Icons.add_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: () => _showEditDialog(),
          ),
        ),
        const SizedBox(height: 12),
        if (_items.isEmpty)
          Container(
            padding: const EdgeInsets.all(20),
            alignment: Alignment.center,
            decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
            child: const Text(
              'SIN CONTENIDOS: AGREGA LA PRIMERA UNIDAD',
              textAlign: TextAlign.center,
              style: TextStyle(fontWeight: FontWeight.w800, fontSize: 12, color: AppColors.muted),
            ),
          ),
        for (final c in _items) _checkRow(c),
      ],
    );
  }

  /// Fila checklist: check persistente + título/cuerpo + editar + borrar.
  Widget _checkRow(AcademicContent c) {
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
      child: Row(
        children: [
          Checkbox(
            value: c.done,
            activeColor: AppColors.accentYellow,
            checkColor: AppColors.text,
            side: const BorderSide(color: AppColors.border, width: 2),
            onChanged: (v) => _toggleDone(c, v ?? false),
          ),
          const SizedBox(width: 6),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  c.title,
                  style: TextStyle(
                    fontWeight: FontWeight.w800,
                    fontSize: 13,
                    color: c.done ? AppColors.muted : AppColors.text,
                    decoration: c.done ? TextDecoration.lineThrough : null,
                  ),
                ),
                if (c.body != null && c.body!.isNotEmpty) ...[
                  const SizedBox(height: 2),
                  Text(
                    c.body!,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted),
                  ),
                ],
              ],
            ),
          ),
          IconButton(
            tooltip: 'Editar',
            icon: const Icon(Icons.edit_rounded, size: 20),
            onPressed: () => _showEditDialog(current: c),
          ),
          IconButton(
            tooltip: 'Eliminar',
            icon: const Icon(Icons.delete_rounded, size: 20),
            onPressed: () => _delete(c.id),
          ),
        ],
      ),
    );
  }
}

/// Tab Evaluaciones (UX-05): notas backend con pin (una por materia),
/// renombrar por PATCH y entrada a la calculadora (simulación local).
class EvaluationsTab extends StatefulWidget {
  const EvaluationsTab({super.key, required this.subjectId, this.subjectName = ''});

  final String subjectId;
  final String subjectName;

  @override
  State<EvaluationsTab> createState() => _EvaluationsTabState();
}

class _EvaluationsTabState extends State<EvaluationsTab> {
  final AcademicApi _api = AcademicApi();
  bool _isLoading = true;
  String? _error;
  List<AcademicGradeNode> _nodes = const [];
  double? _average;
  String? _averageStatus;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _api.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (mounted) {
      setState(() {
        _isLoading = true;
        _error = null;
      });
    }
    try {
      final nodes = await _api.listGrades(widget.subjectId);
      AcademicAverage? avg;
      try {
        avg = await _api.getAverage(widget.subjectId);
      } catch (_) {
        avg = null;
      }
      if (!mounted) return;
      setState(() {
        _nodes = nodes;
        _average = avg?.average;
        _averageStatus = avg?.status;
        _isLoading = false;
      });
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _error = e.message;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _error = 'Error inesperado. Reintenta.';
      });
    }
  }

  Future<void> _togglePin(AcademicGrade grade) async {
    try {
      await _api.pinGrade(grade.id, !grade.pinned);
      await _load();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(grade.pinned ? 'Nota despinneada' : 'Nota fijada (despinnea la anterior)'),
          backgroundColor: AppColors.border,
        ),
      );
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('No se pudo fijar: ${e.message}'), backgroundColor: AppColors.border),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No se pudo fijar: error inesperado'), backgroundColor: AppColors.border),
      );
    }
  }

  Future<void> _rename(AcademicGrade grade) async {
    final ctrl = TextEditingController(text: grade.name);
    final name = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: AppColors.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppDimens.radius),
          side: const BorderSide(color: AppColors.border, width: 3),
        ),
        title: const Text('RENOMBRAR NOTA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
        content: TextField(
          controller: ctrl,
          style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
          decoration: appInputDecoration('Nombre de la evaluación'),
        ),
        actions: [
          NeobrutalistButton(label: 'Cancelar', variant: NeobrutalistButtonVariant.secondary, onPressed: () => Navigator.of(ctx).pop()),
          NeobrutalistButton(
            label: 'Guardar',
            icon: Icons.check_rounded,
            variant: NeobrutalistButtonVariant.accent,
            onPressed: () => Navigator.of(ctx).pop(ctrl.text.trim()),
          ),
        ],
      ),
    );
    ctrl.dispose();
    if (name == null || !mounted) return;
    if (name.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('El nombre no puede estar vacío'), backgroundColor: AppColors.border),
      );
      return;
    }
    try {
      await _api.renameGrade(grade.id, name);
      await _load();
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('No se pudo renombrar: ${e.message}'), backgroundColor: AppColors.border),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No se pudo renombrar: error inesperado'), backgroundColor: AppColors.border),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return Container(
        height: 160,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
        child: const CircularProgressIndicator(color: AppColors.text),
      );
    }
    if (_error != null) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(_error!, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.muted)),
            const SizedBox(height: 12),
            NeobrutalistButton(label: 'Reintentar', icon: Icons.refresh_rounded, variant: NeobrutalistButtonVariant.secondary, onPressed: _load),
          ],
        ),
      );
    }
    final pinned = _nodes.where((n) => n.grade.pinned).toList();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (_average != null)
          Container(
            margin: const EdgeInsets.only(bottom: 12),
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
            child: Text(
              'PROMEDIO BACKEND: ${_average!.toStringAsFixed(1)}${_averageStatus != null ? ' (${_averageStatus!})' : ''}',
              style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text),
            ),
          ),
        if (pinned.isNotEmpty)
          Container(
            margin: const EdgeInsets.only(bottom: 12),
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)]),
            child: Row(
              children: [
                const Icon(Icons.push_pin_rounded, size: 18, color: AppColors.text),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'FIJA: ${pinned.first.grade.name.toUpperCase()}',
                    style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 12.5, color: AppColors.text),
                  ),
                ),
              ],
            ),
          ),
        for (final node in _nodes) _gradeRow(node.grade),
        const SizedBox(height: 20),
        const Text(
          'SIMULAR EN CALCULADORA',
          style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.text),
        ),
        const SizedBox(height: 10),
        GradeCalculatorScreen(
          embedded: true,
          initialCourse: widget.subjectName.isEmpty ? null : widget.subjectName,
        ),
      ],
    );
  }

  Widget _gradeRow(AcademicGrade grade) {
    final score = grade.score != null ? (grade.score! / 10.0).toStringAsFixed(1) : '–';
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    if (grade.pinned)
                      const Padding(
                        padding: EdgeInsets.only(right: 6),
                        child: Icon(Icons.push_pin_rounded, size: 14, color: AppColors.text),
                      ),
                    Expanded(
                      child: Text(grade.name, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text)),
                    ),
                  ],
                ),
                const SizedBox(height: 2),
                Text('Ponderación ${(grade.weight * 100).toStringAsFixed(0)}%', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted)),
              ],
            ),
          ),
          Text(score, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 24, color: AppColors.text)),
          IconButton(
            tooltip: grade.pinned ? 'Quitar fija' : 'Mantener nota (fijar)',
            icon: Icon(grade.pinned ? Icons.push_pin_rounded : Icons.push_pin_outlined, size: 20),
            onPressed: () => _togglePin(grade),
          ),
          IconButton(
            tooltip: 'Renombrar',
            icon: const Icon(Icons.edit_rounded, size: 20),
            onPressed: () => _rename(grade),
          ),
        ],
      ),
    );
  }
}
