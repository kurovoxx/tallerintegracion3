import 'dart:async';
import 'dart:convert';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show defaultTargetPlatform, kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;

import '../../core/database/app_database.dart';
import '../../core/database/local_notes_repository.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

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
  String _currentQuery = '';
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
        await _repo!.getAllNotes();
      } catch (e) {
        debugPrint('Drift init falló, fallback a memoria: $e');
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
        if (mounted) setState(() => _isLoading = false);
        return _filtered;
      }

      final existing = await _repo!.getAllNotes();
      if (existing.isEmpty) {
        final demo = _demoNotes();
        for (final n in demo) {
          await _repo!.upsertNote(n);
          _likesCount[n.id] = (int.tryParse(n.id) ?? 1) * 2;
        }
        await _syncFromBackend();
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
        setState(() {
          _error = e.toString();
          _isLoading = false;
          if (_filtered.isEmpty && _memoryFallback.isEmpty) {
            _useMemoryFallback = true;
            _memoryFallback = _demoNotes();
            _filtered = _applyFilters(
              _memoryFallback,
              _currentQuery,
              _selectedFilter,
            );
            _error = null;
          }
        });
      }
      return _filtered;
    }
  }

  String get notesBaseUrl {
    if (!kIsWeb && Platform.isAndroid) {
      return 'http://10.0.2.2:8082';
    }
    return 'http://localhost:8082';
  }

  Future<void> _syncFromBackend() async {
    if (_useMemoryFallback || _repo == null) return;
    if (mounted) setState(() => _isSyncingBackend = true);
    try {
      final token = SessionManager.token;
      final headers = <String, String>{'Content-Type': 'application/json'};
      if (token != null && token.isNotEmpty) {
        headers['Authorization'] = 'Bearer $token';
      }
      final res = await http
          .get(Uri.parse('$notesBaseUrl/notes/me?limit=50'), headers: headers)
          .timeout(const Duration(seconds: 5));
      if (res.statusCode == 200) {
        final data = jsonDecode(utf8.decode(res.bodyBytes));
        final List notes = data['notes'] ?? [];
        for (final n in notes) {
          try {
            final remoteId = n['id'] as String?;
            if (remoteId != null && remoteId.isNotEmpty) {
              _ownedNoteIds.add(remoteId);
            }
            await _repo!.upsertNote(
              LocalNote(
                id: n['id'] as String,
                title: n['title'] as String? ?? 'Sin título',
                content: n['content'] as String? ?? '',
                visibility: n['visibility'] as String? ?? 'private',
                updatedAt: n['updated_at'] != null
                    ? DateTime.tryParse(n['updated_at'].toString()) ??
                          DateTime.now()
                    : DateTime.now(),
              ),
            );
          } catch (_) {}
        }
      }
    } catch (e) {
      debugPrint('Backend sync falló (offline): $e');
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
      final token = SessionManager.token;
      final headers = <String, String>{'Content-Type': 'application/json'};
      if (token != null && token.isNotEmpty) {
        headers['Authorization'] = 'Bearer $token';
      }
      final res = await http
          .post(Uri.parse(url), headers: headers, body: jsonEncode(body))
          .timeout(const Duration(seconds: 8));
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
        final all = await _repo!.getAllNotes();
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

  /// Lectura híbrida: GET /notes/:id. 200 trae content/title del Drive del autor.
  Future<_RemoteResult> _fetchHybridContent(LocalNote note) async {
    final url = '$notesBaseUrl/notes/${note.id}';
    try {
      final res = await http
          .get(Uri.parse(url), headers: _authHeaders())
          .timeout(const Duration(seconds: 8));
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
      final res = await http
          .patch(
            Uri.parse(url),
            headers: _authHeaders(),
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 8));
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
      final res = await http
          .delete(Uri.parse(url), headers: _authHeaders())
          .timeout(const Duration(seconds: 8));
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
        await _repo!.upsertNote(note);
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
      final res = await http
          .post(Uri.parse(url), headers: _authHeaders())
          .timeout(const Duration(seconds: 8));
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
        base = await _repo!.searchNotesFts(query);
        // Si la búsqueda FTS devuelve vacío con query vacío, usar getAll
        if (query.trim().isEmpty) {
          base = await _repo!.getAllNotes();
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

  void _openNoteViewer(LocalNote note) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => DraggableScrollableSheet(
        initialChildSize: 0.75,
        minChildSize: 0.5,
        maxChildSize: 0.95,
        builder: (context, scroll) => _NoteDetailSheet(
          scrollController: scroll,
          note: note,
          isMine: _isMine(note),
          tag: _noteTags[note.id] ?? 'General',
          likesCount: _likesCount[note.id] ?? 0,
          isLiked: _isLiked[note.id] ?? false,
          isSaved: _isSaved[note.id] ?? false,
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
    final titleCtrl = TextEditingController();
    final contentCtrl = TextEditingController();
    String visibility = 'private';
    showDialog(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
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
          content: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: titleCtrl,
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
                  controller: contentCtrl,
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
                const SizedBox(height: 12),
                Row(
                  children: [
                    Expanded(
                      child: _VisibilityOption(
                        label: 'PÚBLICO',
                        active: visibility == 'public',
                        onTap: () =>
                            setDialogState(() => visibility = 'public'),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: _VisibilityOption(
                        label: 'PRIVADO',
                        active: visibility == 'private',
                        onTap: () =>
                            setDialogState(() => visibility = 'private'),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
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
                backgroundColor: Colors.black,
                foregroundColor: Colors.white,
                shape: const RoundedRectangleBorder(
                  borderRadius: BorderRadius.zero,
                ),
              ),
              onPressed: () async {
                if (titleCtrl.text.trim().isEmpty) return;
                final navigator = Navigator.of(context);
                final note = LocalNote(
                  id: DateTime.now().millisecondsSinceEpoch.toString(),
                  title: titleCtrl.text.trim(),
                  content: contentCtrl.text.trim(),
                  visibility: visibility,
                  updatedAt: DateTime.now(),
                );
                _noteTags[note.id] = visibility == 'public'
                    ? 'General'
                    : 'Privado';
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
                navigator.pop();
                // Reemplazar el id temporal por el UUID real del backend (201).
                // No bloquea: el diálogo ya se cerró; al llegar el UUID se renombra en local.
                unawaited(
                  _createNoteOnBackend(
                    title: note.title,
                    content: note.content,
                    visibility: note.visibility,
                  ).then((realId) async {
                    if (realId != null && realId.isNotEmpty) {
                      await _replaceLocalId(tempId, realId);
                    }
                  }),
                );
              },
              child: const Text(
                'GUARDAR',
                style: TextStyle(fontWeight: FontWeight.w900),
              ),
            ),
          ],
        ),
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
                      'Búsqueda offline instantánea con FTS5',
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
        return Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
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
                style: TextStyle(
                  fontWeight: FontWeight.w600,
                  color: Color(0xFF555555),
                  fontSize: 12,
                ),
              ),
            ],
          ),
        );
      }
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
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
                style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12),
              ),
            ),
          ],
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
      } else if (res.code == 'note_unavailable' ||
          (res.message ?? '').contains('no disponible') ||
          res.status == 404) {
        _driveWarning =
            '⚠️ Nota no disponible en almacenamiento remoto (Google Drive)';
      } else if (res.status == 403) {
        _driveWarning = 'Acceso denegado a esta nota en el servidor.';
      } else {
        _driveWarning =
            'No se pudo cargar la versión del servidor (${res.status}). Se muestra la copia local.';
      }
    });
  }

  /// Editar solo si la lectura remota no reportó error: con el banner de
  /// advertencia visible (Drive no disponible) se bloquea con aviso.
  /// Eliminar permanece habilitado para limpiar la referencia local.
  void _onEditTap() {
    if (_driveWarning != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          backgroundColor: Colors.black,
          content: Text(
            'No se puede editar: la nota no existe en Google Drive.',
            style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800),
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
    setState(() {
      _editing = true;
      _editError = null;
    });
  }

  void _cancelEdit() {
    _disposeEditControllers();
    setState(() {
      _editing = false;
      _editError = null;
    });
  }

  void _disposeEditControllers() {
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
        _disposeEditControllers();
        _editing = false;
        break;
      case _EditSaveOutcome.forbidden:
        _editError = 'Solo el autor puede editar esta nota.';
        break;
      case _EditSaveOutcome.unavailable:
        _editError =
            '⚠️ Nota no disponible en almacenamiento remoto (Google Drive)';
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
                : 'No se pudo eliminar (${res.status}).',
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
    if (res.message != null && res.message!.isNotEmpty) return res.message!;
    return 'No se pudo guardar la copia (${res.status}).';
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

  @override
  Widget build(BuildContext context) {
    return Container(
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
                    Container(
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
                        style: const TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w800,
                          color: Colors.black,
                        ),
                      ),
                    ),
                    const Spacer(),
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
                  Text(
                    _content,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: Colors.black,
                      height: 1.5,
                    ),
                  ),
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
