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
          style: const TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 14,
            color: AppColors.text,
          ),
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
                style: const TextStyle(
                  fontWeight: FontWeight.w700,
                  fontSize: 13,
                  color: AppColors.text,
                ),
                decoration: appInputDecoration('Ej. Unidad 1: Introducción'),
              ),
              const SizedBox(height: 12),
              const AppFieldLabel('CUERPO (OPCIONAL)'),
              const SizedBox(height: 6),
              TextField(
                controller: bodyCtrl,
                maxLines: 4,
                style: const TextStyle(
                  fontWeight: FontWeight.w700,
                  fontSize: 13,
                  color: AppColors.text,
                ),
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
                  const SnackBar(
                    content: Text('El título es requerido'),
                    backgroundColor: AppColors.border,
                  ),
                );
                return;
              }
              Navigator.of(
                ctx,
              ).pop({'title': title, 'body': bodyCtrl.text.trim()});
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
        SnackBar(
          content: Text('No se pudo guardar: ${e.message}'),
          backgroundColor: AppColors.border,
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo guardar: error inesperado'),
          backgroundColor: AppColors.border,
        ),
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
        SnackBar(
          content: Text('No se pudo eliminar: ${e.message}'),
          backgroundColor: AppColors.border,
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo eliminar: error inesperado'),
          backgroundColor: AppColors.border,
        ),
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
        SnackBar(
          content: Text('No se pudo actualizar: ${e.message}'),
          backgroundColor: AppColors.border,
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo actualizar: error inesperado'),
          backgroundColor: AppColors.border,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return Container(
        height: 160,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
        ),
        child: const CircularProgressIndicator(color: AppColors.text),
      );
    }
    if (_error != null) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              _error!,
              style: const TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 13,
                color: AppColors.muted,
              ),
            ),
            const SizedBox(height: 12),
            NeobrutalistButton(
              label: 'Reintentar',
              icon: Icons.refresh_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: _load,
            ),
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
            decoration: BoxDecoration(
              color: AppColors.surface,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radius),
            ),
            child: const Text(
              'SIN CONTENIDOS: AGREGA LA PRIMERA UNIDAD',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontWeight: FontWeight.w800,
                fontSize: 12,
                color: AppColors.muted,
              ),
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
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [
          BoxShadow(
            color: AppColors.border,
            offset: Offset(2, 2),
            blurRadius: 0,
          ),
        ],
      ),
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
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 11,
                      color: AppColors.muted,
                    ),
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

/// Tab Evaluaciones (UX-05): notas backend con renombrar por PATCH y
/// calculadora embebida (simulación local).
class EvaluationsTab extends StatefulWidget {
  const EvaluationsTab({
    super.key,
    required this.subjectId,
    this.subjectName = '',
  });

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

