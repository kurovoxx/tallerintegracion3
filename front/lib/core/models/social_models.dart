// Modelos mínimos para contratos reales de Social (Benjamín).
// Ver back/social/internal/model/group.go, views.go y handlers:
// - GET /me/overview -> MyOverview
// - GET /groups/:id -> GroupView
// - GET /groups/:id/workspace -> Workspace
// - GET /groups/:id/todo -> todoToJSON
// - GET /groups/:id/sprint-sheet -> sprintTaskToJSON
// No inventa campos que el backend no entregue (ej. subject, avatares).

class SocialApiException implements Exception {
  SocialApiException(this.message, {this.statusCode, this.code});

  final String message;
  final int? statusCode;
  final String? code;

  @override
  String toString() {
    final c = code ?? 'error';
    final s = statusCode?.toString() ?? '?';
    return '[$c:$s] $message';
  }
}

String _str(Map<String, dynamic> j, String key, [String fallback = '']) {
  final v = j[key];
  if (v is String) return v;
  return fallback;
}

int _int(Map<String, dynamic> j, String key, [int fallback = 0]) {
  final v = j[key];
  if (v is int) return v;
  if (v is num) return v.toInt();
  return fallback;
}

class GroupCard {
  GroupCard({
    required this.groupId,
    required this.name,
    this.description,
    required this.role,
    required this.memberCount,
    this.joinedAt,
  });

  final String groupId;
  final String name;
  final String? description;
  final String role;
  final int memberCount;
  final DateTime? joinedAt;

  factory GroupCard.fromJson(Map<String, dynamic> j) {
    DateTime? joined;
    final raw = j['joined_at'];
    if (raw is String) joined = DateTime.tryParse(raw);
    return GroupCard(
      groupId: _str(j, 'group_id'),
      name: _str(j, 'name', 'Grupo'),
      description: j['description'] as String?,
      role: _str(j, 'role', 'member'),
      memberCount: _int(j, 'member_count'),
      joinedAt: joined,
    );
  }
}

class SidebarGroup {
  SidebarGroup({
    required this.groupId,
    required this.name,
    required this.role,
  });

  final String groupId;
  final String name;
  final String role;

  factory SidebarGroup.fromJson(Map<String, dynamic> j) => SidebarGroup(
        groupId: _str(j, 'group_id'),
        name: _str(j, 'name', 'Grupo'),
        role: _str(j, 'role', 'member'),
      );
}

class Overview {
  Overview({
    required this.userId,
    required this.sidebarGroups,
    required this.groups,
    required this.groupsCount,
    required this.adminGroupsCount,
  });

  final String userId;
  final List<SidebarGroup> sidebarGroups;
  final List<GroupCard> groups;
  final int groupsCount;
  final int adminGroupsCount;

