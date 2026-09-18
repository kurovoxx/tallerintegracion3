import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/common_widgets.dart';

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
        data: Theme.of(context).copyWith(colorScheme: const ColorScheme.light(primary: AppColors.border)),
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
        data: Theme.of(context).copyWith(colorScheme: const ColorScheme.light(primary: AppColors.border)),
        child: child!,
      ),
    );
    if (t != null) setState(() => _selectedTime = t);
  }

  void _submit() {
    if (!_formKey.currentState!.validate()) return;
    if (_selectedMembers.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Selecciona al menos un miembro'), backgroundColor: AppColors.error));
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('Reunión "${_titleCtrl.text.trim()}" agendada para ${_selectedDate.day}/${_selectedDate.month} a las ${_selectedTime.format(context)}'),
        backgroundColor: AppColors.border,
      ),
    );
    Navigator.of(context).maybePop();
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: EdgeInsets.symmetric(horizontal: isDesktop ? 32 : 16, vertical: 16),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 640),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text('AGENDAR REUNIÓN', style: TextStyle(fontSize: isDesktop ? 22 : 18, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: -0.5)),
                  const SizedBox(height: 4),
                  const Text('Coordina con tu grupo y sincroniza con Google Calendar', style: TextStyle(fontSize: 12.5, fontWeight: FontWeight.w600, color: AppColors.muted)),
                  const SizedBox(height: 18),
                  Container(
                    padding: const EdgeInsets.all(20),
                    decoration: BoxDecoration(
                      color: AppColors.surface,
                      border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
                      borderRadius: BorderRadius.circular(AppDimens.radius),
                      boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
                    ),
                    child: Form(
                      key: _formKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          const AppFieldLabel('TÍTULO DE LA REUNIÓN'),
                          const SizedBox(height: 6),
                          TextFormField(
                            controller: _titleCtrl,
                            decoration: appInputDecoration('Ej. Repaso Cálculo II'),
                            validator: (v) => (v == null || v.trim().isEmpty) ? 'Requerido' : null,
                            style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                          ),
                          const SizedBox(height: 14),
                          const AppFieldLabel('DESCRIPCIÓN (OPCIONAL)'),
                          const SizedBox(height: 6),
                          TextFormField(
                            controller: _descCtrl,
                            maxLines: 3,
                            decoration: appInputDecoration('Agenda, temas a revisar...'),
                            style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13, color: AppColors.text),
                          ),
                          const SizedBox(height: 14),
                          Row(
                            children: [
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    const AppFieldLabel('FECHA'),
                                    const SizedBox(height: 6),
                                    InkWell(
                                      onTap: _pickDate,
                                      borderRadius: BorderRadius.circular(6),
                                      child: Container(
                                        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                                        decoration: BoxDecoration(
                                          color: AppColors.bg,
                                          border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
                                          borderRadius: BorderRadius.circular(6),
                                        ),
                                        child: Row(
                                          children: [
                                            const Icon(Icons.calendar_today_rounded, size: 16, color: AppColors.text),
                                            const SizedBox(width: 8),
                                            Text('${_selectedDate.day}/${_selectedDate.month}/${_selectedDate.year}', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text)),
                                          ],
                                        ),
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    const AppFieldLabel('HORA'),
                                    const SizedBox(height: 6),
                                    InkWell(
                                      onTap: _pickTime,
                                      borderRadius: BorderRadius.circular(6),
                                      child: Container(
                                        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                                        decoration: BoxDecoration(
                                          color: AppColors.bg,
                                          border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
                                          borderRadius: BorderRadius.circular(6),
                                        ),
                                        child: Row(
                                          children: [
                                            const Icon(Icons.access_time_rounded, size: 16, color: AppColors.text),
                                            const SizedBox(width: 8),
                                            Text(_selectedTime.format(context), style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text)),
                                          ],
                                        ),
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 14),
                          const AppFieldLabel('ENLACE / SALA'),
                          const SizedBox(height: 6),
                          TextFormField(
                            controller: _linkCtrl,
                            decoration: appInputDecoration('https://meet.google.com/...'),
                            style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13, color: AppColors.text),
                          ),
                          const SizedBox(height: 14),
                          const AppFieldLabel('MIEMBROS INVITADOS'),
                          const SizedBox(height: 8),
                          Wrap(
                            spacing: 8,
                            runSpacing: 8,
                            children: _members.map((m) {
                              final selected = _selectedMembers.contains(m);
                              return FilterChip(
                                label: Text(m, style: TextStyle(fontWeight: FontWeight.w800, fontSize: 11, color: selected ? Colors.white : AppColors.text)),
                                selected: selected,
                                selectedColor: AppColors.border,
                                backgroundColor: AppColors.bg,
                                checkmarkColor: Colors.white,
                                side: const BorderSide(color: AppColors.border, width: 1.5),
                                onSelected: (v) => setState(() {
                                  if (v) {
                                    _selectedMembers.add(m);
                                  } else {
                                    _selectedMembers.remove(m);
                                  }
                                }),
                              );
                            }).toList(),
                          ),
                          const SizedBox(height: 20),
                          SubmitButton(text: 'AGENDAR REUNIÓN', onPressed: _submit),
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
}