  /// Recarga notas + promedio. En [silent] no toca el spinner para no
  /// resetear la calculadora embebida (caso onSaved tras guardar).
  Future<void> _load({bool silent = false}) async {
    if (mounted && !silent) {
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
        title: const Text(
          'RENOMBRAR NOTA',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 14,
            color: AppColors.text,
          ),
        ),
        content: TextField(
          controller: ctrl,
          style: const TextStyle(
            fontWeight: FontWeight.w700,
            fontSize: 13,
            color: AppColors.text,
          ),
          decoration: appInputDecoration('Nombre de la evaluación'),
        ),
        actions: [
          NeobrutalistButton(
            label: 'Cancelar',
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: () => Navigator.of(ctx).pop(),
          ),
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
        const SnackBar(
          content: Text('El nombre no puede estar vacío'),
          backgroundColor: AppColors.border,
        ),
      );
      return;
    }
    try {
      await _api.renameGrade(grade.id, name);
      await _load();
    } on AcademicApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('No se pudo renombrar: ${e.message}'),
          backgroundColor: AppColors.border,
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo renombrar: error inesperado'),
          backgroundColor: AppColors.border,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return Container(
        height: 160,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
        ),
        child: const CircularProgressIndicator(color: AppColors.text),
      );
    }
    if (_error != null) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              _error!,
              style: const TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 13,
                color: AppColors.muted,
              ),
            ),
            const SizedBox(height: 12),
            NeobrutalistButton(
              label: 'Reintentar',
              icon: Icons.refresh_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: _load,
            ),
          ],
        ),
      );
    }
    final hasOfficial = _average != null;
    final officialValue = hasOfficial ? _average!.toStringAsFixed(1) : null;

    return MaxWidthContainer(
      padding: EdgeInsets.zero,
      maxWidth: 1100,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // A. Encabezado del curso: nombre + promedio oficial guardado.
          NeobrutalistCard(
            title: widget.subjectName.isEmpty ? 'Curso' : widget.subjectName,
            trailing: NeobrutalistBadge(
              label: hasOfficial ? 'Oficial $officialValue' : 'Sin promedio',
              tone: hasOfficial
                  ? NeobrutalistTone.accent
                  : NeobrutalistTone.pending,
              icon: Icons.fact_check_rounded,
            ),
            child: Text(
              hasOfficial
                  ? 'Promedio oficial guardado${_averageStatusLabel != null ? ' · $_averageStatusLabel' : ''}. Se calcula con las evaluaciones registradas en el backend.'
                  : 'Aún no hay promedio oficial: registra evaluaciones en el curso para verlo.',
              style: const TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 12.5,
                color: AppColors.mutedStrong,
                height: 1.35,
              ),
            ),
          ),
          const SizedBox(height: AppDimens.spaceXl),

          // B. Evaluaciones registradas (datos reales del backend).
          _sectionHeader('EVALUACIONES REGISTRADAS', '${_nodes.length}'),
          const SizedBox(height: AppDimens.spaceMd),
          if (_nodes.isEmpty)
            _emptyGrades()
          else
            for (final node in _nodes) _gradeNode(node),

          const SizedBox(height: AppDimens.spaceXl),

          // C. Calculadora / simulador (separado de lo oficial).
          _sectionHeader('CALCULADORA · SIMULADOR'),
          const SizedBox(height: AppDimens.spaceSm),
          const Text(
            'Simulación local: no modifica ni guarda evaluaciones oficiales.',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 12,
              color: AppColors.mutedStrong,
            ),
          ),
          const SizedBox(height: AppDimens.spaceMd),
          GradeCalculatorScreen(
            embedded: true,
            initialCourse: widget.subjectName.isEmpty
                ? null
                : widget.subjectName,
            onSaved: () => _load(silent: true),
          ),
        ],
      ),
    );
  }

  /// Encabezado de sección: label UPPERCASE + contador opcional + divider.
  Widget _sectionHeader(String label, [String? count]) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  fontWeight: FontWeight.w900,
                  fontSize: 13,
                  letterSpacing: 0.6,
                  color: AppColors.text,
                ),
              ),
            ),
            if (count != null)
              NeobrutalistBadge(label: count, tone: NeobrutalistTone.neutral),
          ],
        ),
        const SizedBox(height: AppDimens.spaceSm),
        const Divider(
          color: AppColors.border,
          thickness: AppDimens.borderWidth,
          height: 1,
        ),
      ],
    );
  }

  Widget _emptyGrades() {
    return Container(
      padding: const EdgeInsets.all(AppDimens.spaceXl),
      decoration: BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: const Column(
        children: [
          Icon(Icons.assignment_outlined, size: 32, color: AppColors.muted),
          SizedBox(height: AppDimens.spaceSm),
          Text(
            'AÚN NO HAY EVALUACIONES REGISTRADAS',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 12,
              letterSpacing: 0.4,
              color: AppColors.text,
            ),
          ),
          SizedBox(height: AppDimens.spaceXs),
          Text(
            'Cuando el curso tenga notas en el backend, aparecerán aquí.',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 12,
              color: AppColors.mutedStrong,
            ),
          ),
        ],
      ),
    );
  }

  /// Top + hijas con sangría, cada una con su rename (hijas incluidas).
  Widget _gradeNode(AcademicGradeNode node) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _gradeRow(node.grade),
        for (final child in node.children)
          Padding(
            padding: const EdgeInsets.only(left: 24),
            child: _gradeRow(child, prefix: '↳ '),
          ),
      ],
    );
  }

  /// Etiqueta comprensible para los estados que entrega el backend.
  String? get _averageStatusLabel {
    final raw = _averageStatus?.trim();
    if (raw == null || raw.isEmpty) return null;
    return switch (raw.toLowerCase()) {
      'approved' || 'aprobado' => 'Aprobado',
      'in_progress' || 'en curso' || 'cursando' => 'En curso',
      'pending' || 'pendiente' => 'Pendiente',
      'failed' || 'reprobado' => 'Reprobado',
      _ => raw,
    };
  }

  Widget _gradeRow(AcademicGrade grade, {String prefix = ''}) {
    final hasScore = grade.score != null;
    final score = hasScore ? (grade.score! / 10.0).toStringAsFixed(1) : '–';
    final weightPct = '${(grade.weight * 100).toStringAsFixed(0)}%';

    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: NeobrutalistCard(
        padding: const EdgeInsets.all(AppDimens.spaceMd),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    '$prefix${grade.name}',
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontWeight: FontWeight.w800,
                      fontSize: 13.5,
                      color: AppColors.text,
                    ),
                  ),
                  const SizedBox(height: AppDimens.spaceXs),
                  Wrap(
                    spacing: AppDimens.spaceSm,
                    runSpacing: AppDimens.spaceXs,
                    children: [
                      NeobrutalistBadge(
                        label: 'Ponderación $weightPct',
                        tone: NeobrutalistTone.neutral,
                      ),
                      NeobrutalistBadge(
                        label: hasScore ? 'Registrada' : 'Pendiente',
                        tone: hasScore
                            ? NeobrutalistTone.success
                            : NeobrutalistTone.pending,
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(width: AppDimens.spaceSm),
            Text(
              score,
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 26,
                color: hasScore ? AppColors.text : AppColors.muted,
              ),
            ),
            const SizedBox(width: AppDimens.spaceXs),
            NeobrutalistIconButton(
              icon: Icons.edit_rounded,
              tooltip: 'Renombrar',
              onPressed: () => _rename(grade),
            ),
          ],
        ),
      ),
    );
  }
}

/// Opción de renombrado aplanada: tops + hijas con nombre compuesto.
class RenameOption {
  const RenameOption({required this.id, required this.label});

  final String id;
  final String label;
}

/// Tops + hijas (sangría ›) para el menú de rename. Incluye todo lo
/// renombrable; renombrar hija persiste igual que tops.
List<RenameOption> flattenRenameOptions(List<AcademicGradeNode> nodes) {
  final out = <RenameOption>[];
  for (final n in nodes) {
    out.add(RenameOption(id: n.grade.id, label: n.grade.name));
    for (final c in n.children) {
      out.add(RenameOption(id: c.id, label: '${n.grade.name} › ${c.name}'));
    }
  }
  return out;
}
