import 'dart:async';
import 'dart:convert';
import 'dart:io' show File, Platform, Process;

import 'package:drift/drift.dart' show Value;
import 'package:flutter/foundation.dart'
    show defaultTargetPlatform, kIsWeb, visibleForTesting;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:path_provider/path_provider.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/database/app_database.dart';
import '../../core/database/local_notes_repository.dart';
import '../../core/services/api_config.dart';
import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Base URL del microservicio de notas. En el emulador Android `localhost` es
/// el propio dispositivo, por eso se usa la alias 10.0.2.2 hacia el host.
String get notesBaseUrl {
  if (!kIsWeb && Platform.isAndroid) {
    return 'http://10.0.2.2:8082';
  }
  return notesApiBaseUrl;
}

/// Cliente HTTP inyectable para pruebas de subida/vinculación de adjuntos.
/// En producción se crea (y cierra) un cliente por petición.
@visibleForTesting
http.Client? notesHttpClientOverride;

/// Ejecuta [run] con el cliente inyectado o con uno nuevo que se cierra al
/// terminar, para no filtrar conexiones en la subida de adjuntos.
Future<T> _withNotesClient<T>(
  Future<T> Function(http.Client client) run,
) async {
  final client = notesHttpClientOverride ?? http.Client();
  try {
    return await run(client);
  } finally {
    if (notesHttpClientOverride == null) client.close();
  }
}

class AllNotesScreen extends StatefulWidget {
  const AllNotesScreen({super.key});

  @override
  State<AllNotesScreen> createState() => AllNotesScreenState();
}

class AllNotesScreenState extends State<AllNotesScreen> {
  AppDatabase? _db;
  LocalNotesRepository? _repo;

  final TextEditingController _searchController = TextEditingController();
  Timer? _debounce;

  late Future<List<LocalNote>> _notesFuture;
  List<LocalNote> _filtered = [];
  List<LocalNote> _memoryFallback = [];
  bool _isLoading = true;
  bool _isGridView = false;
  String _selectedFilter = 'all';
  String? _error;
  bool _useMemoryFallback = false;
  bool _isSyncingBackend = false;
  String? _backendError;
  bool _showingLocalExample = false;
  int _hiddenDemoCount = 0;
  String _currentQuery = '';

  // Identificación verificable de ejemplos sembrados (sin borrar nada).
  // Demo solo existe con id '1'..'7' Y título exacto de _demoNotes().
  // Las notas reales usan UUID (36 con guiones) y las locales del usuario
  // usan timestamp (13 dígitos): ninguna colisiona con este par id+título.
  static const Map<String, String> _kDemoTitles = <String, String>{
    '1': 'Cálculo - Límites y derivadas',
    '2': 'Estructuras de Datos - Árboles',
    '3': 'Bases de Datos - SQL Joins',
    '4': 'Redes - Modelo OSI',
    '5': 'Frontend - NestJS MVC',
    '6': 'Cálculo - Integrales dobles',
    '7': 'Nota con Adjuntos de Prueba (Conejita y PDF)',
  };

  bool _isKnownDemo(LocalNote n) => _kDemoTitles[n.id] == n.title;
  // Dueño para aislamiento por cuenta: user_id del JWT activo (solo scoping
  // local; el backend sigue siendo la autoridad). Null sin sesión.
  String? _ownerId() => SessionManager.currentUserId;
  // Sellado de dueño en notas que pertenecen a la cuenta activa.
  LocalNote _stampOwner(LocalNote n) => n.ownerUserId == _ownerId()
      ? n
      : n.copyWith(ownerUserId: Value(_ownerId()));
  // Ids de notas del backend que pertenecen al usuario (GET /notes/me + POST 201).
  // El modelo local no guarda autor: un id con formato UUID no listado aquí se
  // trata como ajeno (solo lectura); los ids locales (dígitos) son del dispositivo.
  final Set<String> _ownedNoteIds = {};

  // Estado interactivo por nota
  final Map<String, int> _likesCount = {};
  final Map<String, bool> _isLiked = {};
  final Map<String, bool> _isSaved = {};

  // Etiquetas demo para filtros
  final Map<String, String> _noteTags = {
    '1': 'Cálculo',
    '2': 'Redes',
    '3': 'Frontend',
    '4': 'Redes',
  };

  List<LocalNote> _demoNotes() {
    final now = DateTime.now();
    return [
      LocalNote(
        id: '1',
        title: 'Cálculo - Límites y derivadas',
        content:
            'Apuntes de cálculo diferencial: límites, continuidad y reglas de derivación. **Markdown** con fórmulas.\n\n- Definición epsilon-delta\n- Regla de la cadena',
        visibility: 'private',
        updatedAt: now,
      ),
      LocalNote(
        id: '2',
        title: 'Estructuras de Datos - Árboles',
        content:
            'Árboles binarios, AVL y recorridos preorden, inorden, postorden.\n\n```\n   1\n  / \\\n 2   3\n```',
        visibility: 'private',
        updatedAt: now.subtract(const Duration(hours: 2)),
      ),
      LocalNote(
        id: '3',
        title: 'Bases de Datos - SQL Joins',
        content:
            'Joins internos, externos, subconsultas y optimización con índices.\n\nSELECT * FROM users JOIN orders ON users.id = orders.user_id;',
        visibility: 'public',
        updatedAt: now.subtract(const Duration(days: 1)),
      ),
      LocalNote(
        id: '4',
        title: 'Redes - Modelo OSI',
        content:
            'Capas OSI y TCP/IP, encapsulamiento y protocolos.\n\n7 Aplicación\n6 Presentación\n5 Sesión',
        visibility: 'public',
        updatedAt: now.subtract(const Duration(days: 2)),
      ),
      LocalNote(
        id: '5',
        title: 'Frontend - NestJS MVC',
        content:
            'Patrón MVC en NestJS con controladores, servicios y módulos.\n\n```ts\n@Controller(\'notes\')\nexport class NotesController {}\n```',
        visibility: 'public',
        updatedAt: now.subtract(const Duration(days: 3)),
      ),
      LocalNote(
        id: '6',
        title: 'Cálculo - Integrales dobles',
        content:
            'Integrales dobles en coordenadas polares y cambio de variables.\n\n∫∫ f(x,y) dA',
        visibility: 'private',
        updatedAt: now.subtract(const Duration(days: 4)),
      ),
      LocalNote(
        id: '7',
        title: 'Nota con Adjuntos de Prueba (Conejita y PDF)',
        content:
            'Adjuntos probados desde el repo: imagen local y documento PDF.\n\n#### Verificación técnica H4\n\n![Conejita](conejita.jpg)\n\nDocumento adjunto: [Prueba PDF](prueba.pdf)',
        visibility: 'public',
        updatedAt: now.subtract(const Duration(hours: 3)),
      ),
    ];
  }

  @override
  void initState() {
    super.initState();
    _notesFuture = _initAndLoad();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    // Inicializar likes/saved para demo
    for (final n in _demoNotes()) {
      _likesCount.putIfAbsent(n.id, () => (int.tryParse(n.id) ?? 1) * 3);
      _isLiked.putIfAbsent(n.id, () => false);
      _isSaved.putIfAbsent(n.id, () => false);
    }
  }

  Future<List<LocalNote>> _initAndLoad() async {
    if (mounted) {
      setState(() {
        _isLoading = true;
        _error = null;
      });
    }
    try {
      try {
        _db = AppDatabase();
        _repo = LocalNotesRepository(_db!);
        await _repo!.getAllNotes(ownerId: _ownerId());
      } catch (e) {
        debugPrint('Drift init falló, fallback a memoria: $e');
        final token = SessionManager.token;
        final hasSession = token != null && token.isNotEmpty;
        if (hasSession) {
          // Con sesión no se siembran demos nuevas: error real.
          if (mounted) {
            setState(() {
              _error = 'No se pudo abrir la base local: $e';
              _backendError = _error;
              _isLoading = false;
            });
          }
          return _filtered;
        }
        _useMemoryFallback = true;
        _memoryFallback = _demoNotes();
        for (final n in _memoryFallback) {
          _likesCount[n.id] = (int.tryParse(n.id) ?? 1) * 2;
          _isLiked[n.id] = false;
          _isSaved[n.id] = false;
        }
        _filtered = _applyFilters(
          _memoryFallback,
          _currentQuery,
          _selectedFilter,
        );
        if (mounted) {
          setState(() {
            _isLoading = false;
            _showingLocalExample = true;
          });
        }
        return _filtered;
      }

      final existing = await _repo!.getAllNotes(ownerId: _ownerId());
      if (existing.isEmpty) {
        final token = SessionManager.token;
        final hasSession = token != null && token.isNotEmpty;
        if (hasSession) {
          // Con sesión no se siembran demos nuevas: se intenta backend y,
          // si falla, se muestra vacío/error real (nunca ejemplos).
          await _syncFromBackend();
        } else {
          final demo = _demoNotes();
          for (final n in demo) {
            await _repo!.upsertNote(n);
            _likesCount[n.id] = (int.tryParse(n.id) ?? 1) * 2;
          }
          await _syncFromBackend();
        }
      } else {
        for (final n in existing) {
          _likesCount.putIfAbsent(n.id, () => 0);
          _isLiked.putIfAbsent(n.id, () => false);
          _isSaved.putIfAbsent(n.id, () => false);
        }
        unawaited(_syncFromBackend().then((_) => _applySearch(_currentQuery)));
      }

      await _applySearch(_currentQuery);
      if (mounted) setState(() => _isLoading = false);
      return _filtered;
    } catch (e) {
      debugPrint('AllNotes init error: $e');
      if (mounted) {
        final token = SessionManager.token;
        final hasSession = token != null && token.isNotEmpty;
        setState(() {
          _isLoading = false;
          if (hasSession) {
            // Con sesión: error real, no se oculta tras demo.
            _error = 'No se pudo cargar tus notas: $e';
            _backendError = _error;
          } else if (_filtered.isEmpty && _memoryFallback.isEmpty) {
            _useMemoryFallback = true;
            _memoryFallback = _demoNotes();
            _filtered = _applyFilters(
              _memoryFallback,
              _currentQuery,
              _selectedFilter,
            );
            _error = null;
            _showingLocalExample = true;
          } else {
            _error = e.toString();
          }
        });
      }
      return _filtered;
    }
  }

  // Sincroniza GET /notes/me (real). Si hay sesión y falla, guarda
  // _backendError real en vez de ocultar el fallo tras datos demo.
  // Ver back/notes/cmd/server/main.go:130 y note_handler.go:241 ListMy.
  Future<void> _syncFromBackend() async {
    if (_useMemoryFallback || _repo == null) return;
    if (mounted) {
      setState(() {
        _isSyncingBackend = true;
        _backendError = null;
      });
    }
    try {
      final token = SessionManager.token;
      if (token == null || token.isEmpty) {
        // Sin sesión (tests/modo local): no se exige backend. Se marca como
        // ejemplo local para no presentar _demoNotes como notas del usuario.
        if (mounted) setState(() => _showingLocalExample = true);
        return;
      }
      final res = await AuthedHttp.run(
        () => _withNotesClient(
          (client) => client
              .get(
                Uri.parse('$notesBaseUrl/notes/me?limit=50'),
                headers: _authHeaders(),
              )
              .timeout(const Duration(seconds: 5)),
        ),
      );
      if (res.statusCode == 200) {
        if (mounted) {
          setState(() {
            _backendError = null;
            _showingLocalExample = false;
          });
        }
        final data = jsonDecode(utf8.decode(res.bodyBytes));
        final List notes = data['notes'] ?? [];
        for (final n in notes) {
          try {
            final remoteId = n['id'] as String?;
            if (remoteId != null && remoteId.isNotEmpty) {
              _ownedNoteIds.add(remoteId);
            }
            await _repo!.upsertNote(
              _stampOwner(
                LocalNote(
                  id: n['id'] as String,
                  title: n['title'] as String? ?? 'Sin título',
                  content: n['content'] as String? ?? '',
                  visibility: n['visibility'] as String? ?? 'private',
                  updatedAt: n['updated_at'] != null
                      ? DateTime.tryParse(n['updated_at'].toString()) ??
                            DateTime.now()
                      : DateTime.now(),
                  ownerUserId: _ownerId(),
                ),
              ),
            );
          } catch (_) {}
        }
      } else {
        String detail = 'HTTP ${res.statusCode}';
        try {
          final body = jsonDecode(utf8.decode(res.bodyBytes));
          if (body is Map && body['error'] is Map) {
            detail =
                '${body['error']['code'] ?? 'error'}: ${body['error']['message'] ?? detail}';
          }
        } catch (_) {}
        if (mounted) {
          setState(
            () => _backendError =
                'No pudimos sincronizar tus notas. Puedes seguir usando las notas guardadas en este equipo.',
          );
        }
        debugPrint('Backend sync error real: $detail');
      }
    } catch (_) {
      debugPrint('Backend sync falló (offline)');
      if (mounted) {
        final token = SessionManager.token;
        if (token != null && token.isNotEmpty) {
          setState(
            () => _backendError =
                'No pudimos sincronizar tus notas. Puedes seguir usando las notas guardadas en este equipo.',
          );
        } else {
          setState(() => _showingLocalExample = true);
        }
      }
    } finally {
      if (mounted) setState(() => _isSyncingBackend = false);
    }
  }

  /// Intento de creación remota. Devuelve el UUID real del backend (201) o null.
  /// No bloquea el guardado local: si el backend falla, solo se imprime el error (offline-first).
  Future<String?> _createNoteOnBackend({
    required String title,
    required String content,
    required String visibility,
  }) async {
    final url = '$notesBaseUrl/notes';
    final body = <String, String>{
      'title': title,
      'content': content,
      'visibility': visibility,
    };
    debugPrint('[FRONT DEBUG] Enviando POST /notes a $url con body: $body');
    try {
      final res = await AuthedHttp.run(
        () => http
            .post(
              Uri.parse(url),
              headers: _authHeaders(),
              body: jsonEncode(body),
            )
            .timeout(const Duration(seconds: 8)),
      );
      debugPrint(
        '[FRONT DEBUG] Respuesta POST /notes: status=${res.statusCode} body=${utf8.decode(res.bodyBytes)}',
      );
      if (res.statusCode == 201) {
        try {
          final data = jsonDecode(utf8.decode(res.bodyBytes));
          final newId = data['note_id'] as String?;
          if (newId != null && newId.isNotEmpty) {
            _ownedNoteIds.add(newId);
            debugPrint('[FRONT DEBUG] UUID real del backend: $newId');
            return newId;
          }
        } catch (_) {}
      }
    } catch (e) {
      debugPrint('[FRONT DEBUG] Error POST /notes a $url: $e');
    }
    return null;
  }

  /// Reemplaza el id temporal (timestamp) por el UUID real del backend en
  /// Drift/memoria y migra el estado interactivo (likes/saved/tags) para que
  /// PATCH/DELETE usen el UUID válido de Postgres.
  Future<void> _replaceLocalId(String tempId, String realId) async {
    if (!mounted) return;
    _ownedNoteIds.add(realId);
    LocalNote? current;
    if (_useMemoryFallback || _repo == null) {
      final i = _memoryFallback.indexWhere((n) => n.id == tempId);
      if (i < 0) return;
      current = _memoryFallback[i];
    } else {
      try {
        final all = await _repo!.getAllNotes(ownerId: _ownerId());
        for (final n in all) {
          if (n.id == tempId) {
            current = n;
            break;
          }
        }
      } catch (_) {}
      if (current == null) return;
    }
    final renamed = LocalNote(
      id: realId,
      title: current.title,
      content: current.content,
      visibility: current.visibility,
      updatedAt: current.updatedAt,
      ownerUserId: current.ownerUserId,
    );
    if (!mounted) return;
    setState(() {
      if (_likesCount.containsKey(tempId)) {
        _likesCount[realId] = _likesCount.remove(tempId)!;
      }
      if (_isLiked.containsKey(tempId)) {
        _isLiked[realId] = _isLiked.remove(tempId)!;
      }
      if (_isSaved.containsKey(tempId)) {
        _isSaved[realId] = _isSaved.remove(tempId)!;
      }
      if (_noteTags.containsKey(tempId)) {
        _noteTags[realId] = _noteTags.remove(tempId)!;
      }
    });
    if (_useMemoryFallback || _repo == null) {
      if (!mounted) return;
      setState(() {
        final i = _memoryFallback.indexWhere((n) => n.id == tempId);
        if (i >= 0) {
          _memoryFallback[i] = renamed;
        } else {
          _memoryFallback.insert(0, renamed);
        }
      });
    } else {
      try {
        await _repo!.deleteNote(tempId);
        await _repo!.upsertNote(renamed);
      } catch (_) {}
    }
    await _applySearch(_currentQuery);
  }

  Map<String, String> _authHeaders() {
    final headers = <String, String>{'Content-Type': 'application/json'};
    final token = SessionManager.token;
    if (token != null && token.isNotEmpty) {
      headers['Authorization'] = 'Bearer $token';
    }
    return headers;
  }

