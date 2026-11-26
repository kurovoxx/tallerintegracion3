import 'package:flutter/material.dart';

import '../models/social_models.dart';
import '../services/social_service.dart';
import '../theme/app_theme.dart';
import 'neobrutalism.dart';

/// Invitaciones pendientes presentadas como notificaciones accionables.
class GroupInvitationNotifications extends StatefulWidget {
  const GroupInvitationNotifications({
    super.key,
    required this.service,
    this.onPendingCountChanged,
  });

  final SocialService service;
  final ValueChanged<int>? onPendingCountChanged;

  @override
  State<GroupInvitationNotifications> createState() =>
      _GroupInvitationNotificationsState();
}

class _GroupInvitationNotificationsState
    extends State<GroupInvitationNotifications> {
  List<GroupInvitation> _invitations = const [];
  final Set<String> _responding = <String>{};
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final invitations = await widget.service.listPendingInvitations();
      if (!mounted) return;
      setState(() {
        _invitations = invitations;
        _loading = false;
      });
      widget.onPendingCountChanged?.call(invitations.length);
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'No se pudieron cargar las invitaciones.';
        _loading = false;
      });
    }
  }

  Future<void> _respond(
    GroupInvitation invitation, {
    required bool accept,
  }) async {
    if (!_responding.add(invitation.id)) return;
    setState(() {});
    try {
      if (accept) {
        await widget.service.acceptInvitation(invitation.id);
      } else {
        await widget.service.rejectInvitation(invitation.id);
      }
      if (!mounted) return;
      setState(() {
        _invitations = _invitations
            .where((item) => item.id != invitation.id)
            .toList();
      });
      widget.onPendingCountChanged?.call(_invitations.length);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            accept
                ? 'Te uniste al grupo ${invitation.groupName}.'
                : 'Invitación a ${invitation.groupName} rechazada.',
          ),
        ),
      );
    } on SocialApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(e.message),
            backgroundColor: AppColors.errorDeep,
          ),
        );
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('No se pudo responder a la invitación.'),
            backgroundColor: AppColors.errorDeep,
          ),
        );
      }
    } finally {
      _responding.remove(invitation.id);
      if (mounted) setState(() {});
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.all(AppDimens.spaceSm),
        child: Center(
          child: SizedBox(
            width: 18,
            height: 18,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        ),
      );
    }
    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.all(AppDimens.spaceSm),
        child: Row(
          children: [
            Expanded(
              child: Text(
                _error!,
                style: TextStyle(
                  color: AppColors.errorDeep,
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
            NeobrutalistIconButton(
              icon: Icons.refresh_rounded,
              tooltip: 'Reintentar invitaciones',
              size: 38,
              onPressed: _load,
            ),
          ],
        ),
      );
    }
    if (_invitations.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(
            AppDimens.spaceSm,
            AppDimens.spaceSm,
            AppDimens.spaceSm,
            AppDimens.spaceXs,
          ),
          child: Row(
            children: [
              Expanded(
                child: Text(
                  'INVITACIONES A GRUPOS',
                  style: TextStyle(
                    color: AppColors.text,
                    fontSize: 11,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
              NeobrutalistBadge(
                label: '${_invitations.length}',
                tone: NeobrutalistTone.accent,
                compact: true,
              ),
            ],
          ),
        ),
        ConstrainedBox(
          constraints: const BoxConstraints(maxHeight: 190),
          child: ListView.builder(
            shrinkWrap: true,
            padding: const EdgeInsets.all(AppDimens.spaceSm),
            itemCount: _invitations.length,
            itemBuilder: (context, index) =>
                _invitationTile(_invitations[index]),
          ),
        ),
      ],
    );
  }

  Widget _invitationTile(GroupInvitation invitation) {
    final responding = _responding.contains(invitation.id);
    final inviter = invitation.invitedByName?.trim();
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: NeobrutalistCard(
        backgroundColor: AppColors.surface,
        shadow: AppShadows.hard2,
        padding: const EdgeInsets.all(AppDimens.spaceSm),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '${inviter == null || inviter.isEmpty ? "Alguien" : inviter} te invita a ${invitation.groupName}',
              style: TextStyle(
                color: AppColors.text,
                fontSize: 12,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: AppDimens.spaceSm),
            Wrap(
              spacing: AppDimens.spaceSm,
              runSpacing: AppDimens.spaceSm,
              children: [
                NeobrutalistButton(
                  label: 'Aceptar',
                  icon: Icons.check_rounded,
                  variant: NeobrutalistButtonVariant.accent,
                  padding: const EdgeInsets.symmetric(
                    horizontal: AppDimens.spaceMd,
                    vertical: AppDimens.spaceSm,
                  ),
                  onPressed: responding
                      ? null
                      : () => _respond(invitation, accept: true),
                ),
                NeobrutalistButton(
                  label: 'Rechazar',
                  icon: Icons.close_rounded,
                  variant: NeobrutalistButtonVariant.secondary,
                  padding: const EdgeInsets.symmetric(
                    horizontal: AppDimens.spaceMd,
                    vertical: AppDimens.spaceSm,
                  ),
                  onPressed: responding
                      ? null
                      : () => _respond(invitation, accept: false),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
