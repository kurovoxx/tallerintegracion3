import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../../core/widgets/group_invitation_notifications.dart';

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
  int _pendingInvitationCount = 0;
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
        SnackBar(content: Text(e.message), backgroundColor: AppColors.error),
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
            if (item.id == n.id)
              MeetingNotification(
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
            else
              item,
        ];
      });
    } catch (_) {
      // Best-effort: la recarga muestra el estado real.
    }
  }

  Future<void> _openLink(String link) async {
    final uri = Uri.tryParse(link);
    if (uri == null) return;
    final opened = await launchUrl(uri, mode: LaunchMode.externalApplication);
    if (opened || !mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('No se pudo abrir el enlace de la reunión.'),
      ),
    );
  }

  static String _formatWhen(DateTime when) {
    final local = when.toLocal();
    return '${local.day.toString().padLeft(2, '0')}/${local.month.toString().padLeft(2, '0')}/${local.year} · '
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final unreadCount = _items.where((n) => !n.read).length;
    // Alto acotado: el marco no ocupa toda la pantalla y la lista hace su
    // propio scroll dentro del espacio restante.
    final maxHeight = (MediaQuery.sizeOf(context).height - 48).clamp(
      260.0,
      600.0,
    );
    return ConstrainedBox(
      constraints: BoxConstraints(maxHeight: maxHeight),
      child: NeobrutalistDialog(
        title: 'Notificaciones',
        maxWidth: 460,
        expandContent: true,
        content: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                const Expanded(
                  child: Text(
                    'TUS NOTIFICACIONES',
                    style: TextStyle(
                      fontWeight: FontWeight.w900,
                      fontSize: 11.5,
                      letterSpacing: 0.4,
                      color: AppColors.mutedStrong,
                    ),
                  ),
                ),
                if (!_loading && unreadCount > 0) ...[
                  NeobrutalistBadge(
                    label: unreadCount == 1 ? '1 nueva' : '$unreadCount nuevas',
                    tone: NeobrutalistTone.accent,
                    compact: true,
                  ),
                  const SizedBox(width: AppDimens.spaceSm),
                ],
                NeobrutalistIconButton(
                  icon: Icons.close_rounded,
                  tooltip: 'Cerrar',
                  size: 34,
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
            const SizedBox(height: AppDimens.spaceMd),
            Expanded(
              child: Column(
                children: [
                  GroupInvitationNotifications(
                    service: _service,
                    onPendingCountChanged: (count) {
                      if (mounted) {
                        setState(() => _pendingInvitationCount = count);
                      }
                    },
                  ),
                  Expanded(child: _buildList()),
                ],
              ),
            ),
          ],
        ),
        actions: [
          NeobrutalistButton(
            label: 'Cerrar',
            icon: Icons.close_rounded,
            variant: NeobrutalistButtonVariant.secondary,
            onPressed: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }

  /// Estado vacío/error como pieza neobrutalista (borde, sombra dura, relieve).
  Widget _stateCard({
    required IconData icon,
    required String title,
    required String message,
    Widget? action,
  }) {
    return Center(
      child: NeobrutalistCard(
        backgroundColor: AppColors.surfaceLow,
        shadow: AppShadows.hard3,
        padding: const EdgeInsets.all(AppDimens.spaceXl),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 44,
              height: 44,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: AppColors.accentYellow,
                border: Border.all(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
                boxShadow: AppShadows.badge,
              ),
              child: Icon(icon, size: 22, color: AppColors.text),
            ),
            const SizedBox(height: AppDimens.spaceMd),
            Text(
              title,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.4,
                color: AppColors.text,
              ),
            ),
            const SizedBox(height: AppDimens.spaceXs),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
            if (action != null) ...[
              const SizedBox(height: AppDimens.spaceMd),
              action,
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildList() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator(strokeWidth: 2));
    }
    if (_error != null) {
      return _stateCard(
        icon: Icons.cloud_off_rounded,
        title: 'SIN CONEXIÓN',
        message: _error!,
        action: NeobrutalistButton(
          label: 'Reintentar',
          icon: Icons.refresh_rounded,
          variant: NeobrutalistButtonVariant.secondary,
          onPressed: _load,
        ),
      );
    }
    if (_items.isEmpty) {
      if (_pendingInvitationCount > 0) {
        return const SizedBox.shrink();
      }
      return _stateCard(
        icon: Icons.notifications_none_rounded,
        title: 'SIN NOTIFICACIONES',
        message: 'Aquí verás los recordatorios de tus reuniones.',
      );
    }
    return Scrollbar(
      thumbVisibility: true,
      trackVisibility: true,
      thickness: 8,
      radius: Radius.zero,
      child: ListView.builder(
        padding: const EdgeInsets.only(right: AppDimens.spaceSm),
        itemCount: _items.length,
        itemBuilder: (context, i) => _tile(_items[i]),
      ),
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
                            borderRadius: BorderRadius.circular(
                              AppDimens.radiusChip,
                            ),
                            onTap: () => _openLink(n.link!),
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
                      'Reunión: ${_formatWhen(n.scheduledAt)}',
                      style: const TextStyle(
                        fontSize: 11.5,
                        fontWeight: FontWeight.w800,
                        color: AppColors.text,
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
                        label: 'Abrir enlace',
                        icon: Icons.link_rounded,
                        variant: NeobrutalistButtonVariant.secondary,
                        padding: const EdgeInsets.symmetric(
                          horizontal: AppDimens.spaceMd,
                          vertical: AppDimens.spaceSm,
                        ),
                        onPressed: () => _openLink(n.link!),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(
                horizontal: AppDimens.spaceMd,
              ),
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
                    onPressed: _deleting.contains(n.id)
                        ? null
                        : () => _delete(n.id),
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