  /// Registra contra la nota recién creada los adjuntos que ya viven en Drive:
  /// POST /notes/{id}/attachments con el external_file_id devuelto por
  /// /notes/upload. Best-effort: si falla, la URL de Drive ya quedó incrustada
  /// en el Markdown y el guardado local no se revierte.
  Future<void> _linkAttachmentsToNote(
    String noteId,
    List<_DriveUploadResult> attachments,
  ) async {
    if (attachments.isEmpty) return;
    for (final att in attachments) {
      final externalId = att.externalFileId;
      if (externalId == null || externalId.isEmpty) continue;
      try {
        final res = await AuthedHttp.run(
          () => _withNotesClient(
            (client) => client
                .post(
                  Uri.parse('$notesBaseUrl/notes/$noteId/attachments'),
                  headers: _authHeaders(),
                  body: jsonEncode(<String, dynamic>{
                    'external_file_id': externalId,
                    'file_name': att.fileName,
                    'file_type': att.fileType,
                    'file_size_bytes': att.fileSizeBytes,
                    'is_inline': att.isInline,
                  }),
                )
                .timeout(const Duration(seconds: 8)),
          ),
        );
        if (res.statusCode != 201) {
          debugPrint(
            '[FRONT DEBUG] No se pudo vincular adjunto $externalId: '
            'status=${res.statusCode} body=${utf8.decode(res.bodyBytes)}',
          );
        }
      } catch (e) {
        debugPrint('[FRONT DEBUG] Error vinculando adjunto $externalId: $e');
      }
    }
  }

  /// Lectura híbrida: GET /notes/:id. 200 trae content/title del Drive del autor.
  Future<_RemoteResult> _fetchHybridContent(LocalNote note) async {
    final url = '$notesBaseUrl/notes/${note.id}';
    try {
      final res = await AuthedHttp.run(
        () => http
            .get(Uri.parse(url), headers: _authHeaders())
            .timeout(const Duration(seconds: 8)),
      );
      if (res.statusCode == 200) {
        final data =
            jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
        return _RemoteResult(ok: true, status: 200, data: data);
      }
      final parsed = _parseBackendError(utf8.decode(res.bodyBytes));
      return _RemoteResult(
        ok: false,
        status: res.statusCode,
        code: parsed['code'],
        message: parsed['message'],
      );
    } catch (e) {
      return _RemoteResult(ok: false, status: -1, message: e.toString());
    }
  }

  /// Edición sincronizada: PATCH /notes/:id con {title, content}.
  Future<_RemoteResult> _patchNoteRemote(
    LocalNote note,
    String title,
    String content,
  ) async {
    final url = '$notesBaseUrl/notes/${note.id}';
    final body = {'title': title, 'content': content};
    try {
      final res = await AuthedHttp.run(
        () => http
            .patch(
              Uri.parse(url),
              headers: _authHeaders(),
              body: jsonEncode(body),
            )
            .timeout(const Duration(seconds: 8)),
      );
      if (res.statusCode == 200) {
        return const _RemoteResult(ok: true, status: 200);
      }
      final parsed = _parseBackendError(utf8.decode(res.bodyBytes));
      return _RemoteResult(
        ok: false,
        status: res.statusCode,
        code: parsed['code'],
        message: parsed['message'],
      );
    } catch (e) {
      return _RemoteResult(ok: false, status: -1, message: e.toString());
    }
  }

  /// Borrado en cascada: DELETE /notes/:id. Éxito con 200 o 204.
  Future<_RemoteResult> _deleteNoteRemote(LocalNote note) async {
    final url = '$notesBaseUrl/notes/${note.id}';
    try {
      final res = await AuthedHttp.run(
        () => http
            .delete(Uri.parse(url), headers: _authHeaders())
            .timeout(const Duration(seconds: 8)),
      );
      if (res.statusCode == 200 || res.statusCode == 204) {
        return _RemoteResult(ok: true, status: res.statusCode);
      }
      final parsed = _parseBackendError(utf8.decode(res.bodyBytes));
      return _RemoteResult(
        ok: false,
        status: res.statusCode,
        code: parsed['code'],
        message: parsed['message'],
      );
    } catch (e) {
      return _RemoteResult(ok: false, status: -1, message: e.toString());
    }
  }

  /// ¿Puede el usuario actual editar/borrar esta nota? El backend es quien lo
  /// impone (403 si no es autor); aquí se decide visibilidad de los botones.
  bool _isMine(LocalNote note) {
    if (_ownedNoteIds.contains(note.id)) return true;
    return !note.id.contains('-');
  }

  /// true si el resultado indica nota no disponible en Drive (404/403 Drive,
  /// code note_unavailable o mensaje "no disponible").
  bool _isDriveUnavailable(_RemoteResult r) {
    if (r.code == 'note_unavailable') return true;
    if ((r.message ?? '').contains('no disponible')) return true;
    return r.status == 404;
  }

  Future<void> _applyLocalUpsert(LocalNote note) async {
    if (!mounted) return;
    if (_useMemoryFallback || _repo == null) {
      final i = _memoryFallback.indexWhere((n) => n.id == note.id);
      setState(() {
        if (i >= 0) {
          _memoryFallback[i] = note;
        } else {
          _memoryFallback.insert(0, note);
        }
      });
    } else {
      try {
        await _repo!.upsertNote(_stampOwner(note));
      } catch (_) {}
    }
    await _applySearch(_currentQuery);
  }

  Future<void> _applyLocalDelete(LocalNote note) async {
    if (!mounted) return;
    setState(() {
      _likesCount.remove(note.id);
      _isLiked.remove(note.id);
      _isSaved.remove(note.id);
      _memoryFallback.removeWhere((n) => n.id == note.id);
    });
    if (!_useMemoryFallback && _repo != null) {
      try {
        await _repo!.deleteNote(note.id);
      } catch (_) {}
    }
    await _applySearch(_currentQuery);
  }

  /// Guarda edición: solo con 200 actualiza local y reporta sincronizado.
  Future<_EditSaveOutcome> _saveEditFlow(
    LocalNote note,
    String title,
    String content,
  ) async {
    final remote = await _patchNoteRemote(note, title, content);
    if (remote.ok) {
      await _applyLocalUpsert(
        LocalNote(
          id: note.id,
          title: title,
          content: content,
          visibility: note.visibility,
          updatedAt: DateTime.now(),
        ),
      );
      if (mounted) setState(() {});
      return _EditSaveOutcome.synced;
    }
    if (remote.status == -1) return _EditSaveOutcome.offline;
    if (remote.status == 403 && remote.code == 'forbidden') {
      return _EditSaveOutcome.forbidden;
    }
    if (_isDriveUnavailable(remote)) return _EditSaveOutcome.unavailable;
    return _EditSaveOutcome.error;
  }

  /// Clona una nota ajena: POST /notes/:id/copy. Con 201 registra el nuevo id
  /// en _ownedNoteIds y refresca la lista local con la copia.
  Future<_RemoteResult> _cloneFlow(LocalNote note) async {
    final url = '$notesBaseUrl/notes/${note.id}/copy';
    try {
      final res = await AuthedHttp.run(
        () => http
            .post(Uri.parse(url), headers: _authHeaders())
            .timeout(const Duration(seconds: 8)),
      );
      if (res.statusCode == 201) {
        try {
          final data = jsonDecode(utf8.decode(res.bodyBytes));
          final newId = data['note_id'] as String?;
          if (newId != null && newId.isNotEmpty) _ownedNoteIds.add(newId);
        } catch (_) {}
        if (!_useMemoryFallback && _repo != null) {
          await _syncFromBackend();
        }
        await _applySearch(_currentQuery);
        if (mounted) setState(() {});
        return _RemoteResult(ok: true, status: res.statusCode);
      }
      final parsed = _parseBackendError(utf8.decode(res.bodyBytes));
      return _RemoteResult(
        ok: false,
        status: res.statusCode,
        code: parsed['code'],
        message: parsed['message'],
      );
    } catch (e) {
      return _RemoteResult(ok: false, status: -1, message: e.toString());
    }
  }

