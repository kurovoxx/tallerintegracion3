import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
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

/// Ventana Evaluaciones: promedio + notas (renombrar reutilizado) +
/// calculadora embebida (simulación local).
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
          child: EvaluationsTab(subjectId: subjectId, subjectName: subjectName),
        ),
      ),
    );
  }
}
