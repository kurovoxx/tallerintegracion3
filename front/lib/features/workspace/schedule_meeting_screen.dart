// Ruta: front/lib/features/workspace/schedule_meeting_screen.dart
//
// Formulario "Agendar reunión": ficha neobrutalista retro con tarjeta de
// borde 2.5 px y sombra hard4, campos de tinta, selectores de fecha/hora
// tipo sticker y chips invertidos para los integrantes. Consume los tokens
// canónicos de core/theme/app_theme.dart y core/widgets/neobrutalism.dart.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

class ScheduleMeetingScreen extends StatefulWidget {
  const ScheduleMeetingScreen({super.key, this.groupId, SocialService? service})
      : _serviceOverride = service;

  final String? groupId;
  final SocialService? _serviceOverride;

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

  late final SocialService _social;
  bool _isSubmitting = false;
  String? _createdMeetingId;
  String? _submitError;

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? SocialService();
  }

  @override
  void dispose() {
    _titleCtrl.dispose();
    _linkCtrl.dispose();
    _descCtrl.dispose();
    if (widget._serviceOverride == null) _social.dispose();
    super.dispose();
  }

  bool get _isRealGroup {
    final id = widget.groupId?.trim() ?? '';
    final uuid = RegExp(
        r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$');
    return id.isNotEmpty && uuid.hasMatch(id);
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

  // POST real: POST /groups/:id/meetings
  // {title, description?, scheduled_at RFC3339} -> 201 {meeting_id}.
  // Ver back/social/cmd/server/main.go:198 y meeting_handler.go:32.
  // No existe GET /groups/:id/meetings: no se inventa listado.
  // No se envían reuniones de prueba sin autorización: solo envía al pulsar.
  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    if (_selectedMembers.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Selecciona al menos un miembro (local)'),
          backgroundColor: AppColors.error,
        ),
      );
      return;
    }
    if (!_isRealGroup) {
      setState(() => _submitError =
          'Sin grupo real (UUID): selecciona un grupo para POST /groups/:id/meetings. No se envió nada.');
      return;
    }
    setState(() {
      _isSubmitting = true;
      _submitError = null;
      _createdMeetingId = null;
    });
    try {
      final when = DateTime(
        _selectedDate.year,
        _selectedDate.month,
        _selectedDate.day,
        _selectedTime.hour,
        _selectedTime.minute,
      ).toUtc();
      final desc = _descCtrl.text.trim();
      final id = await _social.createMeeting(
        groupId: widget.groupId!.trim(),
        title: _titleCtrl.text.trim(),
        description: desc.isEmpty ? null : desc,
        scheduledAtUtc: when,
      );
      if (!mounted) return;
      setState(() => _createdMeetingId = id);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('Reunión creada (real): $id'),
          backgroundColor: AppColors.border,
        ),
      );
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() => _submitError = 'No se pudo crear: $e');
    } catch (e) {
      if (!mounted) return;
      setState(() => _submitError = 'No se pudo crear: $e');
    } finally {
      if (mounted) setState(() => _isSubmitting = false);
    }
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
                          const AppFieldLabel('ENLACE / SALA (LOCAL, NO SE ENVÍA)'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _linkCtrl,
                              decoration: appInputDecoration(
                                'https://meet.google.com/... (solo nota local)',
                              ),
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: 4),
                          const Text(
                            'El contrato POST /groups/:id/meetings solo acepta title, description y scheduled_at. El enlace no se envía.',
                            style: TextStyle(
                              fontSize: 10.5,
                              fontWeight: FontWeight.w600,
                              color: AppColors.muted,
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('GOOGLE CALENDAR (BACKEND)'),
                          const SizedBox(height: AppDimens.spaceSm),
                          Container(
                            padding: const EdgeInsets.all(AppDimens.spaceMd),
                            decoration: BoxDecoration(
                              color: AppColors.bg,
                              border: Border.all(
                                color: AppColors.border,
                                width: AppDimens.borderWidth,
                              ),
                              borderRadius: BorderRadius.circular(
                                AppDimens.radius,
                              ),
                            ),
                            child: const Text(
                              'Sin vinculación simulada aquí. Tras el POST real, el backend sincroniza con Calendar/Discord/Stream en background (best-effort).',
                              style: TextStyle(
                                fontSize: 11.5,
                                fontWeight: FontWeight.w600,
                                color: AppColors.mutedStrong,
                                height: 1.35,
                              ),
                            ),
                          ),
                          if (_createdMeetingId != null) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Container(
                              padding: const EdgeInsets.all(AppDimens.spaceMd),
                              decoration: BoxDecoration(
                                color: const Color(0xFFE7F6E7),
                                border: Border.all(
                                  color: AppColors.border,
                                  width: AppDimens.borderWidth,
                                ),
                                borderRadius: BorderRadius.circular(
                                  AppDimens.radius,
                                ),
                              ),
                              child: Text(
                                'Reunión creada (real): $_createdMeetingId',
                                style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w800,
                                  color: AppColors.text,
                                ),
                              ),
                            ),
                          ],
                          if (_submitError != null) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Container(
                              padding: const EdgeInsets.all(AppDimens.spaceMd),
                              decoration: BoxDecoration(
                                color: const Color(0xFFFDE8E8),
                                border: Border.all(
                                  color: AppColors.border,
                                  width: AppDimens.borderWidth,
                                ),
                                borderRadius: BorderRadius.circular(
                                  AppDimens.radius,
                                ),
                              ),
                              child: Text(
                                _submitError!,
                                style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w700,
                                  color: AppColors.text,
                                ),
                              ),
                            ),
                          ],
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('MIEMBROS INVITADOS (LOCAL)'),
                          const SizedBox(height: 4),
                          const Text(
                            'Selección local: el contrato no recibe miembros.',
                            style: TextStyle(
                              fontSize: 10.5,
                              fontWeight: FontWeight.w600,
                              color: AppColors.muted,
                            ),
                          ),
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
                          if (!_isRealGroup) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Container(
                              padding: const EdgeInsets.all(10),
                              decoration: BoxDecoration(
                                color: AppColors.bg,
                                border: Border.all(
                                  color: AppColors.border,
                                  width: AppDimens.borderWidth,
                                ),
                                borderRadius: BorderRadius.circular(
                                  AppDimens.radius,
                                ),
                              ),
                              child: const Text(
                                'Sin grupo real: el botón no envía nada. Abre esta vista desde un grupo real para POST /groups/:id/meetings.',
                                style: TextStyle(
                                  fontSize: 11.5,
                                  fontWeight: FontWeight.w700,
                                  color: AppColors.mutedStrong,
                                ),
                              ),
                            ),
                          ],
                          const SizedBox(height: AppDimens.spaceXl),
                          NeobrutalistButton(
                            label: _isSubmitting
                                ? 'Enviando POST real...'
                                : 'Agendar reunión (POST real)',
                            icon: Icons.calendar_month_rounded,
                            variant: NeobrutalistButtonVariant.accent,
                            expand: true,
                            onPressed: _isSubmitting ? null : _submit,
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
          'POST real /groups/:id/meetings. No hay GET de reuniones: no se lista aquí.',
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