  /// Borra en backend y, con 200/204, elimina local. Con 404 (el recurso ya no
  /// existe en el backend) también limpia local para no dejar la referencia bloqueada.
  Future<_RemoteResult> _deleteFlow(LocalNote note) async {
    final remote = await _deleteNoteRemote(note);
    if (!mounted) return remote;
    if (remote.ok) {
      await _applyLocalDelete(note);
      if (!mounted) return remote;
      setState(() {});
      return remote;
    }
    if (remote.status == 404) {
      await _applyLocalDelete(note);
      if (!mounted) return const _RemoteResult(ok: true, status: 404);
      setState(() {});
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          backgroundColor: Colors.black,
          content: Text(
            'La nota ya no existía en el servidor; se eliminó la copia local.',
            style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800),
          ),
        ),
      );
      return const _RemoteResult(ok: true, status: 404);
    }
    return remote;
  }

  void _onSearchChanged(String query) {
    _currentQuery = query;
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), () {
      _applySearch(query);
    });
  }

  Future<void> _applySearch(String query) async {
    try {
      List<LocalNote> base;
      if (_useMemoryFallback) {
        base = List.from(_memoryFallback);
      } else {
        base = await _repo!.searchNotesFts(query, ownerId: _ownerId());
        // Si la búsqueda FTS devuelve vacío con query vacío, usar getAll
        if (query.trim().isEmpty) {
          base = await _repo!.getAllNotes(ownerId: _ownerId());
        }
      }
      final filtered = _applyFilters(base, query, _selectedFilter);
      if (mounted) setState(() => _filtered = filtered);
    } catch (e) {
      debugPrint('Search error: $e');
      if (mounted) setState(() => _filtered = []);
    }
  }

  List<LocalNote> _applyFilters(
    List<LocalNote> notes,
    String query,
    String filter,
  ) {
    var res = notes;
    // Con sesión y fallo de backend, se ocultan (sin borrar) solo los
    // ejemplos verificables id+título. Nunca se presentan como reales.
    final token = SessionManager.token;
    final hasSession = token != null && token.isNotEmpty;
    if (hasSession && _backendError != null && res.isNotEmpty) {
      final before = res.length;
      res = res.where((n) => !_isKnownDemo(n)).toList();
      _hiddenDemoCount = before - res.length;
    } else {
      _hiddenDemoCount = 0;
    }
    // Filtro de chips
    if (filter == 'public') {
      res = res.where((n) => n.visibility == 'public').toList();
    } else if (filter == 'private') {
      res = res.where((n) => n.visibility == 'private').toList();
    } else if (filter == 'recent') {
      res = List.from(res)..sort((a, b) => b.updatedAt.compareTo(a.updatedAt));
      res = res.take(3).toList();
    }
    // El filtrado por texto ya lo hace FTS/LIKE, pero reforzamos para memoria y etiquetas
    if (query.trim().isNotEmpty && _useMemoryFallback) {
      final lower = query.toLowerCase();
      res = res.where((n) {
        final tag = (_noteTags[n.id] ?? '').toLowerCase();
        return n.title.toLowerCase().contains(lower) ||
            n.content.toLowerCase().contains(lower) ||
            tag.contains(lower);
      }).toList();
    }
    return res;
  }

  Future<void> _retry() async {
    _searchController.clear();
    _currentQuery = '';
    _selectedFilter = 'all';
    _notesFuture = _initAndLoad();
    await _notesFuture;
  }

  void _toggleLike(LocalNote note) {
    setState(() {
      final liked = _isLiked[note.id] ?? false;
      final count = _likesCount[note.id] ?? 0;
      if (liked) {
        _isLiked[note.id] = false;
        _likesCount[note.id] = (count - 1).clamp(0, 9999);
      } else {
        _isLiked[note.id] = true;
        _likesCount[note.id] = count + 1;
      }
    });
  }

  void _toggleSave(LocalNote note) {
    setState(() {
      final saved = _isSaved[note.id] ?? false;
      _isSaved[note.id] = !saved;
    });
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: Colors.black,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.zero,
          side: const BorderSide(color: Colors.black, width: 2),
        ),
        content: Text(
          (_isSaved[note.id] ?? false)
              ? 'Nota guardada localmente'
              : 'Nota removida de guardados',
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w800,
          ),
        ),
        duration: const Duration(seconds: 1),
      ),
    );
  }

  void _shareNote(LocalNote note) {
    final link = 'https://sigmastudy.app/notes/${note.id}';
    showModalBottomSheet(
      context: context,
      backgroundColor: Colors.transparent,
      builder: (_) => Container(
        decoration: BoxDecoration(
          color: Colors.white,
          border: Border.all(color: Colors.black, width: 2),
          borderRadius: const BorderRadius.vertical(top: Radius.circular(0)),
          boxShadow: const [
            BoxShadow(color: Colors.black, offset: Offset(4, 4)),
          ],
        ),
        padding: const EdgeInsets.all(20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Center(child: Container(width: 40, height: 4, color: Colors.black)),
            const SizedBox(height: 16),
            const Text(
              'COMPARTIR NOTA',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 14,
                color: Colors.black,
              ),
            ),
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: const Color(0xFFF5F0E8),
                border: Border.all(color: Colors.black, width: 2),
              ),
              child: Row(
                children: [
                  const Icon(Icons.link_rounded, size: 18, color: Colors.black),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      link,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 12,
                        color: Colors.black,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                style: ElevatedButton.styleFrom(
                  backgroundColor: const Color(0xFFFFCC00),
                  foregroundColor: Colors.black,
                  shape: const RoundedRectangleBorder(
                    borderRadius: BorderRadius.zero,
                  ),
                  side: const BorderSide(color: Colors.black, width: 2),
                  padding: const EdgeInsets.symmetric(vertical: 12),
                ),
                onPressed: () {
                  Clipboard.setData(ClipboardData(text: link));
                  Navigator.pop(context);
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(
                      content: Text(
                        'Enlace copiado al portapapeles',
                        style: TextStyle(
                          color: Colors.white,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                      backgroundColor: Colors.black,
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.zero,
                      ),
                    ),
                  );
                },
                child: const Text(
                  'COPIAR AL PORTAPAPELES',
                  style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  List<Map<String, dynamic>> _mockResourcesFor(String id) {
    const mocks = <String, List<Map<String, dynamic>>>{
      '1': [
        {
          'type': 'pdf',
          'name': 'Guía_Derivadas_2026.pdf',
          'url': 'prueba.pdf',
          'size': '1.2 MB',
        },
      ],
      '2': [
        {
          'type': 'doc',
          'name': 'CheatSheet_Bash.pdf',
          'url': 'prueba.pdf',
          'size': '450 KB',
        },
      ],
      '7': [
        {
          'type': 'image',
          'name': 'Conejita Adjunta',
          'url': 'conejita.jpg',
          'size': '104 KB',
        },
        {
          'type': 'pdf',
          'name': 'Prueba PDF Adjunto.pdf',
          'url': 'prueba.pdf',
          'size': '124 KB',
        },
      ],
    };
    return mocks[id] ?? const <Map<String, dynamic>>[];
  }

  void _openNoteViewer(LocalNote note) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      backgroundColor: Colors.transparent,
      // En desktop/web el modal por defecto se limita a 640px; sin tope la
      // hoja de detalle ocupa todo el ancho disponible junto al sidebar.
      constraints: const BoxConstraints(maxWidth: double.infinity),
      builder: (_) => DraggableScrollableSheet(
        initialChildSize: 0.95,
        minChildSize: 0.5,
        maxChildSize: 1.0,
        expand: false,
        builder: (context, scroll) => _NoteDetailSheet(
          scrollController: scroll,
          note: note,
          isMine: _isMine(note),
          tag: _noteTags[note.id] ?? 'General',
          likesCount: _likesCount[note.id] ?? 0,
          isLiked: _isLiked[note.id] ?? false,
          isSaved: _isSaved[note.id] ?? false,
          resources: _mockResourcesFor(note.id),
          onToggleLike: () => _toggleLike(note),
          onToggleSave: () => _toggleSave(note),
          onShare: () => _shareNote(note),
          onFetchRemote: () => _fetchHybridContent(note),
          onSaveEdit: (title, content) => _saveEditFlow(note, title, content),
          onDelete: () => _deleteFlow(note),
          onClone: () => _cloneFlow(note),
        ),
      ),
    );
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _searchController.dispose();
    _db?.close();
    super.dispose();
  }

  String get _shortcutLabel =>
      defaultTargetPlatform == TargetPlatform.macOS ? '⌘N' : 'Ctrl+N';

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > 1024;
    return CallbackShortcuts(
      bindings: <ShortcutActivator, VoidCallback>{
        const SingleActivator(LogicalKeyboardKey.keyN, control: true):
            _showCreateDialog,
        const SingleActivator(LogicalKeyboardKey.keyN, meta: true):
            _showCreateDialog,
      },
      child: Focus(
        autofocus: true,
        child: Scaffold(
          backgroundColor: AppColors.bg,
          body: SafeArea(
            child: Padding(
              padding: EdgeInsets.symmetric(
                horizontal: isDesktop ? 32 : 16,
                vertical: 16,
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  _buildHeader(isDesktop),
                  const SizedBox(height: 16),
                  _buildSearchBar(),
                  if (_isSyncingBackend) ...<Widget>[
                    const SizedBox(height: 8),
                    const LinearProgressIndicator(
                      color: AppColors.border,
                      backgroundColor: AppColors.bg,
                    ),
                  ],
                  if (_backendError != null) ...<Widget>[
                    const SizedBox(height: 8),
                    Container(
                      padding: const EdgeInsets.all(10),
                      decoration: BoxDecoration(
                        color: const Color(0xFFFDE8E8),
                        border: Border.all(color: Colors.black, width: 2),
                      ),
                      child: Text(
                        _hiddenDemoCount > 0
                            ? '${_backendError!} Se ocultaron $_hiddenDemoCount ejemplos locales; no se muestran como reales.'
                            : _backendError!,
                        style: const TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w700,
                          color: Colors.black,
                        ),
                      ),
                    ),
                  ],
                  if (_backendError == null &&
                      _showingLocalExample) ...<Widget>[
                    const SizedBox(height: 8),
                    Container(
                      padding: const EdgeInsets.all(10),
                      decoration: BoxDecoration(
                        color: const Color(0xFFFFF3CD),
                        border: Border.all(color: Colors.black, width: 2),
                      ),
                      child: const Text(
                        'Modo local sin sesión: se muestran datos de ejemplo. Inicia sesión para sincronizar tus notas.',
                        style: TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w700,
                          color: Colors.black,
                        ),
                      ),
                    ),
                  ],
                  const SizedBox(height: 12),
                  _buildFilterChips(),
                  const SizedBox(height: 16),
                  Expanded(child: _buildBody(isDesktop)),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  /// Punto de entrada público para que el shell (sidebar, rail, FAB o atajo
  /// Ctrl+N) abra el diálogo de creación de nota.
  void openCreateDialog() => _showCreateDialog();

  void _showCreateDialog() {
    showDialog<void>(
      context: context,
      builder: (_) => _CreateNoteDialog(
        onCreate: (title, content, visibility, attachments) async {
          final note = LocalNote(
            id: DateTime.now().millisecondsSinceEpoch.toString(),
            title: title,
            content: content,
            visibility: visibility,
            updatedAt: DateTime.now(),
            ownerUserId: _ownerId(),
          );
          _noteTags[note.id] = visibility == 'public' ? 'General' : 'Privado';
          final tempId = note.id;
          if (_useMemoryFallback) {
            setState(() {
              _memoryFallback.insert(0, note);
              _likesCount[note.id] = 0;
              _isLiked[note.id] = false;
              _isSaved[note.id] = false;
            });
            _applySearch(_currentQuery);
          } else {
            try {
              await _repo!.upsertNote(note);
              _likesCount[note.id] = 0;
              _isLiked[note.id] = false;
              _isSaved[note.id] = false;
              await _applySearch(_currentQuery);
            } catch (e) {
              setState(() {
                _useMemoryFallback = true;
                _memoryFallback = [note, ..._memoryFallback];
                _likesCount[note.id] = 0;
              });
              _applySearch(_currentQuery);
            }
          }
          unawaited(
            _createNoteOnBackend(
              title: note.title,
              content: note.content,
              visibility: note.visibility,
            ).then((realId) async {
              if (realId != null && realId.isNotEmpty) {
                await _replaceLocalId(tempId, realId);
                await _linkAttachmentsToNote(realId, attachments);
              }
            }),
          );
        },
      ),
    );
  }

  Widget _buildHeader(bool isDesktop) {
    return Row(
      children: <Widget>[
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                'TODAS LAS NOTAS',
                style: TextStyle(
                  fontSize: isDesktop ? 26 : 22,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                  letterSpacing: -0.5,
                ),
              ),
              const SizedBox(height: 4),
              Row(
                children: <Widget>[
                  const Expanded(
                    child: Text(
                      'Busca y organiza tus apuntes',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 12.5,
                        fontWeight: FontWeight.w700,
                        color: AppColors.muted,
                      ),
                    ),
                  ),
                  if (_useMemoryFallback) ...<Widget>[
                    const SizedBox(width: AppDimens.spaceSm),
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 6,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: AppColors.accentYellow,
                        border: Border.all(color: AppColors.border, width: 1.5),
                        borderRadius: BorderRadius.circular(
                          AppDimens.radiusChip,
                        ),
                        boxShadow: AppShadows.badge,
                      ),
                      child: const Text(
                        'MODO OFFLINE',
                        style: TextStyle(
                          fontSize: 9,
                          fontWeight: FontWeight.w900,
                          color: AppColors.text,
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ],
          ),
        ),
        // Creación contextual en desktop: botón con atajo visible junto a los
        // controles de vista. En mobile lo aporta el FAB del shell (solo en
        // la pestaña de Notas).
        if (isDesktop) ...<Widget>[
          NeobrutalistButton(
            label: 'Nueva nota',
            icon: Icons.add_rounded,
            trailing: NeobrutalistBadge(label: _shortcutLabel, compact: true),
            variant: NeobrutalistButtonVariant.accent,
            borderWidth: AppDimens.borderWidthAction,
            onPressed: _showCreateDialog,
          ),
          const SizedBox(width: AppDimens.spaceMd),
        ],
        NeobrutalistIconButton(
          icon: _isGridView ? Icons.view_list_rounded : Icons.grid_view_rounded,
          tooltip: _isGridView ? 'Vista lista' : 'Vista grilla',
          onPressed: () => setState(() => _isGridView = !_isGridView),
        ),
      ],
    );
  }

  Widget _buildSearchBar() {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.badge,
      ),
      child: TextField(
        controller: _searchController,
        onChanged: _onSearchChanged,
        cursorColor: AppColors.text,
        decoration: InputDecoration(
          hintText:
              'Buscar por título o contenido... (ej: "calculo*", "joins")',
          hintStyle: const TextStyle(
            color: AppColors.muted,
            fontWeight: FontWeight.w600,
            fontSize: 13,
          ),
          prefixIcon: const Icon(Icons.search_rounded, color: AppColors.border),
          suffixIcon: ValueListenableBuilder<TextEditingValue>(
            valueListenable: _searchController,
            builder: (context, value, child) {
              if (value.text.isEmpty) return const SizedBox.shrink();
              return IconButton(
                icon: const Icon(Icons.clear_rounded, color: AppColors.muted),
                onPressed: () {
                  _searchController.clear();
                  _onSearchChanged('');
                },
              );
            },
          ),
          border: InputBorder.none,
          contentPadding: const EdgeInsets.symmetric(
            horizontal: 14,
            vertical: 14,
          ),
        ),
        style: const TextStyle(
          fontWeight: FontWeight.w700,
          fontSize: 14,
          color: AppColors.text,
        ),
      ),
    );
  }

  Widget _buildFilterChips() {
    final chips = [
      {'label': 'Todas', 'value': 'all'},
      {'label': 'Públicas', 'value': 'public'},
      {'label': 'Privadas', 'value': 'private'},
      {'label': 'Recientes', 'value': 'recent'},
    ];
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        children: chips.map((c) {
          final selected = _selectedFilter == c['value'];
          return Padding(
            padding: const EdgeInsets.only(right: 8),
            child: GestureDetector(
              onTap: () {
                setState(() => _selectedFilter = c['value']!);
                _applySearch(_currentQuery);
              },
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 8,
                ),
                decoration: BoxDecoration(
                  color: selected ? AppColors.accentYellow : AppColors.surface,
                  border: Border.all(
                    color: AppColors.border,
                    width: AppDimens.borderWidth,
                  ),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: selected ? AppShadows.badge : null,
                ),
                child: Text(
                  c['label']!,
                  style: const TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 11,
                    color: AppColors.text,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
            ),
          );
        }).toList(),
      ),
    );
  }

  Widget _buildBody(bool isDesktop) {
    if (_isLoading) {
      return const Center(
        child: CircularProgressIndicator(color: Colors.black),
      );
    }
    if (_error != null) {
      return Center(
        child: Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: Colors.white,
            border: Border.all(color: Colors.black, width: 2),
            boxShadow: const [
              BoxShadow(
                color: Colors.black,
                offset: Offset(4, 4),
                blurRadius: 0,
              ),
            ],
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.error_outline_rounded,
                size: 36,
                color: Color(0xFFE63B2E),
              ),
              const SizedBox(height: 12),
              Text(
                _error!,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontWeight: FontWeight.w700,
                  color: Colors.black,
                  fontSize: 13,
                ),
              ),
              const SizedBox(height: 14),
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  style: ElevatedButton.styleFrom(
                    backgroundColor: Colors.black,
                    foregroundColor: Colors.white,
                    shape: const RoundedRectangleBorder(
                      borderRadius: BorderRadius.zero,
                    ),
                  ),
                  onPressed: _retry,
                  child: const Text(
                    'REINTENTAR',
                    style: TextStyle(fontWeight: FontWeight.w900),
                  ),
                ),
              ),
            ],
          ),
        ),
      );
    }
    if (_filtered.isEmpty) {
      final isSearching =
          _searchController.text.trim().isNotEmpty || _selectedFilter != 'all';
      if (isSearching) {
        return LayoutBuilder(
          builder: (context, constraints) => SingleChildScrollView(
            child: ConstrainedBox(
              constraints: BoxConstraints(minHeight: constraints.maxHeight),
              child: Center(
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 24),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        padding: const EdgeInsets.all(18),
                        decoration: BoxDecoration(
                          color: Colors.white,
                          border: Border.all(color: Colors.black, width: 2),
                          boxShadow: const [
                            BoxShadow(
                              color: Colors.black,
                              offset: Offset(3, 3),
                              blurRadius: 0,
                            ),
                          ],
                        ),
                        child: const Icon(
                          Icons.search_off_rounded,
                          size: 36,
                          color: Color(0xFF555555),
                        ),
                      ),
                      const SizedBox(height: 14),
                      const Text(
                        'SIN RESULTADOS',
                        style: TextStyle(
                          fontWeight: FontWeight.w900,
                          color: Colors.black,
                        ),
                      ),
                      const SizedBox(height: 6),
                      const Text(
                        'Prueba con otra palabra clave o prefijo (ej: prog*)',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontWeight: FontWeight.w600,
                          color: Color(0xFF555555),
                          fontSize: 12,
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
      return LayoutBuilder(
        builder: (context, constraints) => SingleChildScrollView(
          child: ConstrainedBox(
            constraints: BoxConstraints(minHeight: constraints.maxHeight),
            child: Center(
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Container(
                      padding: const EdgeInsets.all(20),
                      decoration: BoxDecoration(
                        color: Colors.white,
                        border: Border.all(color: Colors.black, width: 2),
                        boxShadow: const [
                          BoxShadow(
                            color: Colors.black,
                            offset: Offset(3, 3),
                            blurRadius: 0,
                          ),
                        ],
                      ),
                      child: const Icon(
                        Icons.note_add_rounded,
                        size: 36,
                        color: Colors.black,
                      ),
                    ),
                    const SizedBox(height: 14),
                    const Text(
                      'NO TIENES NOTAS CREADAS TODAVÍA',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontWeight: FontWeight.w900,
                        color: Colors.black,
                        fontSize: 14,
                      ),
                    ),
                    const SizedBox(height: 6),
                    const Text(
                      'Crea tu primera nota y aparecerá aquí',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontWeight: FontWeight.w600,
                        color: Color(0xFF555555),
                        fontSize: 12,
                      ),
                    ),
                    const SizedBox(height: 16),
                    ElevatedButton.icon(
                      style: ElevatedButton.styleFrom(
                        backgroundColor: Colors.black,
                        foregroundColor: Colors.white,
                        shape: const RoundedRectangleBorder(
                          borderRadius: BorderRadius.zero,
                        ),
                        side: const BorderSide(color: Colors.black, width: 2),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 20,
                          vertical: 12,
                        ),
                      ),
                      onPressed: _showCreateDialog,
                      icon: const Icon(Icons.add_rounded, size: 18),
                      label: const Text(
                        'CREAR PRIMERA NOTA',
                        style: TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 12,
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

    if (_isGridView) {
      // Desktop: grilla adaptativa de 3-4 columnas tipo post-it / ficha técnica.
      final width = MediaQuery.of(context).size.width;
      final crossAxisCount = width > 1024 ? (width > 1500 ? 4 : 3) : 2;
      return GridView.builder(
        padding: const EdgeInsets.fromLTRB(8, 8, 8, 90),
        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: crossAxisCount,
          crossAxisSpacing: AppDimens.spaceLg,
          mainAxisSpacing: AppDimens.spaceLg,
          childAspectRatio: width > 1024 ? 1.35 : (width < 600 ? 0.72 : 1.1),
        ),
        itemCount: _filtered.length,
        itemBuilder: (context, i) {
          final note = _filtered[i];
          return _NoteCard(
            note: note,
            isGrid: true,
            likes: _likesCount[note.id] ?? 0,
            isLiked: _isLiked[note.id] ?? false,
            isSaved: _isSaved[note.id] ?? false,
            tag: _noteTags[note.id] ?? 'General',
            onTap: () => _openNoteViewer(note),
            onLike: () => _toggleLike(note),
            onSave: () => _toggleSave(note),
            onShare: () => _shareNote(note),
          );
        },
      );
    }

    // Lista: una columna, ancho acotado en desktop para que la fila no se
    // estire de borde a borde y el extracto de 2 líneas siga siendo legible.
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(8, 8, 8, 90),
      itemCount: _filtered.length,
      separatorBuilder: (_, _) => const SizedBox(height: AppDimens.spaceMd),
      itemBuilder: (context, i) {
        final note = _filtered[i];
        return Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 880),
            child: _NoteCard(
              note: note,
              isGrid: false,
              likes: _likesCount[note.id] ?? 0,
              isLiked: _isLiked[note.id] ?? false,
              isSaved: _isSaved[note.id] ?? false,
              tag: _noteTags[note.id] ?? 'General',
              onTap: () => _openNoteViewer(note),
              onLike: () => _toggleLike(note),
              onSave: () => _toggleSave(note),
              onShare: () => _shareNote(note),
            ),
          ),
        );
      },
    );
  }
}

class _VisibilityOption extends StatelessWidget {
  final String label;
  final bool active;
  final VoidCallback onTap;
  const _VisibilityOption({
    required this.label,
    required this.active,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 10),
        decoration: BoxDecoration(
          color: active ? const Color(0xFF0055FF) : const Color(0xFFF5F0E8),
          border: Border.all(color: Colors.black, width: 2),
          boxShadow: active
              ? const [
                  BoxShadow(
                    color: Colors.black,
                    offset: Offset(2, 2),
                    blurRadius: 0,
                  ),
                ]
              : null,
        ),
        child: Center(
          child: Text(
            label,
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 11,
              color: active ? Colors.white : Colors.black,
            ),
          ),
        ),
      ),
    );
  }
}

class _NoteCard extends StatefulWidget {
  const _NoteCard({
    required this.note,
    required this.isGrid,
    required this.likes,
    required this.isLiked,
    required this.isSaved,
    required this.tag,
    required this.onTap,
    required this.onLike,
    required this.onSave,
    required this.onShare,
  });

  final LocalNote note;
  final bool isGrid;
  final int likes;
  final bool isLiked;
  final bool isSaved;
  final String tag;
  final VoidCallback onTap;
  final VoidCallback onLike;
  final VoidCallback onSave;
  final VoidCallback onShare;

  @override
  State<_NoteCard> createState() => _NoteCardState();
}

class _NoteCardState extends State<_NoteCard> {
  bool _hovered = false;
  bool _pressed = false;

  void _setHovered(bool value) {
    if (_hovered == value) return;
    setState(() => _hovered = value);
  }