  factory Overview.fromJson(Map<String, dynamic> j) {
    final sidebar =
        (j['sidebar'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    final sideRaw = (sidebar['groups'] as List?) ?? const [];
    final groupsRaw = (j['groups'] as List?) ?? const [];
    final stats =
        (j['stats'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return Overview(
      userId: _str(j, 'user_id'),
      sidebarGroups: sideRaw
          .whereType<Map>()
          .map((e) =>
              SidebarGroup.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
      groups: groupsRaw
          .whereType<Map>()
          .map((e) => GroupCard.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
      groupsCount: _int(stats, 'groups_count'),
      adminGroupsCount: _int(stats, 'admin_groups_count'),
    );
  }
}

// GET /groups/:id -> GroupView (group_handler.go:119 Get).
// Campos reales: id, name, description?, owner_user_id,
// notes_restricted_to_staff, invite_token? (solo admin), created_at, role.
// No trae member_count ni subject: no mostrarlos como reales.
class GroupDetail {
  GroupDetail({
    required this.id,
    required this.name,
    this.description,
    required this.ownerUserId,
    required this.notesRestrictedToStaff,
    this.inviteToken,
    this.createdAt,
    required this.role,
  });

  final String id;
  final String name;
  final String? description;
  final String ownerUserId;
  final bool notesRestrictedToStaff;
  final String? inviteToken;
  final DateTime? createdAt;
  final String role;

  factory GroupDetail.fromJson(Map<String, dynamic> j) {
    DateTime? created;
    final raw = j['created_at'];
    if (raw is String) created = DateTime.tryParse(raw);
    final notesRestricted = j['notes_restricted_to_staff'];
    return GroupDetail(
      id: _str(j, 'id'),
      name: _str(j, 'name', 'Grupo'),
      description: j['description'] as String?,
      ownerUserId: _str(j, 'owner_user_id'),
      notesRestrictedToStaff:
          notesRestricted is bool ? notesRestricted : false,
      inviteToken: j['invite_token'] as String?,
      createdAt: created,
      role: _str(j, 'role', 'member'),
    );
  }
}

class WorkspaceGroup {
  WorkspaceGroup({
    required this.id,
    required this.name,
    this.description,
    required this.role,
  });

  final String id;
  final String name;
  final String? description;
  final String role;

  factory WorkspaceGroup.fromJson(Map<String, dynamic> j) => WorkspaceGroup(
        id: _str(j, 'id'),
        name: _str(j, 'name', 'Grupo'),
        description: j['description'] as String?,
        role: _str(j, 'role', 'member'),
      );
}

class TodoTask {
  TodoTask({
    required this.id,
    required this.groupId,
    required this.boardId,
    required this.boardName,
    required this.title,
    required this.status,
    this.assignedTo,
    this.dueDate,
    this.createdAt,
    this.updatedAt,
  });

  final String id;
  final String groupId;
  final String boardId;
  final String boardName;
  final String title;
  final String status;
  final String? assignedTo;
  final String? dueDate;
  final String? createdAt;
  final String? updatedAt;

  factory TodoTask.fromJson(Map<String, dynamic> j) => TodoTask(
        id: _str(j, 'id'),
        groupId: _str(j, 'group_id'),
        boardId: _str(j, 'board_id'),
        boardName: _str(j, 'board_name', 'General'),
        title: _str(j, 'title', 'Sin título'),
        status: _str(j, 'status', 'todo'),
        assignedTo: j['assigned_to'] as String?,
        dueDate: j['due_date'] as String?,
        createdAt: j['created_at'] as String?,
        updatedAt: j['updated_at'] as String?,
      );
}

class SprintSheetInfo {
  SprintSheetInfo({
    required this.id,
    required this.name,
    this.periodStart,
    this.periodEnd,
  });

  final String id;
  final String name;
  final String? periodStart;
  final String? periodEnd;

  factory SprintSheetInfo.fromJson(Map<String, dynamic> j) =>
      SprintSheetInfo(
        id: _str(j, 'id'),
        name: _str(j, 'name', 'Sprint'),
        periodStart: j['period_start'] as String?,
        periodEnd: j['period_end'] as String?,
      );
}

class SprintTask {
  SprintTask({
    required this.id,
    required this.groupId,
    required this.sheetId,
    required this.sheetName,
    required this.title,
    required this.assignedTo,
    required this.priority,
    required this.status,
    required this.estimatedHours,
    this.createdAt,
    this.updatedAt,
  });

  final String id;
  final String groupId;
  final String sheetId;
  final String sheetName;
  final String title;
  final String assignedTo;
  final String priority;
  final String status;
  final double estimatedHours;
  final String? createdAt;
  final String? updatedAt;

  factory SprintTask.fromJson(Map<String, dynamic> j) {
    final raw = j['estimated_hours'];
    double hours = 0;
    if (raw is num) hours = raw.toDouble();
    return SprintTask(
      id: _str(j, 'id'),
      groupId: _str(j, 'group_id'),
      sheetId: _str(j, 'sheet_id'),
      sheetName: _str(j, 'sheet_name', ''),
      title: _str(j, 'title', 'Sin título'),
      assignedTo: _str(j, 'assigned_to'),
      priority: _str(j, 'priority', 'media'),
      status: _str(j, 'status', 'sin_empezar'),
      estimatedHours: hours,
      createdAt: j['created_at'] as String?,
      updatedAt: j['updated_at'] as String?,
    );
  }
}

class MeetingItem {
  MeetingItem({
    required this.id,
    required this.title,
    this.description,
    required this.scheduledAt,
  });

  final String id;
  final String title;
  final String? description;
  final DateTime scheduledAt;

  factory MeetingItem.fromJson(Map<String, dynamic> j) {
    DateTime when = DateTime.now();
    final raw = j['scheduled_at'];
    if (raw is String) when = DateTime.tryParse(raw) ?? when;
    return MeetingItem(
      id: _str(j, 'id'),
      title: _str(j, 'title', 'Reunión'),
      description: j['description'] as String?,
      scheduledAt: when,
    );
  }
}

class Workspace {
  Workspace({
    required this.group,
    required this.todo,
    required this.inProgress,
    required this.done,
    required this.sheets,
    required this.sprintTasks,
    required this.upcomingMeetings,
    required this.chatProvider,
    required this.chatTokenEndpoint,
  });

  final WorkspaceGroup group;
  final List<TodoTask> todo;
  final List<TodoTask> inProgress;
  final List<TodoTask> done;
  final List<SprintSheetInfo> sheets;
  final List<SprintTask> sprintTasks;
  final List<MeetingItem> upcomingMeetings;
  final String chatProvider;
  final String chatTokenEndpoint;

  factory Workspace.fromJson(Map<String, dynamic> j) {
    final g =
        (j['group'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    final kanban =
        (j['kanban'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    final sprint =
        (j['sprint_sheet'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    final meetings =
        (j['meetings'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    final chat =
        (j['chat'] as Map<String, dynamic>?) ?? const <String, dynamic>{};

    List<TodoTask> todos(String key) {
      final raw = (kanban[key] as List?) ?? const [];
      return raw
          .whereType<Map>()
          .map((e) => TodoTask.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList();
    }

    final sheetsRaw = (sprint['sheets'] as List?) ?? const [];
    final tasksRaw = (sprint['tasks'] as List?) ?? const [];
    final upcomingRaw = (meetings['upcoming'] as List?) ?? const [];

    return Workspace(
      group: WorkspaceGroup.fromJson(g),
      todo: todos('todo'),
      inProgress: todos('in_progress'),
      done: todos('done'),
      sheets: sheetsRaw
          .whereType<Map>()
          .map((e) =>
              SprintSheetInfo.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
      sprintTasks: tasksRaw
          .whereType<Map>()
          .map((e) => SprintTask.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
      upcomingMeetings: upcomingRaw
          .whereType<Map>()
          .map((e) => MeetingItem.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(),
      chatProvider: _str(chat, 'provider', 'stream'),
      chatTokenEndpoint: _str(chat, 'token_endpoint'),
    );
  }
}
