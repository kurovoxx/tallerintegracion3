// Ruta: front/lib/features/workspace/schedule_meeting_screen.dart
//
// Formulario "Agendar reunión": ficha neobrutalista retro con tarjeta de
// borde 2.5 px y sombra hard4, campos de tinta, selectores de fecha/hora
// tipo sticker y chips invertidos para los integrantes. Consume los tokens
// canónicos de core/theme/app_theme.dart y core/widgets/neobrutalism.dart.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

class ScheduleMeetingScreen extends StatefulWidget {
  const ScheduleMeetingScreen({super.key});

  @override
  State<ScheduleMeetingScreen> createState() => _ScheduleMeetingScreenState();
}

class _ScheduleMeetingScreenState extends State<ScheduleMeetingScreen> {
  final _formKey = GlobalKey<FormState>();
  final _titleCtrl = TextEditingController();
  final _linkCtrl = TextEditingController(text: 'https://meet.google.com/');
  final _descCtrl = TextEditingController();
  DateTime _selectedDate = DateTime.now().add(const Duration(days: 1));
  TimeOfDay _selectedTime = const TimeOfDay(hour: 10, minute: 0);
  final List<String> _members = ['Sofía', 'Matías', 'Ana', 'Tú'];
  final Set<String> _selectedMembers = {'Sofía', 'Tú'};

  @override
  void dispose() {
    _titleCtrl.dispose();
    _linkCtrl.dispose();
    _descCtrl.dispose();
    super.dispose();
  }

  Future<void> _pickDate() async {
    final d = await showDatePicker(
      context: context,
      initialDate: _selectedDate,
      firstDate: DateTime.now(),
      lastDate: DateTime.now().add(const Duration(days: 365)),
      builder: (context, child) => Theme(
        data: Theme.of(context).copyWith(
          colorScheme: const ColorScheme.light(primary: AppColors.border),
        ),
        child: child!,
      ),
    );
    if (d != null) setState(() => _selectedDate = d);
  }

  Future<void> _pickTime() async {
    final t = await showTimePicker(
      context: context,
      initialTime: _selectedTime,
      builder: (context, child) => Theme(
        data: Theme.of(context).copyWith(
          colorScheme: const ColorScheme.light(primary: AppColors.border),
        ),
        child: child!,
      ),
    );
    if (t != null) setState(() => _selectedTime = t);
  }