  @override
  Widget build(BuildContext context) {
    // Microinteracción: hover desktop eleva la tarjeta hacia su sombra
    // (hard4 -> hard6); el press la hunde (traslación (4,4) sin sombra).
    final pressed = _pressed;
    final Offset offset = pressed
        ? AppShadows.offsetCard
        : (_hovered ? const Offset(-2, -2) : Offset.zero);
    final List<BoxShadow> shadow = pressed
        ? const <BoxShadow>[]
        : (_hovered ? AppShadows.dialog : AppShadows.card);

    return MouseRegion(
      cursor: SystemMouseCursors.click,
      onEnter: (_) => _setHovered(true),
      onExit: (_) {
        _setHovered(false);
        if (_pressed) setState(() => _pressed = false);
      },
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTapDown: (_) => setState(() => _pressed = true),
        onTapUp: (_) => setState(() => _pressed = false),
        onTapCancel: () => setState(() => _pressed = false),
        onTap: widget.onTap,
        child: AnimatedContainer(
          duration: AppMotion.press,
          curve: AppMotion.standard,
          transform: Matrix4.translationValues(offset.dx, offset.dy, 0),
          padding: const EdgeInsets.all(10),
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: shadow,
          ),
          child: widget.isGrid
              ? Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    _buildBadgesRow(),
                    const SizedBox(height: 6),
                    Text(
                      widget.note.title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w900,
                        fontSize: 13,
                        color: AppColors.text,
                        height: 1.2,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Expanded(
                      child: Text(
                        widget.note.content,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 11,
                          color: AppColors.muted,
                          height: 1.3,
                        ),
                      ),
                    ),
                    const SizedBox(height: 8),
                    _buildBottomActions(isGrid: true),
                  ],
                )
              : Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    _buildBadgesRow(),
                    const SizedBox(height: 6),
                    Text(
                      widget.note.title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w900,
                        fontSize: 16,
                        color: AppColors.text,
                        height: 1.2,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      widget.note.content,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w500,
                        color: AppColors.muted,
                        height: 1.3,
                      ),
                    ),
                    const SizedBox(height: 12),
                    _buildBottomActions(isGrid: false),
                  ],
                ),
        ),
      ),
    );
  }

  Widget _buildBadgesRow() {
    final isPublic = widget.note.visibility == 'public';
    return Row(
      children: <Widget>[
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
          decoration: BoxDecoration(
            color: isPublic ? AppColors.accentBlueDeep : AppColors.accentYellow,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Text(
            isPublic ? 'PUB' : 'PRIV',
            style: TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w900,
              color: isPublic ? AppColors.surface : AppColors.text,
            ),
          ),
        ),
        const SizedBox(width: AppDimens.spaceXs),
        Expanded(
          child: Text(
            _formatDate(widget.note.updatedAt),
            textAlign: TextAlign.end,
            style: const TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w700,
              color: AppColors.muted,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }

  Widget _buildBottomActions({required bool isGrid}) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: <Widget>[
        _CardAction(
          onTap: widget.onLike,
          background: widget.isLiked ? AppColors.errorDeep : AppColors.surface,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              Icon(
                Icons.favorite_rounded,
                size: 12,
                color: widget.isLiked ? AppColors.surface : AppColors.text,
              ),
              const SizedBox(width: 3),
              Text(
                '${widget.likes}',
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: widget.isLiked ? AppColors.surface : AppColors.text,
                ),
              ),
            ],
          ),
        ),
        _CardAction(
          onTap: widget.onSave,
          background: widget.isSaved
              ? AppColors.accentYellow
              : AppColors.surface,
          child: Icon(
            widget.isSaved
                ? Icons.bookmark_rounded
                : Icons.bookmark_border_rounded,
            size: 13,
            color: AppColors.text,
          ),
        ),
        _CardAction(
          onTap: widget.onShare,
          background: AppColors.surface,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              const Icon(Icons.share_rounded, size: 13, color: AppColors.text),
              if (!isGrid) ...<Widget>[
                const SizedBox(width: 4),
                const Text(
                  'Compartir',
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w800,
                    color: AppColors.text,
                  ),
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }

  String _formatDate(DateTime d) =>
      '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';
}

/// Acción compacta de la barra inferior de la tarjeta: borde de tinta de
/// 1.5 px, sombra dura 'hard2' y hundimiento mecánico al presionar.
class _CardAction extends StatefulWidget {
  const _CardAction({
    required this.onTap,
    required this.background,
    required this.child,
  });

  final VoidCallback onTap;
  final Color background;
  final Widget child;

  @override
  State<_CardAction> createState() => _CardActionState();
}

class _CardActionState extends State<_CardAction> {
  bool _pressed = false;
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() {
        _hovered = false;
        _pressed = false;
      }),
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
            _pressed ? AppShadows.offsetBadge.dx : (_hovered ? -1 : 0),
            _pressed ? AppShadows.offsetBadge.dy : (_hovered ? -1 : 0),
            0,
          ),
          padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 4),
          decoration: BoxDecoration(
            color: widget.background,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: _pressed ? const <BoxShadow>[] : AppShadows.badge,
          ),
          child: widget.child,
        ),
      ),
    );
  }
}

