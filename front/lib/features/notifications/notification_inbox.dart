import 'package:flutter/material.dart';

import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Diálogo de bandeja de entrada de notificaciones.
/// Muestra título, descripción, hora, nombre del grupo y enlace (si lo tiene).
/// Cada fila tiene botón de eliminar y se puede marcar como leída tocando.
class NotificationInbox extends StatefulWidget {
  const NotificationInbox({super.key, SocialService? service})
    : _serviceOverride = service;

  final SocialService? _serviceOverride;

  @override
  State<NotificationInbox> createState() => _NotificationInboxState();
}

class _NotificationInboxState extends State<NotificationInbox> {
  late final SocialService _service;

  List<MeetingNotification> _items = const [];
  bool _loading = true;
  String? _error;
  // Ids con DELETE en vuelo: un doble toque no repite la petición.
  final Set<String> _deleting = <String>{};

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? SocialService();
    _load();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final items = await _service.myNotifications();
      if (!mounted) return;
      setState(() {
        _items = items;
        _loading = false;
      });
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudieron cargar las notificaciones.';
        _loading = false;
      });
    }
  }

  Future<void> _delete(String id) async {
    if (_deleting.contains(id)) return;
    setState(() => _deleting.add(id));
    try {
      await _service.deleteNotification(id);
      if (!mounted) return;
      setState(() {
        _items = _items.where((n) => n.id != id).toList();
      });
    } on SocialApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(e.message),
          backgroundColor: AppColors.error,
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('No se pudo eliminar la notificación.'),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _deleting.remove(id));
    }
  }

  Future<void> _markRead(MeetingNotification n) async {
    if (n.read) return;
    try {
      await _service.markNotificationRead(n.id);
      if (!mounted) return;
      setState(() {
        _items = [
          for (final item in _items)
            if (item.id == n.id) MeetingNotification(
              id: item.id,
              meetingId: item.meetingId,
              groupId: item.groupId,
              title: item.title,
              scheduledAt: item.scheduledAt,
              read: true,
              readAt: DateTime.now(),
              createdAt: item.createdAt,
              description: item.description,
              link: item.link,
              groupName: item.groupName,
            )
            else item,
        ];
      });
    } catch (_) {
      // Best-effort: la recarga muestra el estado real.
    }
  }

  static String _formatWhen(DateTime when) {
    final local = when.toLocal();
    return '${local.day.toString().padLeft(2, '0')}/${local.month.toString().padLeft(2, '0')}/${local.year} · '
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text(
        'NOTIFICACIONES',
        style: TextStyle(
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
      content: SizedBox(
        width: 420,
        height: 520,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Expanded(
                  child: Text(
                    'Tus notificaciones',
                    style: TextStyle(
                      fontWeight: FontWeight.w800,
                      fontSize: 13,
                      color: AppColors.text,
                    ),
                  ),
                ),
                NeobrutalistIconButton(
                  icon: Icons.close_rounded,
                  tooltip: 'Cerrar',
                  size: 34,
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
            Container(height: 2, color: AppColors.border),
            Expanded(
              child: _buildList(),
            ),
          ],
        ),
      ),
      actions: [
        NeobrutalistButton(
          label: 'Cerrar',
          icon: Icons.close_rounded,
          variant: NeobrutalistButtonVariant.secondary,
          onPressed: () => Navigator.of(context).pop(),
        ),
      ],
    );
  }

  Widget _buildList() {
    if (_loading) {
      return const Center(
        child: CircularProgressIndicator(strokeWidth: 2),
      );
    }
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.cloud_off_rounded, size: 28, color: AppColors.muted),
            const SizedBox(height: AppDimens.spaceSm),
            Text(
              _error!,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
            const SizedBox(height: AppDimens.spaceMd),
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
    if (_items.isEmpty) {
      return const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.notifications_none_rounded, size: 32, color: AppColors.muted),
            SizedBox(height: AppDimens.spaceSm),
            Text(
              'SIN NOTIFICACIONES',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
              ),
            ),
            SizedBox(height: AppDimens.spaceXs),
            Text(
              'Aquí verás los recordatorios de tus reuniones.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
        ),
      );
    }
    return ListView.builder(
      shrinkWrap: true,
      itemCount: _items.length,
      itemBuilder: (context, i) => _tile(_items[i]),
    );
  }

  Widget _tile(MeetingNotification n) {
    final unread = !n.read;
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: Material(
        type: MaterialType.transparency,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            InkWell(
              borderRadius: BorderRadius.circular(AppDimens.radius),
              onTap: () => _markRead(n),
              child: Container(
                padding: const EdgeInsets.all(AppDimens.spaceMd),
                decoration: BoxDecoration(
                  color: unread ? AppColors.surface : AppColors.surfaceLow,
                  border: Border.all(
                    color: AppColors.border,
                    width: AppDimens.borderWidth,
                  ),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: unread ? const [AppShadows.hard2] : null,
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            n.title,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: unread
                                  ? FontWeight.w900
                                  : FontWeight.w700,
                              color: AppColors.text,
                            ),
                          ),
                        ),
                        if (n.link?.isNotEmpty ?? false)
                          InkWell(
                            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                            onTap: () {
                              // Abrir enlace externo
                              // TODO: usar url_launcher si está disponible
                            },
                            child: const Icon(
                              Icons.open_in_new_rounded,
                              size: 16,
                              color: AppColors.text,
                            ),
                          ),
                      ],
                    ),
                    const SizedBox(height: AppDimens.spaceXs),
                    Text(
                      _formatWhen(n.createdAt),
                      style: const TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                        color: AppColors.mutedStrong,
                      ),
                    ),
                    if (n.groupName?.isNotEmpty ?? false) ...[
                      const SizedBox(height: AppDimens.spaceXs),
                      Text(
                        'Grupo: ${n.groupName}',
                        style: const TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w700,
                          color: AppColors.mutedStrong,
                        ),
                      ),
                    ],
                    if (n.description?.isNotEmpty ?? false) ...[
                      const SizedBox(height: AppDimens.spaceSm),
                      Text(
                        n.description!,
                        maxLines: 3,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: AppColors.text,
                          height: 1.4,
                        ),
                      ),
                    ],
                    if (n.link?.isNotEmpty ?? false) ...[
                      const SizedBox(height: AppDimens.spaceSm),
                      NeobrutalistButton(
                        label: 'Ver reunión',
                        icon: Icons.calendar_today_rounded,
                        variant: NeobrutalistButtonVariant.secondary,
                        padding: const EdgeInsets.symmetric(
                          horizontal: AppDimens.spaceMd,
                          vertical: AppDimens.spaceSm,
                        ),
                        onPressed: () {
                          // Abrir enlace de la reunión
                          // TODO: usar url_launcher si está disponible
                        },
                      ),
                    ],
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: AppDimens.spaceMd),
              child: Row(
                children: [
                  NeobrutalistBadge(
                    label: unread ? 'Nueva' : 'Leída',
                    tone: unread
                        ? NeobrutalistTone.accent
                        : NeobrutalistTone.neutral,
                    compact: true,
                  ),
                  const Spacer(),
                  NeobrutalistButton(
                    label: 'Eliminar',
                    icon: Icons.delete_rounded,
                    variant: NeobrutalistButtonVariant.danger,
                    padding: const EdgeInsets.symmetric(
                      horizontal: AppDimens.spaceMd,
                      vertical: AppDimens.spaceSm,
                    ),
                    onPressed: _deleting.contains(n.id) ? null : () => _delete(n.id),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
