import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/chat_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/chat/group_chat_screen.dart';
import 'package:taller_integracion_front/features/groups/group_detail_screen.dart';
import 'package:taller_integracion_front/features/groups/groups_screen.dart';
import 'package:taller_integracion_front/features/workspace/kanban_screen.dart';
import 'package:taller_integracion_front/features/workspace/schedule_meeting_screen.dart';
import 'package:taller_integracion_front/features/workspace/sprint_sheet_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _memberId = 'bbbbbbbb-0000-4000-8000-000000000001';

class _FakeChatSession implements ChatSession {
  _FakeChatSession({List<ChatMessage>? initial})
    : messages = Stream.value(initial ?? const []);

  @override
  final Stream<List<ChatMessage>> messages;

  final sent = <String>[];

  @override
  Future<void> send(String text) async {
    sent.add(text);
  }

  @override
  Future<void> close() async {}
}

Map<String, dynamic> _overview(List<Map<String, dynamic>> groups) => {
  'user_id': 'u-test',
  'sidebar': {
    'groups': [
      for (final g in groups)
        {'group_id': g['group_id'], 'name': g['name'], 'role': g['role']},
    ],
  },
  'groups': groups,
  'stats': {
    'groups_count': groups.length,
    'admin_groups_count': groups.where((g) => g['role'] == 'admin').length,
  },
};

http.Response _ok(Object body, [int status = 200]) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