class _ActionButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool isActive;
  final Color activeColor;
  final Color activeForeground;
  final VoidCallback onTap;
  const _ActionButton({
    required this.icon,
    required this.label,
    this.isActive = false,
    this.activeColor = AppColors.accentYellow,
    this.activeForeground = AppColors.text,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final background = isActive ? activeColor : AppColors.surface;
    final foreground = isActive ? activeForeground : AppColors.text;
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(
            color: background,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 14, color: foreground),
              const SizedBox(width: 4),
              Text(
                label,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w800,
                  color: foreground,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Resultado de una operación remota contra el backend de notas (8082).
class _RemoteResult {
  final bool ok;
  final int status; // -1 = error de red/offline
  final String?
  code; // code semántico del backend (note_unavailable, forbidden...)
  final String? message;
  final Map<String, dynamic>? data; // payload del 200
  const _RemoteResult({
    required this.ok,
    required this.status,
    this.code,
    this.message,
    this.data,
  });
}

enum _EditSaveOutcome { synced, forbidden, unavailable, offline, error }

/// Modal de detalle con lectura híbrida (GET /notes/:id), edición (PATCH) y
/// borrado (DELETE). El contenido local se muestra de inmediato como fallback.
class _NoteDetailSheet extends StatefulWidget {
  final ScrollController scrollController;
  final LocalNote note;
  final bool isMine;
  final String tag;
  final int likesCount;
  final bool isLiked;
  final bool isSaved;
  final List<Map<String, dynamic>> resources;
  final VoidCallback onToggleLike;
  final VoidCallback onToggleSave;
  final VoidCallback onShare;
  final Future<_RemoteResult> Function() onFetchRemote;
  final Future<_EditSaveOutcome> Function(String title, String content)
  onSaveEdit;
  final Future<_RemoteResult> Function() onDelete;
  final Future<_RemoteResult> Function() onClone;

  const _NoteDetailSheet({
    required this.scrollController,
    required this.note,
    required this.isMine,
    required this.tag,
    required this.likesCount,
    required this.isLiked,
    required this.isSaved,
    this.resources = const [],
    required this.onToggleLike,
    required this.onToggleSave,
    required this.onShare,
    required this.onFetchRemote,
    required this.onSaveEdit,
    required this.onDelete,
    required this.onClone,
  });

  @override
  State<_NoteDetailSheet> createState() => _NoteDetailSheetState();
}

class _NoteDetailSheetState extends State<_NoteDetailSheet> {
  late String _title;
  late String _content;
  late int _likes;
  late bool _liked;
  late bool _saved;
  bool _loadingRemote = true;
  String? _driveWarning;
  bool _editing = false;
  bool _saving = false;
  String? _editError;
  bool _deleting = false;
  bool _cloning = false;
  String? _attachNotice;
  bool _attachNoticeOk = false;
  TextEditingController? _titleCtrl;
  TextEditingController? _contentCtrl;

  @override
  void initState() {
    super.initState();
    _title = widget.note.title;
    _content = widget.note.content;
    _likes = widget.likesCount;
    _liked = widget.isLiked;
    _saved = widget.isSaved;
    _loadRemote();
  }

  @override
  void dispose() {
    _disposeEditControllers();
    super.dispose();
  }

  /// Aviso humano según el estado REAL:
  /// - id local (timestamp, sin '-') que nunca se sincronizó → pendiente;
  /// - id backend (UUID) ilegible → fallo del servidor, sin tecnicismos.
  String _remoteWarningFor(String noteId, _RemoteResult res) {
    final neverSynced = !noteId.contains('-');
    if (res.code == 'note_unavailable' ||
        (res.message ?? '').contains('no disponible') ||
        res.status == 404) {
      if (neverSynced) {
        return 'Esta nota todavía no se ha sincronizado con Google Drive.';
      }
      return 'No pudimos abrir la versión guardada. Inténtalo más tarde.';
    }
    if (res.status == 403) {
      return 'No tienes acceso a esta nota.';
    }
    return 'No se pudo cargar la versión del servidor. Se muestra la copia local.';
  }

  Future<void> _loadRemote() async {
    final res = await widget.onFetchRemote();
    if (!mounted) return;
    setState(() {
      _loadingRemote = false;
      if (res.ok) {
        final d = res.data ?? {};
        final rc = d['content'];
        if (rc is String) _content = rc;
        final rt = d['title'];
        if (rt is String && rt.isNotEmpty) _title = rt;
      } else if (res.status == -1) {
        // Offline: se mantiene el contenido local como fallback.
      } else {
        _driveWarning = _remoteWarningFor(widget.note.id, res);
      }
    });
  }

  /// Editar solo si la lectura remota no reportó error: con el aviso
  /// visible se bloquea mostrando el mismo motivo.
  /// Eliminar permanece habilitado para limpiar la referencia local.
  void _onEditTap() {
    if (_driveWarning != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: Colors.black,
          content: Text(
            _driveWarning!,
            style: const TextStyle(
              color: Colors.white,
              fontWeight: FontWeight.w800,
            ),
          ),
        ),
      );
      return;
    }
    _startEdit();
  }

  void _startEdit() {
    _titleCtrl = TextEditingController(text: _title);
    _contentCtrl = TextEditingController(text: _content);
    _contentCtrl?.addListener(_onEditContentChanged);
    setState(() {
      _editing = true;
      _editError = null;
    });
  }

  void _onEditContentChanged() {
    if (mounted) setState(() {});
  }

  void _cancelEdit() {
    setState(() {
      _editing = false;
      _editError = null;
    });
    _releaseEditControllersAfterFrame();
  }

  void _releaseEditControllersAfterFrame() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _disposeEditControllers();
    });
  }

  void _disposeEditControllers() {
    _contentCtrl?.removeListener(_onEditContentChanged);
    _titleCtrl?.dispose();
    _titleCtrl = null;
    _contentCtrl?.dispose();
    _contentCtrl = null;
  }

  Future<void> _confirmEdit() async {
    final t = _titleCtrl?.text.trim() ?? '';
    final c = _contentCtrl?.text ?? '';
    if (t.isEmpty) {
      setState(() => _editError = 'El título no puede estar vacío.');
      return;
    }
    setState(() {
      _saving = true;
      _editError = null;
    });
    final outcome = await widget.onSaveEdit(t, c);
    if (!mounted) return;
    setState(() => _saving = false);
    switch (outcome) {
      case _EditSaveOutcome.synced:
        _title = t;
        _content = c;
        _editing = false;
        _releaseEditControllersAfterFrame();
        break;
      case _EditSaveOutcome.forbidden:
        _editError = 'Solo el autor puede editar esta nota.';
        break;
      case _EditSaveOutcome.unavailable:
        _editError =
            'Esta nota todavía no se ha sincronizado con Google Drive.';
        break;
      case _EditSaveOutcome.offline:
        _editError = 'Sin conexión: se mantiene el contenido local.';
        break;
      case _EditSaveOutcome.error:
        _editError = 'No se pudo guardar la edición en el servidor.';
        break;
    }
  }

  Future<void> _confirmDelete() async {
    final sure = await showDialog<bool>(
      context: context,
      builder: (dctx) => AlertDialog(
        backgroundColor: Colors.white,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.zero,
          side: BorderSide(color: Colors.black, width: 2),
        ),
        title: const Text(
          'ELIMINAR NOTA',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 14,
            color: Colors.black,
          ),
        ),
        content: const Text(
          '¿Eliminar esta nota? También se borrará de Google Drive.',
          style: TextStyle(fontWeight: FontWeight.w600, color: Colors.black),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(dctx, false),
            child: const Text(
              'CANCELAR',
              style: TextStyle(
                color: Colors.black,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          ElevatedButton(
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFE63B2E),
              foregroundColor: Colors.white,
              shape: const RoundedRectangleBorder(
                borderRadius: BorderRadius.zero,
              ),
            ),
            onPressed: () => Navigator.pop(dctx, true),
            child: const Text(
              'ELIMINAR',
              style: TextStyle(fontWeight: FontWeight.w900),
            ),
          ),
        ],
      ),
    );
    if (sure != true || !mounted) return;
    setState(() => _deleting = true);
    final res = await widget.onDelete();
    if (!mounted) return;
    setState(() => _deleting = false);
    if (res.ok) {
      Navigator.pop(context);
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: Colors.black,
          content: Text(
            res.status == -1
                ? 'Sin conexión: no se pudo eliminar.'
                : 'No se pudo eliminar. Inténtalo más tarde.',
            style: const TextStyle(
              color: Colors.white,
              fontWeight: FontWeight.w800,
            ),
          ),
        ),
      );
    }
  }

  String _fmtDate(DateTime d) =>
      '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';

  String _cloneErrorText(_RemoteResult res) {
    if (res.status == -1) return 'Sin conexión: no se pudo guardar la copia.';
    return 'No se pudo guardar la copia. Inténtalo más tarde.';
  }

  Future<void> _confirmClone() async {
    setState(() => _cloning = true);
    final res = await widget.onClone();
    if (!mounted) return;
    setState(() => _cloning = false);
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: Colors.black,
        content: Text(
          res.ok
              ? 'Copia guardada exitosamente en tu Drive'
              : _cloneErrorText(res),
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  Future<void> _downloadResource(Map<String, dynamic> resource) async {
    final url = (resource['url'] ?? '') as String;
    final info = _ResourceFileInfo.resolve(url);
    String message;
    if (info.localFile != null) {
      final dest = await _copyLocalResourceToDownloads(info.localFile!);
      message = dest != null
          ? 'Documento copiado a descargas: $dest'
          : 'El archivo ya está en disco: ${info.localFile!.absolute.path}';
    } else if (info.isRemote) {
      unawaited(_openResourceExternally(url));
      message = 'Descarga iniciada: ${resource['name']}';
    } else {
      message = 'No se encontró el archivo para descargar: ${resource['name']}';
    }
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: Colors.black,
        content: Text(
          message,
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  void _showResourceDialog(Map<String, dynamic> resource) {
    showDialog<void>(
      context: context,
      builder: (_) => _ResourceViewerDialog(resource: resource),
    );
  }

  /// Adjunto recién subido desde la edición: registra su metadata contra la
  /// nota (si ya tiene UUID remoto) y muestra confirmación en el banner.
  Future<void> _onAttachmentUploadedInEdit(_DriveUploadResult result) async {
    if (mounted) {
      setState(() {
        _attachNotice =
            'Subido a Google Drive: ${result.fileName ?? 'adjunto'}';
        _attachNoticeOk = true;
      });
    }
    final externalId = result.externalFileId;
    final noteId = widget.note.id;
    if (externalId == null || externalId.isEmpty) return;
    // Los ids locales (solo dígitos) aún no existen en el backend.
    if (noteId.isEmpty || int.tryParse(noteId) != null) return;
    try {
      final res = await AuthedHttp.run(
        () => _withNotesClient(
          (client) => client
              .post(
                Uri.parse('$notesBaseUrl/notes/$noteId/attachments'),
                headers: _notesAuthHeaders(),
                body: jsonEncode(<String, dynamic>{
                  'external_file_id': externalId,
                  'file_name': result.fileName,
                  'file_type': result.fileType,
                  'file_size_bytes': result.fileSizeBytes,
                  'is_inline': result.isInline,
                }),
              )
              .timeout(const Duration(seconds: 8)),
        ),
      );
      if (res.statusCode != 201) {
        debugPrint(
          '[FRONT DEBUG] No se pudo registrar adjunto $externalId en '
          'nota $noteId: status=${res.statusCode} '
          'body=${utf8.decode(res.bodyBytes)}',
        );
      }
    } catch (e) {
      debugPrint('[FRONT DEBUG] Error registrando adjunto $externalId: $e');
    }
  }

  List<Map<String, dynamic>> get _mergedResources {
    final merged = <Map<String, dynamic>>[
      ...widget.resources,
      ..._resourcesFromMarkdown(_content),
    ];
    final seen = <String>{};
    final out = <Map<String, dynamic>>[];
    for (final r in merged) {
      final key = '${r['type']}::${r['url'] ?? r['name']}';
      if (seen.add(key)) out.add(r);
    }
    return out;
  }

  List<Widget> _buildResourceItems() {
    final resources = _mergedResources;
    final images = resources.where((r) => r['type'] == 'image').toList();
    final docs = resources.where((r) => r['type'] != 'image').toList();
    return <Widget>[
      if (images.isNotEmpty) ...[
        if (docs.isNotEmpty) ...[
          const Text(
            'IMÁGENES',
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 10,
              letterSpacing: 0.5,
              color: Color(0xFF555555),
            ),
          ),
          const SizedBox(height: 8),
        ],
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final img in images)
              _ResourceImageThumb(
                resource: img,
                onTap: () => _showResourceDialog(img),
              ),
          ],
        ),
        if (docs.isNotEmpty) const SizedBox(height: 14),
      ],
      if (docs.isNotEmpty) ...[
        const Text(
          'DOCUMENTOS',
          style: TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 10,
            letterSpacing: 0.5,
            color: Color(0xFF555555),
          ),
        ),
        const SizedBox(height: 8),
        for (final doc in docs) ...[
          _ResourceDocTile(
            resource: doc,
            onView: () => _showResourceDialog(doc),
            onDownload: _downloadResource,
          ),
          const SizedBox(height: 8),
        ],
      ],
    ];
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      decoration: const BoxDecoration(
        color: Colors.white,
        border: Border(
          top: BorderSide(color: Colors.black, width: 2),
          left: BorderSide(color: Colors.black, width: 2),
          right: BorderSide(color: Colors.black, width: 2),
        ),
        boxShadow: [BoxShadow(color: Colors.black, offset: Offset(6, 6))],
      ),
      child: Column(
        children: [
          Container(
            margin: const EdgeInsets.only(top: 12),
            width: 40,
            height: 4,
            decoration: BoxDecoration(
              color: Colors.black,
              borderRadius: BorderRadius.circular(0),
            ),
          ),
          Expanded(
            child: ListView(
              controller: widget.scrollController,
              padding: const EdgeInsets.all(20),
              children: [
                Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 4,
                      ),
                      decoration: BoxDecoration(
                        color: widget.note.visibility == 'public'
                            ? const Color(0xFF0055FF)
                            : const Color(0xFFFFCC00),
                        border: Border.all(color: Colors.black, width: 1.5),
                      ),
                      child: Text(
                        widget.note.visibility.toUpperCase(),
                        style: TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w900,
                          color: widget.note.visibility == 'public'
                              ? Colors.white
                              : Colors.black,
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Align(
                        alignment: Alignment.centerLeft,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 6,
                            vertical: 2,
                          ),
                          decoration: BoxDecoration(
                            color: const Color(0xFFF5F0E8),
                            border: Border.all(color: Colors.black, width: 1.5),
                          ),
                          child: Text(
                            widget.tag,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontSize: 10,
                              fontWeight: FontWeight.w800,
                              color: Colors.black,
                            ),
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      _fmtDate(widget.note.updatedAt),
                      style: const TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ],
                ),
                if (_loadingRemote) ...[
                  const SizedBox(height: 12),
                  const LinearProgressIndicator(
                    color: Colors.black,
                    backgroundColor: Color(0xFFF5F0E8),
                  ),
                  const SizedBox(height: 4),
                  const Text(
                    'Sincronizando con Drive…',
                    style: TextStyle(
                      fontSize: 11,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF555555),
                    ),
                  ),
                ],
                if (_driveWarning != null) ...[
                  const SizedBox(height: 12),
                  Container(
                    padding: const EdgeInsets.all(10),
                    decoration: BoxDecoration(
                      color: const Color(0xFFFFF3C4),
                      border: Border.all(color: Colors.black, width: 1.5),
                    ),
                    child: Row(
                      children: [
                        const Icon(
                          Icons.warning_amber_rounded,
                          color: Colors.black,
                          size: 18,
                        ),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            _driveWarning!,
                            style: const TextStyle(
                              fontSize: 12,
                              fontWeight: FontWeight.w800,
                              color: Colors.black,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
                const SizedBox(height: 16),
                if (_editing) ...[
                  TextField(
                    controller: _titleCtrl,
                    decoration: const InputDecoration(
                      hintText: 'Título',
                      filled: true,
                      fillColor: Color(0xFFF5F0E8),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.zero,
                        borderSide: BorderSide(color: Colors.black, width: 2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: _contentCtrl,
                    maxLines: 6,
                    decoration: const InputDecoration(
                      hintText: 'Contenido Markdown',
                      filled: true,
                      fillColor: Color(0xFFF5F0E8),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.zero,
                        borderSide: BorderSide(color: Colors.black, width: 2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 8),
                  if (_contentCtrl != null) ...[
                    _AttachmentToolbar(
                      controller: _contentCtrl!,
                      onChanged: () => setState(() {}),
                      onUploaded: _onAttachmentUploadedInEdit,
                      onWarning: (message) {
                        if (!mounted) return;
                        setState(() {
                          _attachNotice = message;
                          _attachNoticeOk = false;
                        });
                      },
                    ),
                    if (_attachNotice != null) ...[
                      const SizedBox(height: 6),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 6,
                        ),
                        decoration: BoxDecoration(
                          color: _attachNoticeOk
                              ? AppColors.subjectMint
                              : AppColors.accentYellow,
                          border: Border.all(
                            color: AppColors.border,
                            width: 1.5,
                          ),
                          borderRadius: BorderRadius.circular(
                            AppDimens.radiusChip,
                          ),
                        ),
                        child: Row(
                          children: [
                            Icon(
                              _attachNoticeOk
                                  ? Icons.cloud_done_rounded
                                  : Icons.info_outline_rounded,
                              size: 14,
                              color: AppColors.text,
                            ),
                            const SizedBox(width: 6),
                            Expanded(
                              child: Text(
                                _attachNotice!,
                                style: const TextStyle(
                                  fontSize: 10,
                                  fontWeight: FontWeight.w800,
                                  color: AppColors.text,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                    _DetectedAttachmentsPreview(
                      content: _contentCtrl!.text,
                      onRemoveSnippet: (url) {
                        _removeAttachmentUrl(_contentCtrl!, url);
                        setState(() {});
                      },
                      onPreviewResource: _showResourceDialog,
                    ),
                  ],
                  if (_editError != null) ...[
                    const SizedBox(height: 8),
                    Text(
                      _editError!,
                      style: const TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFFE63B2E),
                      ),
                    ),
                  ],
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(
                        child: TextButton(
                          onPressed: _saving ? null : _cancelEdit,
                          child: const Text(
                            'CANCELAR',
                            style: TextStyle(
                              color: Colors.black,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: ElevatedButton(
                          style: ElevatedButton.styleFrom(
                            backgroundColor: Colors.black,
                            foregroundColor: Colors.white,
                            shape: const RoundedRectangleBorder(
                              borderRadius: BorderRadius.zero,
                            ),
                          ),
                          onPressed: _saving ? null : _confirmEdit,
                          child: Text(
                            _saving ? 'GUARDANDO…' : 'GUARDAR',
                            style: const TextStyle(fontWeight: FontWeight.w900),
                          ),
                        ),
                      ),
                    ],
                  ),
                ] else ...[
                  Text(
                    _title,
                    style: const TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w900,
                      color: Colors.black,
                      height: 1.2,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Container(height: 2, color: Colors.black),
                  const SizedBox(height: 16),
                  _NeobrutalistMarkdownBody(
                    content: _content,
                    onOpenResource: _showResourceDialog,
                  ),
                ],
                if (_mergedResources.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  Container(height: 2, color: Colors.black),
                  const SizedBox(height: 14),
                  const Row(
                    children: [
                      Icon(
                        Icons.folder_open_rounded,
                        size: 16,
                        color: Colors.black,
                      ),
                      SizedBox(width: 6),
                      Text(
                        'RECURSOS ADJUNTOS',
                        style: TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 13,
                          letterSpacing: 0.5,
                          color: Colors.black,
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 12),
                  ..._buildResourceItems(),
                ],
                const SizedBox(height: 24),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    _ActionButton(
                      icon: Icons.favorite_rounded,
                      label: '$_likes',
                      isActive: _liked,
                      activeColor: AppColors.errorDeep,
                      activeForeground: AppColors.surface,
                      onTap: () {
                        setState(() {
                          _liked = !_liked;
                          _likes += _liked ? 1 : -1;
                        });
                        widget.onToggleLike();
                      },
                    ),
                    _ActionButton(
                      icon: Icons.bookmark_rounded,
                      label: 'Guardar',
                      isActive: _saved,
                      activeColor: AppColors.accentYellow,
                      onTap: () {
                        setState(() => _saved = !_saved);
                        widget.onToggleSave();
                      },
                    ),
                    _ActionButton(
                      icon: Icons.share_rounded,
                      label: 'Compartir',
                      onTap: widget.onShare,
                    ),
                    if (!widget.isMine) ...[
                      _ActionButton(
                        icon: Icons.copy_rounded,
                        label: _cloning ? '…' : 'Guardar copia',
                        onTap: _cloning ? () {} : _confirmClone,
                      ),
                    ],
                    if (widget.isMine && !_editing) ...[
                      _ActionButton(
                        icon: Icons.edit_rounded,
                        label: 'Editar',
                        onTap: _onEditTap,
                      ),
                      _ActionButton(
                        icon: Icons.delete_rounded,
                        label: _deleting ? '…' : 'Eliminar',
                        onTap: _deleting ? () {} : _confirmDelete,
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Diálogo modal neobrutalista para creación de notas con barra de adjuntos
/// y previsualización de archivos vinculados en tiempo real.
class _CreateNoteDialog extends StatefulWidget {
  final Future<void> Function(
    String title,
    String content,
    String visibility,
    List<_DriveUploadResult> attachments,
  )
  onCreate;

  const _CreateNoteDialog({required this.onCreate});

  @override
  State<_CreateNoteDialog> createState() => _CreateNoteDialogState();
}

class _CreateNoteDialogState extends State<_CreateNoteDialog> {
  late final TextEditingController _titleCtrl;
  late final TextEditingController _contentCtrl;
  String _visibility = 'private';
  bool _submitting = false;
  // Adjuntos que ya viven en Drive: se registran contra la nota cuando el
  // backend devuelve su UUID real (POST /notes/{id}/attachments).
  final List<_DriveUploadResult> _pendingAttachments = [];
  String? _attachNotice;
  bool _attachNoticeOk = false;

  @override
  void initState() {
    super.initState();
    _titleCtrl = TextEditingController();
    _contentCtrl = TextEditingController();
    _contentCtrl.addListener(_onContentChanged);
  }

  void _onContentChanged() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _contentCtrl.removeListener(_onContentChanged);
    _titleCtrl.dispose();
    _contentCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_submitting) return;
    if (_titleCtrl.text.trim().isEmpty) return;
    setState(() => _submitting = true);
    try {
      await widget.onCreate(
        _titleCtrl.text.trim(),
        _contentCtrl.text.trim(),
        _visibility,
        List.unmodifiable(_pendingAttachments),
      );
    } finally {
      if (mounted) Navigator.of(context).pop();
    }
  }

  void _onAttachmentUploaded(_DriveUploadResult result) {
    _pendingAttachments.add(result);
    if (!mounted) return;
    setState(() {
      _attachNotice = 'Subido a Google Drive: ${result.fileName ?? 'adjunto'}';
      _attachNoticeOk = true;
    });
  }

  void _onAttachmentWarning(String message) {
    if (!mounted) return;
    setState(() {
      _attachNotice = message;
      _attachNoticeOk = false;
    });
  }

  void _showResource(Map<String, dynamic> res) {
    showDialog<void>(
      context: context,
      builder: (_) => _ResourceViewerDialog(resource: res),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.zero,
        side: BorderSide(color: Colors.black, width: 2),
      ),
      title: const Text(
        'NUEVA NOTA',
        style: TextStyle(
          fontWeight: FontWeight.w900,
          fontSize: 14,
          color: Colors.black,
        ),
      ),
      content: SizedBox(
        width: 480,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextField(
                controller: _titleCtrl,
                decoration: const InputDecoration(
                  hintText: 'Título',
                  filled: true,
                  fillColor: Color(0xFFF5F0E8),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.zero,
                    borderSide: BorderSide(color: Colors.black, width: 2),
                  ),
                ),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _contentCtrl,
                maxLines: 4,
                decoration: const InputDecoration(
                  hintText: 'Contenido Markdown',
                  filled: true,
                  fillColor: Color(0xFFF5F0E8),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.zero,
                    borderSide: BorderSide(color: Colors.black, width: 2),
                  ),
                ),
              ),
              const SizedBox(height: 8),
              _AttachmentToolbar(
                controller: _contentCtrl,
                onChanged: () => setState(() {}),
                onUploaded: _onAttachmentUploaded,
                onWarning: _onAttachmentWarning,
              ),
              if (_attachNotice != null) ...[
                const SizedBox(height: 6),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: _attachNoticeOk
                        ? AppColors.subjectMint
                        : AppColors.accentYellow,
                    border: Border.all(color: AppColors.border, width: 1.5),
                    borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                  ),
                  child: Row(
                    children: [
                      Icon(
                        _attachNoticeOk
                            ? Icons.cloud_done_rounded
                            : Icons.info_outline_rounded,
                        size: 14,
                        color: AppColors.text,
                      ),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(
                          _attachNotice!,
                          style: const TextStyle(
                            fontSize: 10,
                            fontWeight: FontWeight.w800,
                            color: AppColors.text,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
              _DetectedAttachmentsPreview(
                content: _contentCtrl.text,
                onRemoveSnippet: (url) {
                  _removeAttachmentUrl(_contentCtrl, url);
                  _pendingAttachments.removeWhere((a) => a.fileUrl == url);
                  setState(() {});
                },
                onPreviewResource: _showResource,
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  Expanded(
                    child: _VisibilityOption(
                      label: 'PÚBLICO',
                      active: _visibility == 'public',
                      onTap: () => setState(() => _visibility = 'public'),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: _VisibilityOption(
                      label: 'PRIVADO',
                      active: _visibility == 'private',
                      onTap: () => setState(() => _visibility = 'private'),
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _submitting ? null : () => Navigator.pop(context),
          child: const Text(
            'CANCELAR',
            style: TextStyle(color: Colors.black, fontWeight: FontWeight.w800),
          ),
        ),
        ElevatedButton(
          style: ElevatedButton.styleFrom(
            backgroundColor: Colors.black,
            foregroundColor: Colors.white,
            shape: const RoundedRectangleBorder(
              borderRadius: BorderRadius.zero,
            ),
          ),
          onPressed: _submitting ? null : _submit,
          child: Text(
            _submitting ? 'GUARDANDO…' : 'GUARDAR',
            style: const TextStyle(fontWeight: FontWeight.w900),
          ),
        ),
      ],
    );
  }
}

/// Inserta el fragmento Markdown en la posición actual del cursor.
void _insertSnippetAtCursor(TextEditingController controller, String snippet) {
  final text = controller.text;
  final selection = controller.selection;
  final start = selection.start;
  final end = selection.end;

  String newText;
  int newOffset;

  if (start >= 0 && end >= 0 && start <= text.length && end <= text.length) {
    final before = text.substring(0, start);
    final after = text.substring(end);
    final needsLeadingNewline = before.isNotEmpty && !before.endsWith('\n');
    final needsTrailingNewline = after.isNotEmpty && !after.startsWith('\n');
    final insertion =
        '${needsLeadingNewline ? '\n' : ''}$snippet${needsTrailingNewline ? '\n' : ''}';
    newText = '$before$insertion$after';
    newOffset = start + insertion.length;
  } else {
    final needsNewline = text.isNotEmpty && !text.endsWith('\n');
    final insertion = '${needsNewline ? '\n' : ''}$snippet\n';
    newText = '$text$insertion';
    newOffset = newText.length;
  }

  controller.value = TextEditingValue(
    text: newText,
    selection: TextSelection.collapsed(offset: newOffset),
  );
}

/// Elimina de la nota la referencia Markdown asociada a la ruta/URL indicada.
void _removeAttachmentUrl(TextEditingController controller, String url) {
  final text = controller.text;
  final escaped = RegExp.escape(url);
  final pattern = RegExp(
    r'(\n?!\[[^\]]*\]\(' +
        escaped +
        r'\)\n?|\n?\[[^\]]+\]\(' +
        escaped +
        r'\)\n?)',
  );
  final newText = text
      .replaceAll(pattern, '\n')
      .replaceAll(RegExp(r'\n{3,}'), '\n\n')
      .trim();
  controller.value = TextEditingValue(
    text: newText,
    selection: TextSelection.collapsed(offset: newText.length),
  );
}

/// Extrae {code, message} del envelope de error del backend: {"error": {...}}.
Map<String, String?> _parseBackendError(String body) {
  try {
    final decoded = jsonDecode(body);
    if (decoded is Map) {
      final err = decoded['error'];
      if (err is Map) {
        return {
          'code': err['code']?.toString(),
          'message': err['message']?.toString(),
        };
      }
    }
  } catch (_) {}
  return {'code': null, 'message': null};
}

/// Cabeceras JSON con Bearer JWT cuando hay sesión activa.
Map<String, String> _notesAuthHeaders() {
  final headers = <String, String>{'Content-Type': 'application/json'};
  final token = SessionManager.token;
  if (token != null && token.isNotEmpty) {
    headers['Authorization'] = 'Bearer $token';
  }
  return headers;
}

/// Adjunto ya subido a Google Drive mediante POST /notes/upload. Conserva el
/// external_file_id para poder registrarlo luego contra la nota con
/// POST /notes/{id}/attachments, además del enlace web incrustado en Markdown.
class _DriveUploadResult {
  final bool ok;
  final String? fileUrl;
  final String? externalFileId;
  final String? fileName;
  final String? fileType;
  final int fileSizeBytes;
  final bool isInline;
  final String? message;

  const _DriveUploadResult({
    required this.ok,
    this.fileUrl,
    this.externalFileId,
    this.fileName,
    this.fileType,
    this.fileSizeBytes = 0,
    this.isInline = false,
    this.message,
  });
}

/// Nombre de archivo a declarar en Drive a partir de la ruta/URL de origen: usa
/// el basename real (conejita.jpg, prueba.pdf, foto.png) para que el backend
/// resuelva el MIME por extensión; si no trae extensión añade una coherente.
String _attachmentFileNameFrom(String sourcePath, {required bool isImage}) {
  final parts = sourcePath
      .trim()
      .split(RegExp(r'[\\/]'))
      .where((p) => p.isNotEmpty)
      .toList();
  var name = parts.isEmpty ? '' : parts.last;
  if (name.isEmpty) name = isImage ? 'adjunto.png' : 'adjunto.pdf';
  if (!name.contains('.')) name = isImage ? '$name.png' : '$name.pdf';
  return name;
}

/// Traduce el fallo de POST /notes/upload a un mensaje accionable en español.
String _uploadFailureMessage(int status, Map<String, String?> parsed) {
  final backendMessage = parsed['message'];
  if (status == 401) {
    return 'Tu sesión expiró: vuelve a iniciar sesión para subir a Drive.';
  }
  if (status == 403) {
    return (backendMessage != null && backendMessage.isNotEmpty)
        ? backendMessage
        : 'Conecta tu Google Drive para subir adjuntos.';
  }
  if (status == 413) return 'El archivo supera el límite de 10 MB.';
  if (status == 400) {
    return (backendMessage != null && backendMessage.isNotEmpty)
        ? backendMessage
        : 'El archivo no es válido para subir.';
  }
  if (status == -1) {
    return 'Sin conexión con el servidor: el adjunto quedó como referencia local.';
  }
  return 'No se pudo subir el adjunto a Drive (código $status).';
}

/// Sube los bytes reales de [file] al Drive del usuario autenticado usando
/// POST /notes/upload (multipart field 'file' + Bearer JWT de SessionManager).
/// Nunca lanza: ante cualquier fallo devuelve ok=false con un mensaje amigable
/// para que el llamador conserve el fallback local sin romper la UI.
Future<_DriveUploadResult> _uploadAttachmentToDrive({
  required File file,
  required String fileName,
  required bool isInline,
}) async {
  final sessionToken = SessionManager.token;
  if (sessionToken == null || sessionToken.isEmpty) {
    return const _DriveUploadResult(
      ok: false,
      message: 'Sin sesión activa: el adjunto quedó como referencia local.',
    );
  }
  try {
    final List<int> bytes;
    try {
      bytes = await file.readAsBytes();
    } catch (_) {
      return _DriveUploadResult(
        ok: false,
        message: 'No se pudo leer el archivo local "$fileName".',
      );
    }
    if (bytes.isEmpty) {
      return const _DriveUploadResult(
        ok: false,
        message: 'El archivo está vacío: no se puede subir a Drive.',
      );
    }
    if (bytes.length > 10 * 1024 * 1024) {
      return const _DriveUploadResult(
        ok: false,
        message: 'El archivo supera el límite de 10 MB.',
      );
    }
    final request = http.MultipartRequest(
      'POST',
      Uri.parse('$notesBaseUrl/notes/upload'),
    );
    final res = await AuthedHttp.run(
      () => _withNotesClient((client) async {
        // Se reconstruye por intento: un MultipartRequest finalizado no se
        // puede reenviar tras un refresh.
        request.headers['Authorization'] = 'Bearer ${SessionManager.token}';
        request.files.clear();
        request.files.add(
          http.MultipartFile.fromBytes('file', bytes, filename: fileName),
        );
        final streamed = await client
            .send(request)
            .timeout(const Duration(seconds: 30));
        return http.Response.fromStream(
          streamed,
        ).timeout(const Duration(seconds: 30));
      }),
    );
    final rawBody = utf8.decode(res.bodyBytes);
    if (res.statusCode == 201) {
      final data = jsonDecode(rawBody) as Map<String, dynamic>;
      final url = (data['file_url'] as String?)?.trim() ?? '';
      final extId = (data['external_file_id'] as String?)?.trim() ?? '';
      if (url.isEmpty || extId.isEmpty) {
        return const _DriveUploadResult(
          ok: false,
          message: 'Drive no devolvió el enlace del archivo subido.',
        );
      }
      return _DriveUploadResult(
        ok: true,
        fileUrl: url,
        externalFileId: extId,
        fileName: (data['file_name'] as String?) ?? fileName,
        fileType: (data['file_type'] as String?) ?? '',
        fileSizeBytes:
            (data['file_size_bytes'] as num?)?.toInt() ?? bytes.length,
        isInline: isInline,
      );
    }
    return _DriveUploadResult(
      ok: false,
      message: _uploadFailureMessage(
        res.statusCode,
        _parseBackendError(rawBody),
      ),
    );
  } on TimeoutException {
    return const _DriveUploadResult(
      ok: false,
      message:
          'Sin conexión con el servidor: el adjunto quedó como referencia local.',
    );
  } catch (_) {
    return const _DriveUploadResult(
      ok: false,
      message:
          'No se pudo subir a Drive: el adjunto quedó como referencia local.',
    );
  }
}

/// Barra de botones de acción rápida para adjuntar imágenes o documentos.
class _AttachmentToolbar extends StatelessWidget {
  final TextEditingController controller;
  final VoidCallback onChanged;
  final ValueChanged<_DriveUploadResult>? onUploaded;
  final ValueChanged<String>? onWarning;

  const _AttachmentToolbar({
    required this.controller,
    required this.onChanged,
    this.onUploaded,
    this.onWarning,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Row(
        children: [
          const Icon(Icons.attachment_rounded, size: 16, color: AppColors.text),
          const SizedBox(width: 6),
          const Text(
            'ADJUNTAR:',
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
              letterSpacing: 0.5,
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Wrap(
              spacing: 6,
              runSpacing: 4,
              children: [
                _AttachmentToolbarBtn(
                  icon: Icons.add_photo_alternate_rounded,
                  label: 'IMAGEN',
                  fill: AppColors.subjectMint,
                  onTap: () => _openAttachmentDialog(context, isImage: true),
                ),
                _AttachmentToolbarBtn(
                  icon: Icons.picture_as_pdf_rounded,
                  label: 'PDF / DOC',
                  fill: AppColors.accentYellow,
                  onTap: () => _openAttachmentDialog(context, isImage: false),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  void _openAttachmentDialog(BuildContext context, {required bool isImage}) {
    showDialog<void>(
      context: context,
      builder: (_) => _AddAttachmentDialog(
        isImage: isImage,
        onInsert: (snippet) {
          _insertSnippetAtCursor(controller, snippet);
          onChanged();
        },
        onUploaded: onUploaded,
        onWarning: onWarning,
      ),
    );
  }
}

class _AttachmentToolbarBtn extends StatelessWidget {
  final IconData icon;
  final String label;
  final Color fill;
  final VoidCallback onTap;

  const _AttachmentToolbarBtn({
    required this.icon,
    required this.label,
    required this.fill,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(
            color: fill,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 13, color: AppColors.text),
              const SizedBox(width: 4),
              Text(
                label,
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Diálogo selector de adjunto: incluye preset rápido del repo (conejita.jpg o prueba.pdf)
/// y campos para archivo/URL personalizado. Al confirmar, lee los bytes reales
/// del archivo local y lo sube al Drive del usuario (POST /notes/upload);
/// solo si la subida falla o no hay sesión se conserva la referencia local.
class _AddAttachmentDialog extends StatefulWidget {
  final bool isImage;
  final ValueChanged<String> onInsert;
  final ValueChanged<_DriveUploadResult>? onUploaded;
  final ValueChanged<String>? onWarning;

  const _AddAttachmentDialog({
    required this.isImage,
    required this.onInsert,
    this.onUploaded,
    this.onWarning,
  });

  @override
  State<_AddAttachmentDialog> createState() => _AddAttachmentDialogState();
}

class _AddAttachmentDialogState extends State<_AddAttachmentDialog> {
  late final TextEditingController _nameCtrl;
  late final TextEditingController _urlCtrl;
  bool _uploading = false;

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController(
      text: widget.isImage ? 'Conejita' : 'Documento Prueba',
    );
    _urlCtrl = TextEditingController(
      text: widget.isImage ? 'conejita.jpg' : 'prueba.pdf',
    );
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _urlCtrl.dispose();
    super.dispose();
  }

  void _insert(String snippet) {
    widget.onInsert(snippet);
    Navigator.of(context).pop();
  }

  /// Adjunta [source]: URL remota se incrusta tal cual; archivo local se sube a
  /// Drive cuando hay JWT y el binario existe en disco; en cualquier otro caso
  /// se conserva la referencia local y se avisa al usuario sin romper el flujo.
  Future<void> _attach({
    required String name,
    required String source,
    required bool isImage,
  }) async {
    if (_uploading) return;
    final label = name.trim().isEmpty
        ? (isImage ? 'Imagen' : 'Documento')
        : name.trim();
    final sourcePath = source.trim();
    if (sourcePath.isEmpty) return;
    String snippetFor(String url) =>
        isImage ? '![$label]($url)' : '[$label]($url)';

    if (_isRemoteUrl(sourcePath)) {
      _insert(snippetFor(sourcePath));
      return;
    }

    final token = SessionManager.token;
    final local = kIsWeb ? null : _resolveLocalFile(sourcePath);
    if (token != null && token.isNotEmpty && local != null) {
      setState(() => _uploading = true);
      final result = await _uploadAttachmentToDrive(
        file: local,
        fileName: _attachmentFileNameFrom(sourcePath, isImage: isImage),
        isInline: isImage,
      );
      if (!mounted) return;
      if (result.ok && result.fileUrl != null) {
        widget.onInsert(snippetFor(result.fileUrl!));
        widget.onUploaded?.call(result);
        Navigator.of(context).pop();
        return;
      }
      setState(() => _uploading = false);
      widget.onInsert(snippetFor(sourcePath));
      widget.onWarning?.call(
        result.message ??
            'No se pudo subir a Drive: el adjunto quedó como referencia local.',
      );
      Navigator.of(context).pop();
      return;
    }

    widget.onInsert(snippetFor(sourcePath));
    if (local == null) {
      widget.onWarning?.call(
        'No se encontró el archivo local "$sourcePath": se guardó la ruta como referencia.',
      );
    } else {
      widget.onWarning?.call(
        'Sin sesión activa: el adjunto quedó como referencia local (no se subió a Drive).',
      );
    }
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final title = widget.isImage
        ? 'ADJUNTAR IMAGEN'
        : 'ADJUNTAR DOCUMENTO / PDF';
    final isImage = widget.isImage;

    return AlertDialog(
      backgroundColor: AppColors.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppDimens.radius),
        side: const BorderSide(color: AppColors.border, width: 3),
      ),
      title: Row(
        children: [
          Icon(
            isImage ? Icons.image_rounded : Icons.picture_as_pdf_rounded,
            size: 20,
            color: AppColors.text,
          ),
          const SizedBox(width: 8),
          Text(
            title,
            style: const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ],
      ),
      content: SizedBox(
        width: 440,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text(
                'PRESET RÁPIDO DISPONIBLE:',
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: AppColors.muted,
                  letterSpacing: 0.5,
                ),
              ),
              const SizedBox(height: 6),
              MouseRegion(
                cursor: SystemMouseCursors.click,
                child: GestureDetector(
                  onTap: _uploading
                      ? null
                      : () => _attach(
                          name: isImage ? 'Conejita' : 'Prueba PDF',
                          source: isImage ? 'conejita.jpg' : 'prueba.pdf',
                          isImage: isImage,
                        ),
                  child: Container(
                    padding: const EdgeInsets.all(10),
                    decoration: BoxDecoration(
                      color: isImage
                          ? AppColors.subjectMint
                          : AppColors.accentYellow,
                      border: Border.all(color: AppColors.border, width: 2),
                      borderRadius: BorderRadius.circular(AppDimens.radius),
                      boxShadow: AppShadows.badge,
                    ),
                    child: Row(
                      children: [
                        Icon(
                          isImage
                              ? Icons.pets_rounded
                              : Icons.description_rounded,
                          size: 20,
                          color: AppColors.text,
                        ),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                isImage
                                    ? 'Conejita de prueba (conejita.jpg)'
                                    : 'PDF de prueba (prueba.pdf)',
                                style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w900,
                                  color: AppColors.text,
                                ),
                              ),
                              Text(
                                _uploading
                                    ? 'Subiendo a Google Drive…'
                                    : isImage
                                    ? 'Imagen local · se sube a tu Drive al usar'
                                    : 'PDF local · se sube a tu Drive al usar',
                                style: const TextStyle(
                                  fontSize: 10,
                                  fontWeight: FontWeight.w700,
                                  color: AppColors.muted,
                                ),
                              ),
                            ],
                          ),
                        ),
                        Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 6,
                            vertical: 3,
                          ),
                          decoration: BoxDecoration(
                            color: AppColors.border,
                            borderRadius: BorderRadius.circular(
                              AppDimens.radiusChip,
                            ),
                          ),
                          child: _uploading
                              ? const SizedBox(
                                  width: 10,
                                  height: 10,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : const Text(
                                  '+ USAR',
                                  style: TextStyle(
                                    fontSize: 9,
                                    fontWeight: FontWeight.w900,
                                    color: Colors.white,
                                  ),
                                ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 16),
              const Row(
                children: [
                  Expanded(
                    child: Divider(color: AppColors.border, thickness: 1),
                  ),
                  Padding(
                    padding: EdgeInsets.symmetric(horizontal: 8),
                    child: Text(
                      'O PERSONALIZADO',
                      style: TextStyle(
                        fontSize: 9,
                        fontWeight: FontWeight.w900,
                        color: AppColors.muted,
                      ),
                    ),
                  ),
                  Expanded(
                    child: Divider(color: AppColors.border, thickness: 1),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _nameCtrl,
                decoration: InputDecoration(
                  labelText: isImage
                      ? 'Texto alternativo / Nombre'
                      : 'Nombre del documento',
                  hintText: isImage
                      ? 'Ej. Diagrama de arquitectura'
                      : 'Ej. Guía semana 3',
                  filled: true,
                  fillColor: AppColors.surfaceLow,
                  border: const OutlineInputBorder(
                    borderRadius: BorderRadius.zero,
                    borderSide: BorderSide(color: AppColors.border, width: 2),
                  ),
                ),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: _urlCtrl,
                decoration: InputDecoration(
                  labelText: isImage
                      ? 'Ruta local o URL web de imagen'
                      : 'Ruta local o URL web del PDF',
                  hintText: isImage
                      ? 'conejita.jpg o https://...'
                      : 'prueba.pdf o https://...',
                  filled: true,
                  fillColor: AppColors.surfaceLow,
                  border: const OutlineInputBorder(
                    borderRadius: BorderRadius.zero,
                    borderSide: BorderSide(color: AppColors.border, width: 2),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _uploading ? null : () => Navigator.of(context).pop(),
          child: const Text(
            'CANCELAR',
            style: TextStyle(
              fontWeight: FontWeight.w800,
              color: AppColors.text,
            ),
          ),
        ),
        ElevatedButton(
          style: ElevatedButton.styleFrom(
            backgroundColor: AppColors.border,
            foregroundColor: AppColors.surface,
            shape: const RoundedRectangleBorder(
              borderRadius: BorderRadius.zero,
            ),
          ),
          onPressed: _uploading
              ? null
              : () {
                  final name = _nameCtrl.text.trim();
                  final url = _urlCtrl.text.trim();
                  if (url.isEmpty) return;
                  _attach(name: name, source: url, isImage: isImage);
                },
          child: _uploading
              ? const SizedBox(
                  width: 14,
                  height: 14,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: AppColors.surface,
                  ),
                )
              : const Text(
                  'SUBIR E INSERTAR',
                  style: TextStyle(fontWeight: FontWeight.w900),
                ),
        ),
      ],
    );
  }
}

/// Contenedor de previsualización en tiempo real para adjuntos detectados en el texto.
class _DetectedAttachmentsPreview extends StatelessWidget {
  final String content;
  final ValueChanged<String> onRemoveSnippet;
  final ValueChanged<Map<String, dynamic>> onPreviewResource;

  const _DetectedAttachmentsPreview({
    required this.content,
    required this.onRemoveSnippet,
    required this.onPreviewResource,
  });

  @override
  Widget build(BuildContext context) {
    final attachments = _resourcesFromMarkdown(content);
    if (attachments.isEmpty) return const SizedBox.shrink();

    return Container(
      margin: const EdgeInsets.only(top: 8, bottom: 8),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.badge,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(
                Icons.attach_file_rounded,
                size: 14,
                color: AppColors.text,
              ),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  'ADJUNTOS VINCULADOS (${attachments.length})',
                  style: const TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                    letterSpacing: 0.5,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 6),
              const Text(
                'Toca / (x)',
                style: TextStyle(
                  fontSize: 9,
                  fontWeight: FontWeight.w700,
                  color: AppColors.muted,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              for (final att in attachments)
                _DetectedAttachmentChip(
                  resource: att,
                  onTap: () => onPreviewResource(att),
                  onRemove: () => onRemoveSnippet(att['url'] as String),
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class _DetectedAttachmentChip extends StatelessWidget {
  final Map<String, dynamic> resource;
  final VoidCallback onTap;
  final VoidCallback onRemove;

  const _DetectedAttachmentChip({
    required this.resource,
    required this.onTap,
    required this.onRemove,
  });

  @override
  Widget build(BuildContext context) {
    final isImage = resource['type'] == 'image';
    return Container(
      decoration: BoxDecoration(
        color: isImage ? AppColors.subjectMint : AppColors.accentYellow,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          MouseRegion(
            cursor: SystemMouseCursors.click,
            child: GestureDetector(
              onTap: onTap,
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(
                      isImage
                          ? Icons.image_rounded
                          : Icons.picture_as_pdf_rounded,
                      size: 13,
                      color: AppColors.text,
                    ),
                    const SizedBox(width: 4),
                    ConstrainedBox(
                      constraints: const BoxConstraints(maxWidth: 160),
                      child: Text(
                        resource['name'] as String,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w900,
                          color: AppColors.text,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          Container(width: 1, height: 16, color: AppColors.border),
          MouseRegion(
            cursor: SystemMouseCursors.click,
            child: GestureDetector(
              onTap: onRemove,
              child: const Padding(
                padding: EdgeInsets.symmetric(horizontal: 6, vertical: 4),
                child: Icon(
                  Icons.close_rounded,
                  size: 13,
                  color: AppColors.text,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Detecta adjuntos desde el contenido Markdown de una nota: imágenes en
/// formato `![alt](ruta)` y documentos PDF en `[etiqueta](ruta.pdf)`.
List<Map<String, dynamic>> _resourcesFromMarkdown(String content) {
  final result = <Map<String, dynamic>>[];
  final seen = <String>{};

  final imageRe = RegExp(r'!\[(.*?)\]\((.*?)\)');
  for (final m in imageRe.allMatches(content)) {
    final url = (m.group(2) ?? '').trim();
    if (url.isEmpty) continue;
    if (!seen.add('img::$url')) continue;
    final alt = (m.group(1) ?? '').trim();
    result.add({
      'type': 'image',
      'name': alt.isEmpty ? url : alt,
      'url': url,
      'size': 'Adjunto',
    });
  }

  // Documentos: rutas terminadas en .pdf o enlaces de archivo de Google Drive
  // (los adjuntos subidos con /notes/upload usan la URL webViewLink de Drive,
  // que no expone la extensión en la URL).
  final docRe = RegExp(
    r'\[(.*?)\]\((.*?\.pdf|https?://drive\.google\.com/file/d/[^\s)]+)\)',
    caseSensitive: false,
  );
  for (final m in docRe.allMatches(content)) {
    final url = (m.group(2) ?? '').trim();
    if (url.isEmpty) continue;
    if (!seen.add('doc::$url')) continue;
    final label = (m.group(1) ?? '').trim();
    var name = label.isEmpty ? url : label;
    if (!name.toLowerCase().endsWith('.pdf')) name = '$name.pdf';
    result.add({'type': 'pdf', 'name': name, 'url': url, 'size': 'PDF'});
  }
  return result;
}

/// Resolución robusta de rutas de archivo locales: directa (relativa al cwd,
/// típicamente `front/`), relativa a `../<ruta>` (raíz del repo) o absoluta.
/// Devuelve null si el archivo no existe (o en web, donde no hay dart:io).
File? _resolveLocalFile(String url) {
  if (kIsWeb) return null;
  for (final candidate in <File>[File(url), File('../$url')]) {
    try {
      if (candidate.existsSync()) return candidate;
    } catch (_) {}
  }
  return null;
}

/// Tamaño legible (B/KB/MB) para el resumen técnico del visor de documentos.
String _formatFileSize(int bytes) {
  if (bytes < 1024) return '$bytes B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  return '${(bytes / (1024 * 1024)).toStringAsFixed(2)} MB';
}

bool _isRemoteUrl(String url) {
  final lower = url.toLowerCase();
  return lower.startsWith('http://') || lower.startsWith('https://');
}

/// Diagnóstico de un recurso documental: existencia en disco, ruta absoluta y
/// tamaño real, para el resumen técnico del visor y la ficha del adjunto.
class _ResourceFileInfo {
  final File? localFile;
  final String? absolutePath;
  final int? sizeBytes;
  final bool isRemote;

  const _ResourceFileInfo({
    required this.localFile,
    required this.absolutePath,
    required this.sizeBytes,
    required this.isRemote,
  });

  bool get exists => localFile != null;

  bool get isReady => exists || isRemote;

  factory _ResourceFileInfo.resolve(String url) {
    final local = _resolveLocalFile(url);
    int? size;
    if (local != null) {
      try {
        size = local.lengthSync();
      } catch (_) {}
    }
    return _ResourceFileInfo(
      localFile: local,
      absolutePath: local?.absolute.path,
      sizeBytes: size,
      isRemote: _isRemoteUrl(url),
    );
  }
}

/// Apertura real del documento: en Windows lanza el visor predeterminado con
/// `cmd /c start` sobre la ruta resuelta; en el resto de plataformas usa
/// url_launcher (archivo local o URL remota). Devuelve true si se lanzó.
Future<bool> _openResourceExternally(String url) async {
  final info = _ResourceFileInfo.resolve(url);
  if (!kIsWeb && Platform.isWindows && info.localFile != null) {
    try {
      // En pruebas automatizadas no se abren visores externos reales.
      if (Platform.environment.containsKey('FLUTTER_TEST')) return true;
      final res = await Process.run('cmd', <String>[
        '/c',
        'start',
        '',
        info.localFile!.absolute.path,
      ]);
      return res.exitCode == 0;
    } catch (_) {
      return false;
    }
  }
  try {
    final Uri? target = info.localFile != null && !kIsWeb
        ? Uri.file(info.localFile!.absolute.path)
        : Uri.tryParse(url);
    if (target == null) return false;
    return await launchUrl(target, mode: LaunchMode.externalApplication);
  } catch (_) {
    return false;
  }
}

/// Copia el archivo local al directorio de descargas del sistema y devuelve la
/// ruta destino (null si la plataforma no lo soporta o falla el copiado).
Future<String?> _copyLocalResourceToDownloads(File source) async {
  try {
    final dir = await getDownloadsDirectory();
    if (dir == null || dir.path.isEmpty) return null;
    final name = source.uri.pathSegments.isNotEmpty
        ? source.uri.pathSegments.last
        : 'adjunto';
    final destPath = '${dir.path}${Platform.pathSeparator}$name';
    if (destPath == source.absolute.path) return destPath;
    final dest = await source.copy(destPath);
    return dest.path;
  } catch (_) {
    return null;
  }
}

/// Imagen de recurso: `Image.file` si la ruta existe en disco, `Image.network`
/// para URLs http/https y fallback neobrutalista si nada aplica o falla.
Widget _buildResourceImage({
  required String url,
  required BoxFit fit,
  required Widget Function() fallback,
  Widget Function(BuildContext, Widget, ImageChunkEvent?)? loadingBuilder,
}) {
  final local = _resolveLocalFile(url);
  if (local != null) {
    return Image.file(
      local,
      fit: fit,
      gaplessPlayback: true,
      errorBuilder: (_, _, _) => fallback(),
    );
  }
  final lower = url.toLowerCase();
  if (lower.startsWith('http://') || lower.startsWith('https://')) {
    return Image.network(
      url,
      fit: fit,
      gaplessPlayback: true,
      errorBuilder: (_, _, _) => fallback(),
      loadingBuilder: loadingBuilder,
    );
  }
  return fallback();
}

/// Miniatura de recurso de imagen: borde de tinta de 2 px, sombra dura y
/// errorBuilder neobrutalista si la red falla. Tap abre el visor en grande.
class _ResourceImageThumb extends StatelessWidget {
  final Map<String, dynamic> resource;
  final VoidCallback onTap;
  const _ResourceImageThumb({required this.resource, required this.onTap});

  @override
  Widget build(BuildContext context) {
    Widget fallback() {
      return Container(
        color: AppColors.surfaceLow,
        alignment: Alignment.center,
        child: const Column(
          mainAxisSize: MainAxisSize.min,
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.broken_image_rounded, size: 20, color: AppColors.muted),
            SizedBox(height: 3),
            Text(
              'IMAGEN',
              style: TextStyle(
                fontSize: 8,
                fontWeight: FontWeight.w900,
                color: AppColors.muted,
              ),
            ),
          ],
        ),
      );
    }

    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          width: 104,
          height: 78,
          clipBehavior: Clip.hardEdge,
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.badge,
          ),
          child: _buildResourceImage(
            url: (resource['url'] ?? '') as String,
            fit: BoxFit.cover,
            fallback: fallback,
            loadingBuilder: (context, child, progress) {
              if (progress == null) return child;
              return Container(
                color: AppColors.bg,
                alignment: Alignment.center,
                child: const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(
                    strokeWidth: 2.5,
                    color: AppColors.border,
                  ),
                ),
              );
            },
          ),
        ),
      ),
    );
  }
}

/// Ficha de documento (PDF/DOC): ícono de tipo, nombre, tamaño y acciones de
/// ver/descargar dentro del detalle de la nota.
class _ResourceDocTile extends StatelessWidget {
  final Map<String, dynamic> resource;
  final VoidCallback onView;
  final ValueChanged<Map<String, dynamic>> onDownload;
  const _ResourceDocTile({
    required this.resource,
    required this.onView,
    required this.onDownload,
  });

  @override
  Widget build(BuildContext context) {
    final isPdf = resource['type'] == 'pdf';
    final info = _ResourceFileInfo.resolve((resource['url'] ?? '') as String);
    final sizeLabel = info.sizeBytes != null
        ? _formatFileSize(info.sizeBytes!)
        : '${resource['size'] ?? '—'}';
    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.badge,
      ),
      child: Row(
        children: [
          Container(
            width: 40,
            height: 40,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: isPdf ? AppColors.errorDeep : AppColors.accentBlueDeep,
              border: Border.all(color: AppColors.border, width: 2),
              borderRadius: BorderRadius.circular(AppDimens.radius),
            ),
            child: Icon(
              isPdf ? Icons.picture_as_pdf_rounded : Icons.description_rounded,
              size: 20,
              color: AppColors.surface,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  resource['name'] as String,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w800,
                    fontSize: 12.5,
                    color: AppColors.text,
                    height: 1.2,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  '${isPdf ? 'PDF' : 'DOC'} · $sizeLabel'
                  '${info.exists ? ' · DOCUMENTO LISTO' : ''}',
                  style: const TextStyle(
                    fontWeight: FontWeight.w700,
                    fontSize: 11,
                    color: AppColors.muted,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(width: 8),
          _DocAction(label: 'VER', onTap: onView, fill: AppColors.accentYellow),
          const SizedBox(width: 6),
          _DocAction(
            label: 'DESCARGAR',
            onTap: () => onDownload(resource),
            fill: AppColors.border,
            foreground: AppColors.surface,
          ),
        ],
      ),
    );
  }
}

/// Acción compacta de las fichas de documento (borde 1.5 px + sombra dura).
class _DocAction extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  final Color fill;
  final Color foreground;
  const _DocAction({
    required this.label,
    required this.onTap,
    this.fill = AppColors.surface,
    this.foreground = AppColors.text,
  });

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
          decoration: BoxDecoration(
            color: fill,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Text(
            label,
            style: TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 9,
              color: foreground,
            ),
          ),
        ),
      ),
    );
  }
}

/// Fila etiqueta/valor del resumen técnico de un documento adjunto.
Widget _resourceInfoRow(String label, String value, {bool mono = false}) {
  return Padding(
    padding: const EdgeInsets.only(bottom: 7),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 76,
          child: Text(
            label,
            style: const TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.5,
              color: AppColors.muted,
            ),
          ),
        ),
        Expanded(
          child: Text(
            value,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              fontFamily: mono ? 'monospace' : null,
              color: AppColors.text,
            ),
          ),
        ),
      ],
    ),
  );
}

/// Visor de recurso: imagen grande (con fallback neobrutalista) o ficha técnica
/// del documento con apertura real en el visor del sistema y copia a descargas.
class _ResourceViewerDialog extends StatelessWidget {
  final Map<String, dynamic> resource;
  const _ResourceViewerDialog({required this.resource});

  bool get _isWindows => !kIsWeb && Platform.isWindows;

  String get _url => (resource['url'] ?? '') as String;

  Future<void> _openInSystemViewer(BuildContext context, String url) async {
    final ok = await _openResourceExternally(url);
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: Colors.black,
        content: Text(
          ok
              ? 'Abriendo ${resource['name']} en el visor del sistema…'
              : 'No se pudo abrir el documento. Verifica que el archivo exista en disco.',
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  Future<void> _download(BuildContext context, _ResourceFileInfo info) async {
    String message;
    if (info.localFile != null) {
      final dest = await _copyLocalResourceToDownloads(info.localFile!);
      message = dest != null
          ? 'Documento copiado a descargas: $dest'
          : 'El archivo ya está en disco: ${info.localFile!.absolute.path}';
    } else if (info.isRemote) {
      unawaited(_openResourceExternally(_url));
      message = 'Descarga iniciada: ${resource['name']}';
    } else {
      message = 'No se encontró el archivo para descargar: ${resource['name']}';
    }
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: Colors.black,
        content: Text(
          message,
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isImage = resource['type'] == 'image';
    final info = _ResourceFileInfo.resolve(_url);
    return AlertDialog(
      backgroundColor: AppColors.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppDimens.radius),
        side: const BorderSide(color: AppColors.border, width: 3),
      ),
      title: Text(
        resource['name'] as String,
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(
          fontWeight: FontWeight.w900,
          fontSize: 14,
          color: AppColors.text,
        ),
      ),
      content: SingleChildScrollView(
        child: isImage
            ? Container(
                width: double.infinity,
                height: 480,
                constraints: const BoxConstraints(maxWidth: 900),
                clipBehavior: Clip.hardEdge,
                decoration: BoxDecoration(
                  color: AppColors.surfaceLow,
                  border: Border.all(
                    color: AppColors.border,
                    width: AppDimens.borderWidth,
                  ),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: AppShadows.dialog,
                ),
                child: InteractiveViewer(
                  minScale: 0.8,
                  maxScale: 4.0,
                  child: _buildResourceImage(
                    url: _url,
                    fit: BoxFit.contain,
                    fallback: () => Container(
                      color: AppColors.surfaceLow,
                      alignment: Alignment.center,
                      child: const Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(
                            Icons.broken_image_rounded,
                            size: 40,
                            color: AppColors.muted,
                          ),
                          SizedBox(height: 8),
                          Text(
                            'IMAGEN NO DISPONIBLE',
                            style: TextStyle(
                              fontWeight: FontWeight.w900,
                              fontSize: 11,
                              color: AppColors.muted,
                            ),
                          ),
                        ],
                      ),
                    ),
                    loadingBuilder: (context, child, progress) {
                      if (progress == null) return child;
                      return Container(
                        color: AppColors.bg,
                        alignment: Alignment.center,
                        child: const CircularProgressIndicator(
                          color: AppColors.border,
                        ),
                      );
                    },
                  ),
                ),
              )
            : _buildDocInfo(info),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text(
            'CERRAR',
            style: TextStyle(
              fontWeight: FontWeight.w800,
              color: AppColors.text,
            ),
          ),
        ),
        if (!isImage)
          TextButton(
            onPressed: () => _download(context, info),
            child: const Text(
              'DESCARGAR',
              style: TextStyle(
                fontWeight: FontWeight.w800,
                color: AppColors.text,
              ),
            ),
          ),
        ElevatedButton(
          style: ElevatedButton.styleFrom(
            backgroundColor: isImage
                ? AppColors.accentYellow
                : AppColors.accentBlueDeep,
            foregroundColor: isImage ? AppColors.text : AppColors.surface,
            side: const BorderSide(color: AppColors.border, width: 2),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(AppDimens.radius),
            ),
          ),
          onPressed: () => isImage
              ? _download(context, info)
              : _openInSystemViewer(context, _url),
          child: Text(
            isImage
                ? 'DESCARGAR'
                : (_isWindows
                      ? 'ABRIR EN VISOR DE WINDOWS'
                      : 'ABRIR EN VISOR DEL SISTEMA'),
            style: const TextStyle(fontWeight: FontWeight.w900),
          ),
        ),
      ],
    );
  }

  /// Resumen técnico del documento: nombre, tamaño real, ruta absoluta y badge
  /// de disponibilidad, más el tipo de acceso (local verificado o remoto).
  Widget _buildDocInfo(_ResourceFileInfo info) {
    final isPdf = resource['type'] == 'pdf';
    final sizeLabel = info.sizeBytes != null
        ? _formatFileSize(info.sizeBytes!)
        : info.isRemote
        ? 'Remoto'
        : '${resource['size'] ?? '—'}';
    final pathLabel =
        info.absolutePath ??
        (info.isRemote ? _url : 'No se encontró el archivo en disco');
    final badgeLabel = info.exists
        ? 'DOCUMENTO LISTO'
        : info.isRemote
        ? 'DOCUMENTO REMOTO'
        : 'ARCHIVO NO ENCONTRADO';
    final badgeColor = info.exists
        ? AppColors.subjectMint
        : info.isRemote
        ? AppColors.accentYellow
        : const Color(0xFFFFD6D2);

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.bg,
        border: Border.all(color: AppColors.border, width: 2),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              Container(
                width: 40,
                height: 40,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: isPdf ? AppColors.errorDeep : AppColors.accentBlueDeep,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                ),
                child: Icon(
                  isPdf
                      ? Icons.picture_as_pdf_rounded
                      : Icons.description_rounded,
                  size: 20,
                  color: AppColors.surface,
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  resource['name'] as String,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 13,
                    color: AppColors.text,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          _resourceInfoRow('TAMAÑO', sizeLabel),
          _resourceInfoRow('RUTA', pathLabel, mono: true),
          _resourceInfoRow(
            'ORIGEN',
            info.exists
                ? 'Archivo local verificado en disco'
                : info.isRemote
                ? 'URL remota (visor/descarga externa)'
                : 'Sin archivo asociado en disco',
          ),
          const SizedBox(height: 2),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: badgeColor,
              border: Border.all(color: AppColors.border, width: 1.5),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              boxShadow: AppShadows.badge,
            ),
            child: Text(
              badgeLabel,
              style: const TextStyle(
                fontSize: 9,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.5,
                color: AppColors.text,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Cuerpo de nota renderizado como Markdown visual neobrutalista: encabezados,
/// negrita/cursiva, listas con bullets sólidos, bloques de código, blockquotes,
/// imágenes embebidas (archivo local o URL remota) y chips de PDF.
class _NeobrutalistMarkdownBody extends StatelessWidget {
  final String content;
  final ValueChanged<Map<String, dynamic>> onOpenResource;
  const _NeobrutalistMarkdownBody({
    required this.content,
    required this.onOpenResource,
  });

  static const TextStyle _paragraphStyle = TextStyle(
    fontSize: 14,
    height: 1.5,
    color: AppColors.text,
  );

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: _buildBlocks(content),
    );
  }

  List<Widget> _buildBlocks(String content) {
    final lines = content.split('\n');
    final widgets = <Widget>[];
    final paragraph = <String>[];

    void flushParagraph() {
      if (paragraph.isNotEmpty) {
        widgets.add(_paragraph(paragraph.join(' ')));
        paragraph.clear();
      }
    }

    var i = 0;
    while (i < lines.length) {
      final raw = lines[i];
      final line = raw.trim();

      // Bloque de código delimitado por triple backtick.
      if (line.startsWith('```')) {
        flushParagraph();
        final buf = <String>[];
        i++;
        while (i < lines.length && !lines[i].trimLeft().startsWith('```')) {
          buf.add(lines[i]);
          i++;
        }
        i++;
        widgets.add(_codeBlock(buf.join('\n')));
        continue;
      }

      // Encabezados # / ## / ### / #### (H1-H6; H4+ se muestran como
      // subtítulo técnico compacto sin las almohadillas).
      final heading = RegExp(r'^(#{1,6})\s+(.+)$').firstMatch(line);
      if (heading != null) {
        flushParagraph();
        widgets.add(_heading(heading.group(1)!.length, heading.group(2)!));
        i++;
        continue;
      }

      // Cita / blockquote '> '.
      if (line.startsWith('> ')) {
        flushParagraph();
        widgets.add(_blockquote(line.substring(2)));
        i++;
        continue;
      }

      // Imagen standalone ![alt](url).
      final standaloneImg = RegExp(r'^!\[(.*?)\]\((.*?)\)$').firstMatch(line);
      if (standaloneImg != null) {
        flushParagraph();
        widgets.add(
          _blockImage(standaloneImg.group(1) ?? '', standaloneImg.group(2)!),
        );
        i++;
        continue;
      }

      // Documento standalone [label](url.pdf).
      final standaloneDoc = RegExp(
        r'^\[(.*?)\]\((.*?\.pdf)\)$',
        caseSensitive: false,
      ).firstMatch(line);
      if (standaloneDoc != null) {
        flushParagraph();
        widgets.add(
          _blockDoc(standaloneDoc.group(1) ?? '', standaloneDoc.group(2)!),
        );
        i++;
        continue;
      }

      // Item de lista '- ', '* ' o '1. '.
      final listItem = RegExp(r'^([-*]|\d+\.)\s+(.+)$').firstMatch(line);
      if (listItem != null) {
        flushParagraph();
        final isOrdered = RegExp(r'^\d+\.$').hasMatch(listItem.group(1)!);
        widgets.add(_listItem(listItem.group(2)!, isOrdered));
        i++;
        continue;
      }

      if (line.isEmpty) {
        flushParagraph();
        i++;
        continue;
      }

      paragraph.add(line);
      i++;
    }
    flushParagraph();
    return widgets;
  }

  Widget _paragraph(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Text.rich(
        TextSpan(style: _paragraphStyle, children: _parseInlineSpans(text)),
      ),
    );
  }

  Widget _heading(int level, String text) {
    // H4-H6: subtítulo técnico compacto con barra de acento y tipografía
    // monoespaciada neobrutalista (sin mostrar '####').
    if (level >= 4) {
      return Padding(
        padding: const EdgeInsets.only(top: 8, bottom: 6),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              width: 4,
              height: 14,
              margin: const EdgeInsets.only(top: 2, right: 8),
              color: level == 4 ? AppColors.accentBlueDeep : AppColors.muted,
            ),
            Expanded(
              child: Text.rich(
                TextSpan(
                  style: const TextStyle(
                    fontSize: 12.5,
                    height: 1.3,
                    fontWeight: FontWeight.w900,
                    fontFamily: 'monospace',
                    letterSpacing: 0.3,
                    color: AppColors.text,
                  ),
                  children: _parseInlineSpans(text),
                ),
              ),
            ),
          ],
        ),
      );
    }
    final fontSize = level == 1
        ? 20.0
        : level == 2
        ? 17.0
        : 15.0;
    final weight = level <= 2 ? FontWeight.w900 : FontWeight.w800;
    return Padding(
      padding: const EdgeInsets.only(top: 6, bottom: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text.rich(
            TextSpan(
              style: TextStyle(
                fontSize: fontSize,
                fontWeight: weight,
                color: AppColors.text,
              ),
              children: _parseInlineSpans(text),
            ),
          ),
          if (level == 1) ...[
            const SizedBox(height: 4),
            Container(height: 3, width: 48, color: AppColors.border),
          ],
        ],
      ),
    );
  }

  Widget _listItem(String text, bool ordered) {
    return Padding(
      padding: const EdgeInsets.only(left: 10, bottom: 5),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 9,
            height: 9,
            margin: const EdgeInsets.only(top: 6, right: 9),
            decoration: BoxDecoration(
              color: ordered ? AppColors.border : AppColors.text,
              borderRadius: BorderRadius.circular(ordered ? 5 : 2),
            ),
          ),
          Expanded(
            child: Text.rich(
              TextSpan(
                style: _paragraphStyle,
                children: _parseInlineSpans(text),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _blockquote(String text) {
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: const BoxDecoration(
        color: Color(0xFFF5F0E8),
        border: Border(
          left: BorderSide(color: AppColors.border, width: 3),
          top: BorderSide(color: AppColors.border, width: 1),
          right: BorderSide(color: AppColors.border, width: 1),
          bottom: BorderSide(color: AppColors.border, width: 1),
        ),
      ),
      child: Text.rich(
        TextSpan(
          style: _paragraphStyle.copyWith(fontStyle: FontStyle.italic),
          children: _parseInlineSpans(text),
        ),
      ),
    );
  }

  Widget _blockImage(String alt, String url) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Align(
        alignment: Alignment.center,
        child: ConstrainedBox(
          constraints: const BoxConstraints(
            minHeight: 250,
            maxHeight: 520,
            maxWidth: 850,
          ),
          child: MouseRegion(
            cursor: SystemMouseCursors.click,
            child: GestureDetector(
              onTap: () => onOpenResource({
                'type': 'image',
                'name': alt.isEmpty ? url : alt,
                'url': url,
                'size': 'Adjunto',
              }),
              child: Container(
                clipBehavior: Clip.hardEdge,
                decoration: BoxDecoration(
                  color: AppColors.surfaceLow,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: AppShadows.badge,
                ),
                child: Stack(
                  children: [
                    _buildResourceImage(
                      url: url,
                      fit: BoxFit.contain,
                      fallback: () => Container(
                        width: 420,
                        height: 260,
                        color: AppColors.surfaceLow,
                        alignment: Alignment.center,
                        child: const Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.broken_image_rounded,
                              size: 36,
                              color: AppColors.muted,
                            ),
                            SizedBox(height: 6),
                            Text(
                              'IMAGEN NO DISPONIBLE',
                              style: TextStyle(
                                fontSize: 10,
                                fontWeight: FontWeight.w900,
                                color: AppColors.muted,
                              ),
                            ),
                          ],
                        ),
                      ),
                      loadingBuilder: (context, child, progress) {
                        if (progress == null) return child;
                        return Container(
                          width: 420,
                          height: 260,
                          color: AppColors.surfaceLow,
                          alignment: Alignment.center,
                          child: const SizedBox(
                            width: 22,
                            height: 22,
                            child: CircularProgressIndicator(
                              strokeWidth: 2.5,
                              color: AppColors.border,
                            ),
                          ),
                        );
                      },
                    ),
                    Positioned(
                      bottom: 6,
                      right: 6,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 7,
                          vertical: 4,
                        ),
                        decoration: BoxDecoration(
                          color: AppColors.accentYellow,
                          border: Border.all(
                            color: AppColors.border,
                            width: 1.5,
                          ),
                          borderRadius: BorderRadius.circular(
                            AppDimens.radiusChip,
                          ),
                          boxShadow: AppShadows.badge,
                        ),
                        child: const Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.zoom_in_rounded,
                              size: 13,
                              color: AppColors.text,
                            ),
                            SizedBox(width: 4),
                            Text(
                              'AMPLIAR',
                              style: TextStyle(
                                fontSize: 9,
                                fontWeight: FontWeight.w900,
                                color: AppColors.text,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                    if (alt.isNotEmpty)
                      Positioned(
                        top: 6,
                        left: 6,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 7,
                            vertical: 3,
                          ),
                          decoration: BoxDecoration(
                            color: AppColors.surface,
                            border: Border.all(
                              color: AppColors.border,
                              width: 1.5,
                            ),
                            borderRadius: BorderRadius.circular(
                              AppDimens.radiusChip,
                            ),
                          ),
                          child: Text(
                            alt,
                            style: const TextStyle(
                              fontSize: 9,
                              fontWeight: FontWeight.w900,
                              color: AppColors.text,
                            ),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _blockDoc(String label, String url) {
    final name = label.toLowerCase().endsWith('.pdf') ? label : '$label.pdf';
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
        child: GestureDetector(
          onTap: () => onOpenResource({
            'type': 'pdf',
            'name': name,
            'url': url,
            'size': 'PDF',
          }),
          child: Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: AppColors.surface,
              border: Border.all(color: AppColors.border, width: 2),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: AppShadows.badge,
            ),
            child: Row(
              children: [
                Container(
                  width: 36,
                  height: 36,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: AppColors.errorDeep,
                    border: Border.all(color: AppColors.border, width: 1.5),
                    borderRadius: BorderRadius.circular(AppDimens.radius),
                  ),
                  child: const Icon(
                    Icons.picture_as_pdf_rounded,
                    size: 18,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        name,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w900,
                          color: AppColors.text,
                        ),
                      ),
                      const Text(
                        'Documento PDF adjunto · Toca para abrir visor',
                        style: TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w700,
                          color: AppColors.muted,
                        ),
                      ),
                    ],
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 5,
                  ),
                  decoration: BoxDecoration(
                    color: AppColors.accentYellow,
                    border: Border.all(color: AppColors.border, width: 1.5),
                    borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                    boxShadow: AppShadows.badge,
                  ),
                  child: const Text(
                    'VER',
                    style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w900,
                      color: AppColors.text,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _codeBlock(String code) {
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.only(bottom: 8),
      decoration: BoxDecoration(
        color: const Color(0xFFF4F4F0),
        border: Border.all(color: AppColors.border, width: 2),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.badge,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            decoration: const BoxDecoration(
              color: AppColors.surfaceLow,
              border: Border(
                bottom: BorderSide(color: AppColors.border, width: 1.5),
              ),
            ),
            child: Row(
              children: [
                const Icon(
                  Icons.terminal_rounded,
                  size: 14,
                  color: AppColors.text,
                ),
                const SizedBox(width: 6),
                const Text(
                  'CÓDIGO',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                    letterSpacing: 0.5,
                  ),
                ),
                const Spacer(),
                Builder(
                  builder: (ctx) => InkWell(
                    onTap: () {
                      Clipboard.setData(ClipboardData(text: code));
                      ScaffoldMessenger.of(ctx).showSnackBar(
                        const SnackBar(
                          backgroundColor: Colors.black,
                          duration: Duration(seconds: 1),
                          content: Text(
                            'Código copiado al portapapeles',
                            style: TextStyle(
                              color: Colors.white,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        ),
                      );
                    },
                    child: const Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          Icons.copy_rounded,
                          size: 12,
                          color: AppColors.text,
                        ),
                        SizedBox(width: 4),
                        Text(
                          'COPIAR',
                          style: TextStyle(
                            fontSize: 9,
                            fontWeight: FontWeight.w900,
                            color: AppColors.text,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(10),
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: SelectableText(
                code,
                style: const TextStyle(
                  fontFamily: 'monospace',
                  fontSize: 12,
                  height: 1.4,
                  color: AppColors.text,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Divide una línea en spans con estilos inline: **negrita**, *cursiva*,
  /// `código`, imágenes `![alt](ruta)` y chips de enlace `[label](archivo.pdf)`.
  List<InlineSpan> _parseInlineSpans(String text) {
    final spans = <InlineSpan>[];
    final re = RegExp(
      r'(\*\*.+?\*\*|\*[^*\n]+?\*|!\[[^\]]*\]\([^)\s]+\)|\[[^\]]+\]\([^)\s]+\.pdf\)|`[^`\n]+`)',
      caseSensitive: false,
    );
    var last = 0;
    for (final m in re.allMatches(text)) {
      if (m.start > last) {
        spans.add(TextSpan(text: text.substring(last, m.start)));
      }
      final token = m.group(0)!;
      if (token.startsWith('**')) {
        spans.add(
          TextSpan(
            text: token.substring(2, token.length - 2),
            style: const TextStyle(
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        );
      } else if (token.startsWith('*')) {
        spans.add(
          TextSpan(
            text: token.substring(1, token.length - 1),
            style: const TextStyle(
              fontStyle: FontStyle.italic,
              color: AppColors.text,
            ),
          ),
        );
      } else if (token.startsWith('`')) {
        spans.add(
          TextSpan(
            text: token.substring(1, token.length - 1),
            style: const TextStyle(
              fontFamily: 'monospace',
              fontSize: 12,
              backgroundColor: Color(0xFFF4F4F0),
              color: AppColors.text,
            ),
          ),
        );
      } else if (token.startsWith('![')) {
        final img = RegExp(r'!\[([^\]]*)\]\(([^)\s]+)\)').firstMatch(token);
        if (img != null) {
          spans.add(
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: _inlineImage(img.group(1) ?? '', img.group(2)!),
            ),
          );
        }
      } else if (token.startsWith('[')) {
        final link = RegExp(
          r'\[([^\]]+)\]\(([^)\s]+\.pdf)\)',
        ).firstMatch(token);
        if (link != null) {
          spans.add(
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: _pdfChip(link.group(1)!, link.group(2)!),
            ),
          );
        }
      }
      last = m.end;
    }
    if (last < text.length) {
      spans.add(TextSpan(text: text.substring(last)));
    }
    return spans;
  }

  Widget _inlineImage(String alt, String url) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: () => onOpenResource({
          'type': 'image',
          'name': alt.isEmpty ? url : alt,
          'url': url,
          'size': 'Adjunto',
        }),
        child: Container(
          width: 200,
          height: 132,
          margin: const EdgeInsets.symmetric(vertical: 4),
          clipBehavior: Clip.hardEdge,
          decoration: BoxDecoration(
            color: AppColors.surfaceLow,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.badge,
          ),
          child: _buildResourceImage(
            url: url,
            fit: BoxFit.contain,
            fallback: () => const Center(
              child: Icon(
                Icons.broken_image_rounded,
                size: 24,
                color: AppColors.muted,
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _pdfChip(String label, String url) {
    final name = label.toLowerCase().endsWith('.pdf') ? label : '$label.pdf';
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: () => onOpenResource({
          'type': 'pdf',
          'name': name,
          'url': url,
          'size': 'PDF',
        }),
        child: Container(
          margin: const EdgeInsets.symmetric(horizontal: 2, vertical: 2),
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
          decoration: BoxDecoration(
            color: AppColors.accentYellow,
            border: Border.all(color: AppColors.border, width: 1.5),
            borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            boxShadow: AppShadows.badge,
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.picture_as_pdf_rounded,
                size: 14,
                color: AppColors.text,
              ),
              const SizedBox(width: 5),
              Flexible(
                child: Text(
                  name,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                  ),
                ),
              ),
              const SizedBox(width: 4),
              const Icon(
                Icons.open_in_new_rounded,
                size: 12,
                color: AppColors.text,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