  void _submit() {
    if (!_formKey.currentState!.validate()) return;
    if (_selectedMembers.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Selecciona al menos un miembro'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          'Reunión "${_titleCtrl.text.trim()}" agendada para '
          '${_selectedDate.day}/${_selectedDate.month} a las '
          '${_selectedTime.format(context)}',
        ),
        backgroundColor: AppColors.border,
      ),
    );
    Navigator.of(context).maybePop();
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: SingleChildScrollView(
          physics: const BouncingScrollPhysics(),
          padding: EdgeInsets.symmetric(
            horizontal: isDesktop ? AppDimens.spaceXxl : AppDimens.spaceLg,
            vertical: AppDimens.spaceLg,
          ),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 640),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildHeader(isDesktop),
                  const SizedBox(height: AppDimens.spaceLg),
                  Container(
                    padding: const EdgeInsets.all(AppDimens.spaceXl),
                    decoration: BoxDecoration(
                      color: AppColors.surface,
                      border: Border.all(
                        color: AppColors.border,
                        width: AppDimens.borderWidthAction,
                      ),
                      borderRadius: BorderRadius.circular(AppDimens.radius),
                      boxShadow: AppShadows.card,
                    ),
                    child: Form(
                      key: _formKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          const AppFieldLabel('TÍTULO DE LA REUNIÓN'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _titleCtrl,
                              decoration: appInputDecoration(
                                'Ej. Repaso Cálculo II',
                              ),
                              validator: (v) =>
                                  (v == null || v.trim().isEmpty)
                                  ? 'Requerido'
                                  : null,
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('DESCRIPCIÓN (OPCIONAL)'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _descCtrl,
                              maxLines: 3,
                              decoration: appInputDecoration(
                                'Agenda, temas a revisar...',
                              ),
                              style: const TextStyle(
                                fontWeight: FontWeight.w600,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Expanded(
                                child: _buildPickerBox(
                                  label: 'FECHA',
                                  icon: Icons.calendar_month_rounded,
                                  value:
                                      '${_selectedDate.day}/${_selectedDate.month}/${_selectedDate.year}',
                                  onTap: _pickDate,
                                ),
                              ),
                              const SizedBox(width: AppDimens.spaceMd),
                              Expanded(
                                child: _buildPickerBox(
                                  label: 'HORA',
                                  icon: Icons.access_time_rounded,
                                  value: _selectedTime.format(context),
                                  onTap: _pickTime,
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('ENLACE / SALA'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _linkCtrl,
                              decoration: appInputDecoration(
                                'https://meet.google.com/...',
                              ),
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('MIEMBROS INVITADOS'),
                          const SizedBox(height: AppDimens.spaceSm),
                          Wrap(
                            spacing: AppDimens.spaceSm,
                            runSpacing: AppDimens.spaceSm,
                            children: [
                              for (final member in _members)
                                _MemberStickerChip(
                                  label: member,
                                  selected: _selectedMembers.contains(member),
                                  onTap: () {
                                    setState(() {
                                      if (_selectedMembers.contains(member)) {
                                        _selectedMembers.remove(member);
                                      } else {
                                        _selectedMembers.add(member);
                                      }
                                    });
                                  },
                                ),
                            ],
                          ),
                          const SizedBox(height: AppDimens.spaceXl),
                          NeobrutalistButton(
                            label: 'Agendar reunión',
                            icon: Icons.calendar_month_rounded,
                            variant: NeobrutalistButtonVariant.accent,
                            expand: true,
                            onPressed: _submit,
                          ),
                        ],
                      ),
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

  Widget _buildHeader(bool isDesktop) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'AGENDAR REUNIÓN',
          style: TextStyle(
            fontSize: isDesktop ? 24 : 20,
            fontWeight: FontWeight.w900,
            color: AppColors.text,
            letterSpacing: -0.5,
          ),
        ),
        const SizedBox(height: AppDimens.spaceXs),
        const Text(
          'Coordina con tu grupo y sincroniza con Google Calendar',
          style: TextStyle(
            fontSize: 12.5,
            fontWeight: FontWeight.w700,
            color: AppColors.mutedStrong,
          ),
        ),
      ],
    );
  }

  Widget _buildPickerBox({
    required String label,
    required IconData icon,
    required String value,
    required VoidCallback onTap,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        AppFieldLabel(label),
        const SizedBox(height: AppDimens.spaceSm),
        ClickCursor(
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: onTap,
            child: Container(
              padding: const EdgeInsets.symmetric(
                horizontal: AppDimens.spaceMd,
                vertical: 13,
              ),
              decoration: BoxDecoration(
                color: AppColors.bg,
                border: Border.all(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
                borderRadius: BorderRadius.circular(AppDimens.radius),
                boxShadow: AppShadows.badge,
              ),
              child: Row(
                children: [
                  Icon(icon, size: 17, color: AppColors.text),
                  const SizedBox(width: AppDimens.spaceSm),
                  Expanded(
                    child: Text(
                      value,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w900,
                        fontSize: 13,
                        color: AppColors.text,
                      ),
                    ),
                  ),
                  const Icon(
                    Icons.expand_more_rounded,
                    size: 16,
                    color: AppColors.text,
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}

/// Sombra dura bajo los campos de texto: borde de tinta sobre papel.
class _FieldShell extends StatelessWidget {
  const _FieldShell({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(boxShadow: AppShadows.badge),
      child: child,
    );
  }
}

/// Chip sticker de integrante: borde de 2 px, sombra dura cuando está libre y
/// fondo resaltador (invertido) al seleccionarse.
class _MemberStickerChip extends StatefulWidget {
  const _MemberStickerChip({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  State<_MemberStickerChip> createState() => _MemberStickerChipState();
}

class _MemberStickerChipState extends State<_MemberStickerChip> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final pressed = _pressed;
    final selected = widget.selected;
    return Semantics(
      button: true,
      selected: selected,
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: (_) => setState(() => _pressed = true),
          onTapUp: (_) => setState(() => _pressed = false),
          onTapCancel: () => setState(() => _pressed = false),
          onTap: widget.onTap,
          child: AnimatedContainer(
            duration: AppMotion.press,
            curve: AppMotion.standard,
            transform: Matrix4.translationValues(
              pressed ? AppShadows.offsetBadge.dx : 0,
              pressed ? AppShadows.offsetBadge.dy : 0,
              0,
            ),
            padding: const EdgeInsets.symmetric(
              horizontal: AppDimens.spaceMd,
              vertical: 7,
            ),
            decoration: BoxDecoration(
              color: selected ? AppColors.accentYellow : AppColors.surface,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              boxShadow: selected || pressed
                  ? const <BoxShadow>[]
                  : AppShadows.badge,
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  selected
                      ? Icons.check_box_rounded
                      : Icons.check_box_outline_blank_rounded,
                  size: 13,
                  color: AppColors.text,
                ),
                const SizedBox(width: AppDimens.spaceXs),
                Text(
                  widget.label.toUpperCase(),
                  style: const TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.6,
                    color: AppColors.text,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
