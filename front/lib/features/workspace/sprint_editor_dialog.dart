import 'package:flutter/material.dart';
import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../../core/models/social_models.dart';

class SprintDraft {
  const SprintDraft(this.name, this.start, this.end);
  final String name, start, end;
}

class SprintEditorDialog extends StatefulWidget {
  const SprintEditorDialog({
    super.key,
    this.sheet,
    required this.suggestedName,
  });
  final SprintSheetInfo? sheet;
  final String suggestedName;
  @override
  State<SprintEditorDialog> createState() => _SprintEditorDialogState();
}

class _SprintEditorDialogState extends State<SprintEditorDialog> {
  late final TextEditingController _name;
  late DateTime _start, _end;
  String? _error;
  @override
  void initState() {
    super.initState();
    _name = TextEditingController(
      text: widget.sheet?.name ?? widget.suggestedName,
    );
    _start =
        DateTime.tryParse(widget.sheet?.periodStart ?? '') ?? DateTime.now();
    _end =
        DateTime.tryParse(widget.sheet?.periodEnd ?? '') ??
        _start.add(const Duration(days: 4));
  }

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  String iso(DateTime d) =>
      '${d.year.toString().padLeft(4, '0')}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';
  Future<void> _pick(bool start) async {
    final selected = await showNeobrutalistDatePicker(
      context: context,
      initialDate: start ? _start : _end,
      firstDate: DateTime(1900),
      lastDate: DateTime(2200),
    );
    if (selected == null || !mounted) return;
    setState(() {
      if (start) {
        _start = selected;
      } else {
        _end = selected;
      }
    });
  }

  void _submit() {
    if (_name.text.trim().isEmpty || iso(_end).compareTo(iso(_start)) < 0) {
      setState(
        () => _error =
            'Escribe un nombre y una fecha fin igual o posterior al inicio.',
      );
      return;
    }
    Navigator.pop(
      context,
      SprintDraft(_name.text.trim(), iso(_start), iso(_end)),
    );
  }

  @override
  Widget build(BuildContext context) => NeobrutalistDialog(
    title: widget.sheet == null ? 'NUEVO SPRINT' : 'EDITAR SPRINT',
    confirmLabel: widget.sheet == null ? 'CREAR SPRINT' : 'GUARDAR CAMBIOS',
    cancelLabel: 'CANCELAR',
    closeOnConfirm: false,
    onConfirm: _submit,
    content: SingleChildScrollView(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const AppFieldLabel('Nombre del Sprint'),
          const SizedBox(height: 4),
          TextField(
            controller: _name,
            decoration: appInputDecoration('Nombre del Sprint'),
          ),
          const SizedBox(height: AppDimens.spaceMd),
          // Fechas: cajas de campo neobrutalistas (antes TextButton Material,
          // que heredaba el morado del colorScheme por defecto).
          const AppFieldLabel('FECHA INICIO'),
          const SizedBox(height: 4),
          NeobrutalistFieldBox(
            key: const Key('sprint-start-date'),
            text: iso(_start),
            icon: Icons.event_rounded,
            trailingIcon: Icons.edit_calendar_rounded,
            semanticLabel: 'Fecha inicio ${iso(_start)}',
            onTap: () => _pick(true),
          ),
          const SizedBox(height: AppDimens.spaceMd),
          const AppFieldLabel('FECHA FIN'),
          const SizedBox(height: 4),
          NeobrutalistFieldBox(
            key: const Key('sprint-end-date'),
            text: iso(_end),
            icon: Icons.event_available_rounded,
            trailingIcon: Icons.edit_calendar_rounded,
            semanticLabel: 'Fecha fin ${iso(_end)}',
            onTap: () => _pick(false),
          ),
          if (widget.sheet != null) ...[
            const SizedBox(height: AppDimens.spaceMd),
            const Text(
              'Cambiar el rango no elimina las horas registradas.',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
          if (_error != null) ...[
            const SizedBox(height: AppDimens.spaceSm),
            Text(
              _error!,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w800,
                color: AppColors.errorDeep,
              ),
            ),
          ],
        ],
      ),
    ),
  );
}
