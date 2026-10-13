import 'package:flutter/material.dart';

import '../../core/database/app_database.dart';
import '../../core/database/local_notes_repository.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../notes/all_notes_screen.dart';
import 'academic_mappers.dart';
import 'course_detail_tabs.dart';

/// Ventanas separadas del curso (UX-05b): AppBar + back, mismo patrón que
/// "Ver Asistencia" (push + regresar al curso, nunca al home).

AppBar _windowAppBar(String title) {
  return AppBar(
    backgroundColor: AppColors.surface,
    foregroundColor: AppColors.text,
    leading: const BackButton(),
    title: Text(
      title,
      style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 15),
    ),
  );
}

/// Ventana Unidades: checklist con check persistente (done en backend).
class UnitsWindow extends StatelessWidget {
  const UnitsWindow({
    super.key,
    required this.subjectId,
    required this.subjectName,
  });

  final String subjectId;
  final String subjectName;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      appBar: _windowAppBar('UNIDADES · ${subjectName.toUpperCase()}'),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: ContentsTab(subjectId: subjectId),
        ),
      ),
    );
  }
}

/// Entrada de apunte filtrado por materia (convención "[CODE] ").
class _SubjectNote {
  _SubjectNote({
    required this.id,
    required this.title,
    required this.content,
    required this.visibility,
    required this.updatedAt,
  });

  final String id;
  final String title;
  final String content;
  final String visibility;
  final DateTime updatedAt;
}

/// Ventana Materiales: crear apunte (preset materia) + lista filtrada con
/// formato de tarjeta de nota (solo lectura + tap abrir).
class MaterialsWindow extends StatefulWidget {
  const MaterialsWindow({
    super.key,
    required this.subjectCode,
    required this.subjectName,
  });

  final String subjectCode;
  final String subjectName;

  @override
  State<MaterialsWindow> createState() => _MaterialsWindowState();
}

class _MaterialsWindowState extends State<MaterialsWindow> {
  bool _isLoading = true;
  String? _error;
  List<_SubjectNote> _notes = const [];

  @override
  void initState() {    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (mounted) {
      setState(() {
        _isLoading = true;
        _error = null;
      });
    }
    try {
      final db = AppDatabase();
      final repo = LocalNotesRepository(db);
      final owner = SessionManager.currentUserId;
      final all = await repo.getAllNotes(ownerId: owner);
      final filtered = all
          .where((n) => noteMatchesSubjectCode(n.title, widget.subjectCode))
          .map(
            (n) => _SubjectNote(
              id: n.id,
              title: n.title,
              content: n.content,
              visibility: n.visibility,
              updatedAt: n.updatedAt,
            ),
          )
          .toList()
        ..sort((a, b) => b.updatedAt.compareTo(a.updatedAt));
      if (!mounted) return;
      setState(() {
        _notes = filtered;
        _isLoading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _error = 'No se pudieron cargar los apuntes: $e';
      });
    }
  }

  Future<void> _openCreate() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => AllNotesScreen(
          initialSubjectCode: widget.subjectCode,
          autoOpenCreate: true,
        ),
      ),
    );
    if (mounted) await _load();
  }

  void _openNote() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            AllNotesScreen(initialSubjectCode: widget.subjectCode),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      appBar: _windowAppBar('MATERIALES · ${widget.subjectName.toUpperCase()}'),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              NeobrutalistButton(
                label: 'Crear apunte',
                icon: Icons.add_rounded,
                variant: NeobrutalistButtonVariant.accent,
                expand: true,
                onPressed: _openCreate,
              ),
              const SizedBox(height: 12),
              if (_isLoading)
                Container(
                  height: 120,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
                  child: const CircularProgressIndicator(color: AppColors.text),
                )
              else if (_error != null)
                Container(
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
                )
              else if (_notes.isEmpty)
                Container(
                  padding: const EdgeInsets.all(20),
                  alignment: Alignment.center,
                  decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius)),
                  child: const Text(
                    'SIN APUNTES DE ESTA MATERIA: CREA EL PRIMERO',
                    textAlign: TextAlign.center,
                    style: TextStyle(fontWeight: FontWeight.w800, fontSize: 12, color: AppColors.muted),
                  ),
                )
              else
                for (final n in _notes) _noteCard(n),
            ],
          ),
        ),
      ),
    );
  }

  /// Tarjeta con el formato visual de nota (lista): badges + título +
  /// extracto. Solo lectura; tap abre la materia filtrada.
  Widget _noteCard(_SubjectNote n) {
    final snippet = n.content.trim().replaceAll(RegExp(r'\s+'), ' ');
    return InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: _openNote,
      child: Container(
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)],
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(4)),
                  child: Text(
                    n.visibility.toUpperCase(),
                    style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 10, color: AppColors.text),
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  '${n.updatedAt.day.toString().padLeft(2, '0')}/${n.updatedAt.month.toString().padLeft(2, '0')}/${n.updatedAt.year}',
                  style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 11, color: AppColors.muted),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              n.title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text, height: 1.2),
            ),
            const SizedBox(height: 4),
            Text(
              snippet,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w500, color: AppColors.muted, height: 1.3),
            ),
          ],
        ),
      ),
    );
  }
}

/// Ventana Evaluaciones: promedio + nota fija + notas (pin/renombrar
/// reutilizados) + entrada a la calculadora (simulación local).
class EvaluationsWindow extends StatelessWidget {
  const EvaluationsWindow({
    super.key,
    required this.subjectId,
    required this.subjectName,
  });

  final String subjectId;
  final String subjectName;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      appBar: _windowAppBar('EVALUACIONES · ${subjectName.toUpperCase()}'),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: EvaluationsTab(subjectId: subjectId),
        ),
      ),
    );
  }
}