Future<void> _pump(WidgetTester tester, Widget w, Size size) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(home: w));
  await tester.pumpAndSettle();
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  group('GROUPS — estilo vistas con datos reales', () {
    testWidgets('iniciales reales desde display_name, sin +N ambiguo', (
      tester,
    ) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final groups = [
        {
          'group_id': _gid,
          'name': 'Grupo Real',
          'description': 'INF-360',
          'role': 'admin',
          'member_count': 2,
          'joined_at': '2026-08-11T12:00:00Z',
        },
      ];
      final service = SocialService(
        client: MockClient((request) async {
          if (request.method == 'GET' && request.url.path == '/me/overview') {
            return _ok(_overview(groups));
          }
          if (request.method == 'GET' &&
              request.url.path.endsWith('/members')) {
            return _ok([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': _memberId,
                'role': 'admin',
                'display_name': 'Agustín Vega',
              },
              {
                'id': 'm2',
                'group_id': _gid,
                'user_id': 'cccccccc-0000-4000-8000-000000000002',
                'role': 'member',
                'display_name': 'Ana',
              },
            ]);
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);

      await _pump(
        tester,
        GroupsScreen(service: service),
        const Size(1440, 900),
      );

      // Agustín Vega -> AV, sin UUID visible.
      expect(find.text('AV'), findsOneWidget);
      expect(find.textContaining('2 integrantes'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets('INVITAR solo admin; miembro ve Info', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final groups = [
        {
          'group_id': _gid,
          'name': 'Grupo Admin',
          'description': 'INF',
          'role': 'admin',
          'member_count': 1,
          'joined_at': '2026-08-11T12:00:00Z',
        },
        {
          'group_id': '22222222-2222-2222-2222-222222222222',
          'name': 'Grupo Miembro',
          'description': 'INF',
          'role': 'member',
          'member_count': 1,
          'joined_at': '2026-08-11T12:00:00Z',
        },
      ];
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path == '/me/overview') return _ok(_overview(groups));
          if (request.url.path.endsWith('/members')) return _ok([]);
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupsScreen(service: service),
        const Size(1440, 900),
      );
      expect(find.text('INVITAR'), findsOneWidget);
      expect(find.text('INFO'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    test('create notifica sidebar (groupsChanged)', () async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final before = SocialService.groupsChanged.value;
      final service = SocialService(
        client: MockClient((request) async {
          if (request.method == 'POST' && request.url.path == '/groups') {
            return _ok({'group_id': _gid}, 201);
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await service.createGroup(name: 'Nuevo');
      expect(SocialService.groupsChanged.value, before + 1);
    });

    test('leave llama POST /groups/:id/leave y notifica', () async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      var called = false;
      final before = SocialService.groupsChanged.value;
      final service = SocialService(
        client: MockClient((request) async {
          if (request.method == 'POST' &&
              request.url.path == '/groups/$_gid/leave') {
            called = true;
            return http.Response('', 204);
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await service.leaveGroup(_gid);
      expect(called, isTrue);
      expect(SocialService.groupsChanged.value, before + 1);
    });

    testWidgets('dialogo Info muestra Abandonar grupo', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final groups = [
        {
          'group_id': _gid,
          'name': 'Grupo Real',
          'description': 'INF',
          'role': 'member',
          'member_count': 1,
          'joined_at': '2026-08-11T12:00:00Z',
        },
      ];
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path == '/me/overview') return _ok(_overview(groups));
          if (request.url.path.endsWith('/members')) return _ok([]);
          if (request.method == 'GET' && request.url.path == '/groups/$_gid') {
            return _ok({
              'id': _gid,
              'name': 'Grupo Real',
              'description': 'INF',
              'owner_user_id': 'u',
              'notes_restricted_to_staff': false,
              'role': 'member',
            });
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupsScreen(service: service),
        const Size(1440, 900),
      );
      await tester.tap(find.text('INFO').first);
      await tester.pumpAndSettle();
      expect(find.text('Abandonar grupo'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  });

  group('DISCORD — tarjeta histórica + config solo admin', () {
    MockClient discordClient({required String role, bool withConfig = true}) {
      return MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path == '/groups/$_gid') {
          return _ok({
            'id': _gid,
            'name': 'G',
            'owner_user_id': 'u',
            'notes_restricted_to_staff': false,
            'role': role,
          });
        }
        if (request.method == 'GET' && path.endsWith('/discord-config')) {
          if (!withConfig) return http.Response('{}', 404);
          return _ok({
            'server_name': 'Servidor Real',
            'invite_url': 'https://discord.gg/real123',
            'webhook_url': null,
          });
        }
        if (request.method == 'GET' && path.endsWith('/workspace')) {
          return _ok({
            'group': {'id': _gid, 'name': 'G', 'role': role},
            'kanban': {'todo': [], 'in_progress': [], 'done': []},
            'sprint_sheet': {'sheets': [], 'tasks': []},
            'meetings': {'upcoming': []},
            'chat': {'provider': 'stream', 'token_endpoint': ''},
          });
        }
        return _ok({}, 404);
      });
    }

    testWidgets('admin ve ABRIR + Configurar; miembro solo ABRIR', (
      tester,
    ) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);

      var service = SocialService(client: discordClient(role: 'admin'));
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupDetailScreen(
          group: {'id': _gid, 'name': 'G', 'role': 'admin'},
          service: service,
        ),
        const Size(1280, 900),
      );
      // Ir a pestaña DISCORD (índice 1).
      await tester.tap(find.text('DISCORD').first);
      await tester.pumpAndSettle();
      expect(find.text('DISCORD DEL GRUPO'), findsOneWidget);
      expect(find.text('ABRIR DISCORD'), findsOneWidget);
      expect(find.text('Configurar Discord'), findsOneWidget);
      expect(find.text('Servidor Real'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets('miembro no ve Configurar Discord', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(client: discordClient(role: 'member'));
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupDetailScreen(
          group: {'id': _gid, 'name': 'G', 'role': 'member'},
          service: service,
        ),
        const Size(1280, 900),
      );
      await tester.tap(find.text('DISCORD').first);
      await tester.pumpAndSettle();
      expect(find.text('DISCORD DEL GRUPO'), findsOneWidget);
      expect(find.text('ABRIR DISCORD'), findsOneWidget);
      expect(find.text('Configurar Discord'), findsNothing);
      expect(tester.takeException(), isNull);
    });
  });

  group('SPRINT — carcasa única y horas', () {
    Map<String, dynamic> workspace({
      required List<Map<String, dynamic>> tasks,
      required List<Map<String, dynamic>> sheets,
    }) => {
      'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
      'kanban': {'todo': [], 'in_progress': [], 'done': []},
      'sprint_sheet': {'sheets': sheets, 'tasks': tasks},
      'meetings': {'upcoming': []},
      'chat': {'provider': 'stream', 'token_endpoint': ''},
    };

    testWidgets('grupo vacío muestra carcasa + mensaje integrado', (
      tester,
    ) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path.endsWith('/workspace')) {
            return _ok(workspace(tasks: [], sheets: []));
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        SprintSheetScreen(groupId: _gid, service: service),
        const Size(1280, 900),
      );
      expect(find.text('HOJA DE SPRINT'), findsWidgets);
      expect(find.text('Aún no hay tareas en este sprint.'), findsOneWidget);
      // Un solo NUEVA TAREA arriba.
      expect(find.text('NUEVA TAREA'), findsOneWidget);
      expect(find.text('AGREGAR TAREA'), findsNothing);
      expect(tester.takeException(), isNull);
    });

    testWidgets('excedidas muestra 0h restantes + Xh excedidas', (
      tester,
    ) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          final path = request.url.path;
          if (request.method == 'GET' && path.endsWith('/workspace')) {
            return _ok(
              workspace(
                tasks: [
                  {
                    'id': 'dddddddd-0000-4000-8000-000000000001',
                    'group_id': _gid,
                    'sheet_id': 'cccccccc-0000-4000-8000-000000000001',
                    'sheet_name': 'Sprint 1',
                    'title': 'Tarea Excedida',
                    'assigned_to': _memberId,
                    'priority': 'alta',
                    'status': 'en_proceso',
                    'estimated_hours': 2.0,
                  },
                ],
                sheets: [
                  {
                    'id': 'cccccccc-0000-4000-8000-000000000001',
                    'name': 'Sprint 1',
                    'period_start': '2026-09-28',
                    'period_end': '2026-10-02',
                  },
                ],
              ),
            );
          }
          if (path.endsWith('/members')) {
            return _ok([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': _memberId,
                'role': 'member',
                'display_name': 'Ana',
              },
            ]);
          }
          if (path.contains('/hours')) {
            return _ok({
              'data': [
                {
                  'task_id': 'dddddddd-0000-4000-8000-000000000001',
                  'log_date': '2026-09-29',
                  'hours': 5.0,
                },
              ],
              'total_hours': 5.0,
            });
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        SprintSheetScreen(groupId: _gid, service: service),
        const Size(1280, 900),
      );
      expect(find.textContaining('excedidas'), findsWidgets);
      expect(tester.takeException(), isNull);
    });

    testWidgets('sin Sprint 3 hardcode en modo real', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path.endsWith('/workspace')) {
            return _ok(
              workspace(
                tasks: [
                  {
                    'id': 'dddddddd-0000-4000-8000-000000000001',
                    'group_id': _gid,
                    'sheet_id': 'cccccccc-0000-4000-8000-000000000001',
                    'sheet_name': 'Sprint 1',
                    'title': 'Tarea A',
                    'assigned_to': _memberId,
                    'priority': 'media',
                    'status': 'sin_empezar',
                    'estimated_hours': 1.0,
                  },
                ],
                sheets: [
                  {
                    'id': 'cccccccc-0000-4000-8000-000000000001',
                    'name': 'Sprint 1',
                  },
                ],
              ),
            );
          }
          if (request.url.path.endsWith('/members')) return _ok([]);
          if (request.url.path.contains('/hours')) {
            return _ok({'data': [], 'total_hours': 0});
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        SprintSheetScreen(groupId: _gid, service: service),
        const Size(1280, 900),
      );
      expect(find.text('SPRINT 3'), findsNothing);
      expect(find.text('SPRINT 1'), findsWidgets);
      expect(tester.takeException(), isNull);
    });
  });

  group('KANBAN — sin decoración falsa', () {
    MockClient kanbanClient() {
      return MockClient((request) async {
        if (request.url.path.endsWith('/workspace')) {
          return _ok({
            'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
            'kanban': {
              'todo': [
                {
                  'id': 'aaaaaaaa-0000-4000-8000-000000000001',
                  'group_id': _gid,
                  'board_id': 'b1',
                  'board_name': 'General',
                  'title': 'Tarea Real',
                  'status': 'todo',
                  'assigned_to': null,
                },
              ],
              'in_progress': [],
              'done': [],
            },
            'sprint_sheet': {'sheets': [], 'tasks': []},
            'meetings': {'upcoming': []},
            'chat': {'provider': 'stream', 'token_endpoint': ''},
          });
        }
        if (request.url.path.endsWith('/members')) return _ok([]);
        return _ok({}, 404);
      });
    }

    testWidgets('texto organiza, sin GENERAL ni SPRINT 3', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(client: kanbanClient());
      addTearDown(service.dispose);
      await _pump(
        tester,
        KanbanScreen(groupId: _gid, service: service),
        const Size(1440, 900),
      );
      expect(find.text('Organiza las tareas según su estado.'), findsOneWidget);
      expect(
        find.text(
          'Arrastra las fichas técnicas entre placas para cambiar su estado.',
        ),
        findsNothing,
      );
      expect(find.text('GENERAL'), findsNothing);
      expect(find.text('SPRINT 3'), findsNothing);
      expect(find.textContaining('Tarea Real'), findsWidgets);
      // Move por flechas sigue funcionando.
      expect(find.byTooltip('Mover a columna siguiente'), findsWidgets);
      expect(tester.takeException(), isNull);
    });
  });

  group('MEETINGS — layout y upcoming real', () {
    testWidgets('formulario + próximas reales', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path.endsWith('/workspace')) {
            return _ok({
              'group': {'id': _gid, 'name': 'G', 'role': 'member'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {'sheets': [], 'tasks': []},
              'meetings': {
                'upcoming': [
                  {
                    'id': 'm1',
                    'title': 'Reunión Real',
                    'description': 'Desc',
                    'scheduled_at': '2026-10-05T15:00:00Z',
                  },
                ],
              },
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            });
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        ScheduleMeetingScreen(groupId: _gid, service: service),
        const Size(1280, 900),
      );
      expect(find.text('AGENDAR REUNIÓN'), findsWidgets);
      expect(find.text('Reunión Real'), findsOneWidget);
      // Sin campos fantasma históricos.
      expect(find.text('Vincular Google Calendar'), findsNothing);
      expect(find.text('MIEMBROS INVITADOS'), findsNothing);
      expect(tester.takeException(), isNull);
    });
  });

  group('CHECKPOINT — Invite/Info/Join + limpieza', () {
    testWidgets('admin tiene INVITAR e INFO; Info no duplica código', (
      tester,
    ) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final groups = [
        {
          'group_id': _gid,
          'name': 'Grupo Admin',
          'description': 'INF',
          'role': 'admin',
          'member_count': 1,
          'joined_at': '2026-08-11T12:00:00Z',
        },
      ];
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path == '/me/overview') {
            return _ok(_overview(groups));
          }
          if (request.url.path.endsWith('/members')) return _ok([]);
          if (request.method == 'GET' && request.url.path == '/groups/$_gid') {
            return _ok({
              'id': _gid,
              'name': 'Grupo Admin',
              'description': 'INF',
              'owner_user_id': 'u',
              'notes_restricted_to_staff': false,
              'role': 'admin',
              'invite_token': 'tok-123',
            });
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupsScreen(service: service),
        const Size(1440, 900),
      );

      expect(find.text('INVITAR'), findsOneWidget);
      expect(find.byTooltip('Información del grupo'), findsOneWidget);
      // INFO sin código: el código vive solo en INVITAR.
      await tester.tap(find.byTooltip('Información del grupo'));
      await tester.pumpAndSettle();
      expect(find.text('Abandonar grupo'), findsOneWidget);
      expect(find.text('Código de invitación'), findsNothing);
      await tester.tap(find.text('Cerrar'));
      await tester.pumpAndSettle();
      // INVITAR muestra identificador + código, ambos copiables.
      await tester.tap(find.text('INVITAR'));
      await tester.pumpAndSettle();
      expect(find.text('Identificador del grupo'), findsOneWidget);
      expect(find.text('Código de invitación'), findsOneWidget);
      // Ambos valores son SelectableText (seleccionables/copiables).
      expect(
        find.byWidgetPredicate((w) => w is SelectableText && w.data == _gid),
        findsOneWidget,
      );
      expect(
        find.byWidgetPredicate(
          (w) => w is SelectableText && w.data == 'tok-123',
        ),
        findsOneWidget,
      );
      expect(find.text('Generar nuevo código'), findsOneWidget);
      expect(find.text('Copiar datos de invitación'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets('invite y join son 1:1 (mismos campos)', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path == '/me/overview') {
            return _ok(_overview([]));
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupsScreen(service: service),
        const Size(1440, 900),
      );

      // Join pide exactamente lo que Invite muestra.
      await tester.tap(find.text('UNIRSE A GRUPO').first);
      await tester.pumpAndSettle();
      expect(find.text('Identificador del grupo'), findsOneWidget);
      expect(find.text('Código de invitación'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets('unirse llama POST join y refresca', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      var groups = [
        {
          'group_id': _gid,
          'name': 'Grupo Real',
          'description': 'INF',
          'role': 'member',
          'member_count': 1,
          'joined_at': '2026-08-11T12:00:00Z',
        },
      ];
      var joined = false;
      final before = SocialService.groupsChanged.value;
      final service = SocialService(
        client: MockClient((request) async {
          if (request.method == 'GET' && request.url.path == '/me/overview') {
            return _ok(_overview(groups));
          }
          if (request.url.path.endsWith('/members')) return _ok([]);
          if (request.method == 'POST' &&
              request.url.path == '/groups/$_gid/join') {
            final body = jsonDecode(request.body) as Map<String, dynamic>;
            expect(body['invite_token'], 'codigo-1');
            joined = true;
            groups = [
              ...groups,
              {
                'group_id': '22222222-2222-2222-2222-222222222222',
                'name': 'Grupo Nuevo',
                'description': 'INF',
                'role': 'member',
                'member_count': 2,
                'joined_at': '2026-09-29T12:00:00Z',
              },
            ];
            return http.Response('', 201);
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupsScreen(service: service),
        const Size(1440, 900),
      );

      await tester.tap(find.text('UNIRSE A GRUPO').first);
      await tester.pumpAndSettle();
      expect(find.text('UNIRSE A GRUPO'), findsWidgets);
      await tester.enterText(find.byType(TextField).at(0), _gid);
      await tester.enterText(find.byType(TextField).at(1), 'codigo-1');
      await tester.tap(find.text('Unirse').last);
      await tester.pumpAndSettle();

      expect(joined, isTrue);
      expect(find.text('Te uniste al grupo.'), findsOneWidget);
      expect(find.text('Grupo Nuevo'), findsOneWidget);
      expect(SocialService.groupsChanged.value, before + 1);
      expect(tester.takeException(), isNull);
    });

    testWidgets('unirse con vacíos muestra mensaje humano', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path == '/me/overview') {
            return _ok(_overview([]));
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(tester, GroupsScreen(service: service), const Size(390, 844));

      await tester.tap(find.text('UNIRSE A GRUPO').first);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Unirse').last);
      await tester.pumpAndSettle();
      expect(
        find.text('Completa el identificador y el código.'),
        findsOneWidget,
      );
      expect(tester.takeException(), isNull);
    });

    testWidgets('discord sin textos debug', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          final path = request.url.path;
          if (request.method == 'GET' && path == '/groups/$_gid') {
            return _ok({
              'id': _gid,
              'name': 'G',
              'owner_user_id': 'u',
              'notes_restricted_to_staff': false,
              'role': 'admin',
            });
          }
          if (request.method == 'GET' && path.endsWith('/discord-config')) {
            return _ok({
              'server_name': 'Servidor',
              'invite_url': 'https://discord.gg/x',
              'webhook_url': null,
            });
          }
          if (request.method == 'GET' && path.endsWith('/workspace')) {
            return _ok({
              'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {'sheets': [], 'tasks': []},
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            });
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupDetailScreen(
          group: {'id': _gid, 'name': 'G', 'role': 'admin'},
          service: service,
        ),
        const Size(1280, 900),
      );
      await tester.tap(find.text('DISCORD').first);
      await tester.pumpAndSettle();
      expect(find.textContaining('(real)'), findsNothing);
      expect(find.textContaining('(REAL)'), findsNothing);
      expect(find.text('ABRIR DISCORD'), findsOneWidget);
      expect(find.text('Configurar Discord'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets('sprint real sin caja duplicada ni Sin tareas', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path.endsWith('/workspace')) {
            return _ok({
              'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {'sheets': [], 'tasks': []},
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            });
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        SprintSheetScreen(groupId: _gid, service: service),
        const Size(1280, 900),
      );
      expect(find.text('Aún no hay tareas en este sprint.'), findsOneWidget);
      expect(find.text('Sin tareas por ahora.'), findsNothing);
      expect(find.text('NUEVA TAREA'), findsOneWidget);
      expect(find.text('RECARGAR'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets('kanban real sin caja TABLERO y con Recargar', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path.endsWith('/workspace')) {
            return _ok({
              'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {'sheets': [], 'tasks': []},
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            });
          }
          if (request.url.path.endsWith('/members')) return _ok([]);
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        KanbanScreen(groupId: _gid, service: service),
        const Size(1440, 900),
      );
      expect(find.text('TABLERO KANBAN'), findsOneWidget);
      expect(find.text('TABLERO'), findsNothing);
      expect(find.text('RECARGAR'), findsOneWidget);
      expect(find.text('POR HACER'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  });

  group('PRE-CHECKPOINT — Chat/Header/Sprint', () {
    StreamChatConnector connectorFor(Future<ChatSession> Function() open) {
      return StreamChatConnector(
        factory:
            ({
              required String apiKey,
              required String userToken,
              required String userId,
              required String channelId,
            }) => open(),
      );
    }

    testWidgets('chat muestra identidad humana y nunca UUID', (tester) async {
      final payload = base64Url.encode(
        utf8.encode(jsonEncode({'user_id': 'u1', 'exp': 9999999999})),
      );
      await SessionManager.saveSession('h.$payload.f', {'id': 'u1'});
      addTearDown(SessionManager.clear);

      final longId = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee';
      final session = _FakeChatSession(
        initial: [
          ChatMessage(
            id: 'm1',
            text: 'Hola de otro',
            authorId: longId,
            authorName: 'Ana Administradora',
            createdAt: DateTime(2026, 9, 29, 10, 5),
            isMine: false,
          ),
          ChatMessage(
            id: 'm2',
            text: 'Hola mío',
            authorId: 'u1',
            authorName: 'u1',
            createdAt: DateTime(2026, 9, 29, 10, 6),
            isMine: true,
          ),
        ],
      );
      final service = SocialService(
        client: MockClient((request) async {
          if (request.url.path.endsWith('/stream-token')) {
            return _ok({'token': 'tok', 'channel_id': 'c1', 'api_key': 'key'});
          }
          if (request.url.path.endsWith('/members')) {
            return _ok([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': longId,
                'role': 'member',
                'display_name': 'Ana Administradora',
              },
            ]);
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);

      tester.view.physicalSize = const Size(1280, 900);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          home: GroupChatScreen(
            groupId: _gid,
            service: service,
            connector: connectorFor(() async => session),
          ),
        ),
      );
      await tester.pumpAndSettle();

      // Identidad: ajeno con display_name, propio como "Tú", hora visible.
      expect(find.text('Ana Administradora'), findsOneWidget);
      expect(find.text('Tú'), findsOneWidget);
      expect(find.text('10:05'), findsOneWidget);
      expect(find.text('10:06'), findsOneWidget);
      expect(find.textContaining('aaaaaaaa'), findsNothing);
      expect(find.text('u1'), findsNothing);
      // Enviar sigue funcionando.
      await tester.enterText(find.byType(TextField), 'Enterado');
      await tester.tap(find.byIcon(Icons.send_rounded));
      await tester.pumpAndSettle();
      expect(session.sent, ['Enterado']);
      expect(tester.takeException(), isNull);
    });

    testWidgets('header muestra 1 integrante', (tester) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          if (request.method == 'GET' && request.url.path == '/groups/$_gid') {
            return _ok({
              'id': _gid,
              'name': 'Grupo Solo',
              'description': 'Desc',
              'owner_user_id': 'u',
              'notes_restricted_to_staff': false,
              'role': 'admin',
            });
          }
          if (request.url.path.endsWith('/members')) {
            return _ok([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': _memberId,
                'role': 'admin',
              },
            ]);
          }
          return _ok({
            'group': {'id': _gid, 'name': 'Grupo Solo', 'role': 'admin'},
            'kanban': {'todo': [], 'in_progress': [], 'done': []},
            'sprint_sheet': {'sheets': [], 'tasks': []},
            'meetings': {'upcoming': []},
            'chat': {'provider': 'stream', 'token_endpoint': ''},
          });
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        GroupDetailScreen(
          group: {'id': _gid, 'name': 'Grupo Solo', 'role': 'admin'},
          service: service,
        ),
        const Size(1280, 900),
      );
      expect(find.text('GRUPO SOLO'), findsOneWidget);
      expect(find.text('1 integrante'), findsOneWidget);
      expect(find.textContaining('Desc'), findsNothing);
      expect(tester.takeException(), isNull);
    });

    testWidgets('sprint real sin leyenda pero con status en filas', (
      tester,
    ) async {
      SessionManager.saveSession('jwt-test', {'id': 'u'});
      addTearDown(SessionManager.clear);
      final service = SocialService(
        client: MockClient((request) async {
          final path = request.url.path;
          if (request.method == 'GET' && path.endsWith('/workspace')) {
            return _ok({
              'group': {'id': _gid, 'name': 'G', 'role': 'admin'},
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {
                'sheets': [
                  {
                    'id': 'cccccccc-0000-4000-8000-000000000001',
                    'name': 'Sprint 1',
                  },
                ],
                'tasks': [
                  {
                    'id': 'dddddddd-0000-4000-8000-000000000001',
                    'group_id': _gid,
                    'sheet_id': 'cccccccc-0000-4000-8000-000000000001',
                    'sheet_name': 'Sprint 1',
                    'title': 'Tarea Lista',
                    'assigned_to': _memberId,
                    'priority': 'media',
                    'status': 'listo',
                    'estimated_hours': 1.0,
                  },
                ],
              },
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            });
          }
          if (path.endsWith('/members')) return _ok([]);
          if (path.contains('/hours')) {
            return _ok({'data': [], 'total_hours': 0});
          }
          return _ok({}, 404);
        }),
      );
      addTearDown(service.dispose);
      await _pump(
        tester,
        SprintSheetScreen(groupId: _gid, service: service),
        const Size(1280, 900),
      );
      // Leyenda redundante fuera; el estado sigue en la fila.
      expect(find.text('En proceso'), findsNothing);
      expect(find.text('Sin empezar'), findsNothing);
      expect(find.text('Pendiente'), findsNothing);
      expect(find.text('LISTO'), findsWidgets);
      expect(tester.takeException(), isNull);
    });
  });
}
