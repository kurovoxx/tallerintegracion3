import 'dart:async';
import 'dart:convert';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;

import '../../core/database/app_database.dart';
import '../../core/database/local_notes_repository.dart';
import '../../core/services/session_manager.dart';

class AllNotesScreen extends StatefulWidget {
  const AllNotesScreen({super.key});

  @override
  State<AllNotesScreen> createState() => _AllNotesScreenState();
}

class _AllNotesScreenState extends State<AllNotesScreen> {
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
      LocalNote(id: '1', title: 'Cálculo - Límites y derivadas', content: 'Apuntes de cálculo diferencial: límites, continuidad y reglas de derivación. **Markdown** con fórmulas.\n\n- Definición epsilon-delta\n- Regla de la cadena', visibility: 'private', updatedAt: now),
      LocalNote(id: '2', title: 'Estructuras de Datos - Árboles', content: 'Árboles binarios, AVL y recorridos preorden, inorden, postorden.\n\n```\n   1\n  / \\\n 2   3\n```', visibility: 'private', updatedAt: now.subtract(const Duration(hours: 2))),
      LocalNote(id: '3', title: 'Bases de Datos - SQL Joins', content: 'Joins internos, externos, subconsultas y optimización con índices.\n\nSELECT * FROM users JOIN orders ON users.id = orders.user_id;', visibility: 'public', updatedAt: now.subtract(const Duration(days: 1))),
      LocalNote(id: '4', title: 'Redes - Modelo OSI', content: 'Capas OSI y TCP/IP, encapsulamiento y protocolos.\n\n7 Aplicación\n6 Presentación\n5 Sesión', visibility: 'public', updatedAt: now.subtract(const Duration(days: 2))),
      LocalNote(id: '5', title: 'Frontend - NestJS MVC', content: 'Patrón MVC en NestJS con controladores, servicios y módulos.\n\n```ts\n@Controller(\'notes\')\nexport class NotesController {}\n```', visibility: 'public', updatedAt: now.subtract(const Duration(days: 3))),
      LocalNote(id: '6', title: 'Cálculo - Integrales dobles', content: 'Integrales dobles en coordenadas polares y cambio de variables.\n\n∫∫ f(x,y) dA', visibility: 'private', updatedAt: now.subtract(const Duration(days: 4))),
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
        _filtered = _applyFilters(_memoryFallback, _currentQuery, _selectedFilter);
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
            _filtered = _applyFilters(_memoryFallback, _currentQuery, _selectedFilter);
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
        final data = jsonDecode(res.body);
        final List notes = data['notes'] ?? [];
        for (final n in notes) {
          try {
            await _repo!.upsertNote(LocalNote(
              id: n['id'] as String,
              title: n['title'] as String? ?? 'Sin título',
              content: n['content'] as String? ?? '',
              visibility: n['visibility'] as String? ?? 'private',
              updatedAt: n['updated_at'] != null ? DateTime.tryParse(n['updated_at'].toString()) ?? DateTime.now() : DateTime.now(),
            ));
          } catch (_) {}
        }
      }
    } catch (e) {
      debugPrint('Backend sync falló (offline): $e');
    } finally {
      if (mounted) setState(() => _isSyncingBackend = false);
    }
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

  List<LocalNote> _applyFilters(List<LocalNote> notes, String query, String filter) {
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
        return n.title.toLowerCase().contains(lower) || n.content.toLowerCase().contains(lower) || tag.contains(lower);
      }).toList();
    }
    return res;
  }

  String _formatDate(DateTime d) => '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';

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
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.zero, side: const BorderSide(color: Colors.black, width: 2)),
        content: Text(
          (_isSaved[note.id] ?? false) ? 'Nota guardada localmente' : 'Nota removida de guardados',
          style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w800),
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
        decoration: BoxDecoration(color: Colors.white, border: Border.all(color: Colors.black, width: 2), borderRadius: const BorderRadius.vertical(top: Radius.circular(0)), boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(4, 4))]),
        padding: const EdgeInsets.all(20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Center(child: Container(width: 40, height: 4, color: Colors.black)),
            const SizedBox(height: 16),
            const Text('COMPARTIR NOTA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: Colors.black)),
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(color: const Color(0xFFF5F0E8), border: Border.all(color: Colors.black, width: 2)),
              child: Row(
                children: [
                  const Icon(Icons.link_rounded, size: 18, color: Colors.black),
                  const SizedBox(width: 8),
                  Expanded(child: Text(link, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12, color: Colors.black))),
                ],
              ),
            ),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                style: ElevatedButton.styleFrom(backgroundColor: const Color(0xFFFFCC00), foregroundColor: Colors.black, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero), side: const BorderSide(color: Colors.black, width: 2), padding: const EdgeInsets.symmetric(vertical: 12)),
                onPressed: () {
                  Clipboard.setData(ClipboardData(text: link));
                  Navigator.pop(context);
                  ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Enlace copiado al portapapeles', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800)), backgroundColor: Colors.black, shape: RoundedRectangleBorder(borderRadius: BorderRadius.zero)));
                },
                child: const Text('COPIAR AL PORTAPAPELES', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
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
        builder: (context, scroll) => Container(
          decoration: BoxDecoration(color: Colors.white, border: Border.all(color: Colors.black, width: 2), borderRadius: const BorderRadius.vertical(top: Radius.circular(0)), boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(6, 6))]),
          child: Column(
            children: [
              Container(
                margin: const EdgeInsets.only(top: 12),
                width: 40,
                height: 4,
                decoration: BoxDecoration(color: Colors.black, borderRadius: BorderRadius.circular(0)),
              ),
              Expanded(
                child: ListView(
                  controller: scroll,
                  padding: const EdgeInsets.all(20),
                  children: [
                    Row(
                      children: [
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                          decoration: BoxDecoration(color: note.visibility == 'public' ? const Color(0xFF0055FF) : const Color(0xFFFFCC00), border: Border.all(color: Colors.black, width: 1.5)),
                          child: Text(note.visibility.toUpperCase(), style: TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: note.visibility == 'public' ? Colors.white : Colors.black)),
                        ),
                        const SizedBox(width: 8),
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                          decoration: BoxDecoration(color: const Color(0xFFF5F0E8), border: Border.all(color: Colors.black, width: 1.5)),
                          child: Text(_noteTags[note.id] ?? 'General', style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w800, color: Colors.black)),
                        ),
                        const Spacer(),
                        Text(_formatDate(note.updatedAt), style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: Color(0xFF1A1A1A))),
                      ],
                    ),
                    const SizedBox(height: 16),
                    Text(note.title, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w900, color: Colors.black, height: 1.2)),
                    const SizedBox(height: 12),
                    Container(height: 2, color: Colors.black),
                    const SizedBox(height: 16),
                    Text(note.content, style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w500, color: Colors.black, height: 1.5)),
                    const SizedBox(height: 24),
                    Row(
                      children: [
                        _ActionButton(icon: Icons.favorite_rounded, label: '${_likesCount[note.id] ?? 0}', isActive: _isLiked[note.id] ?? false, activeColor: const Color(0xFFE63B2E), onTap: () => _toggleLike(note)),
                        const SizedBox(width: 8),
                        _ActionButton(icon: Icons.bookmark_rounded, label: 'Guardar', isActive: _isSaved[note.id] ?? false, activeColor: const Color(0xFFFFCC00), onTap: () => _toggleSave(note)),
                        const SizedBox(width: 8),
                        _ActionButton(icon: Icons.share_rounded, label: 'Compartir', onTap: () => _shareNote(note)),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
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

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > 1024;
    return Scaffold(
      backgroundColor: const Color(0xFFF5F0E8),
      body: SafeArea(
        child: Padding(
          padding: EdgeInsets.symmetric(horizontal: isDesktop ? 32 : 16, vertical: 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _buildHeader(isDesktop),
              const SizedBox(height: 16),
              _buildSearchBar(),
              if (_isSyncingBackend) ...[
                const SizedBox(height: 8),
                const LinearProgressIndicator(color: Colors.black, backgroundColor: Color(0xFFF5F0E8)),
              ],
              const SizedBox(height: 12),
              _buildFilterChips(),
              const SizedBox(height: 16),
              Expanded(child: _buildBody(isDesktop)),
            ],
          ),
        ),
      ),
      floatingActionButton: Padding(
        padding: const EdgeInsets.only(bottom: 16, right: 8),
        child: InkWell(
          onTap: () => _showCreateDialog(),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
            decoration: BoxDecoration(
              color: const Color(0xFFFFCC00),
              border: Border.all(color: Colors.black, width: 2),
              boxShadow: const [
                BoxShadow(color: Colors.black, offset: Offset(4, 4), blurRadius: 0),
              ],
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.add, color: Colors.black, size: 20),
                SizedBox(width: 8),
                Text(
                  'NUEVA NOTA',
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    fontSize: 13,
                    letterSpacing: 0.5,
                    color: Colors.black,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  void _showCreateDialog() {
    final titleCtrl = TextEditingController();
    final contentCtrl = TextEditingController();
    String visibility = 'private';
    showDialog(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          backgroundColor: Colors.white,
          shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero, side: BorderSide(color: Colors.black, width: 2)),
          title: const Text('NUEVA NOTA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: Colors.black)),
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
                    border: OutlineInputBorder(borderRadius: BorderRadius.zero, borderSide: BorderSide(color: Colors.black, width: 2)),
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
                    border: OutlineInputBorder(borderRadius: BorderRadius.zero, borderSide: BorderSide(color: Colors.black, width: 2)),
                  ),
                ),
                const SizedBox(height: 12),
                Row(
                  children: [
                    Expanded(
                      child: _VisibilityOption(
                        label: 'PÚBLICO',
                        active: visibility == 'public',
                        onTap: () => setDialogState(() => visibility = 'public'),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: _VisibilityOption(
                        label: 'PRIVADO',
                        active: visibility == 'private',
                        onTap: () => setDialogState(() => visibility = 'private'),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context), child: const Text('CANCELAR', style: TextStyle(color: Colors.black, fontWeight: FontWeight.w800))),
            ElevatedButton(
              style: ElevatedButton.styleFrom(backgroundColor: Colors.black, foregroundColor: Colors.white, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero)),
              onPressed: () async {
                if (titleCtrl.text.trim().isEmpty) return;
                final note = LocalNote(
                  id: DateTime.now().millisecondsSinceEpoch.toString(),
                  title: titleCtrl.text.trim(),
                  content: contentCtrl.text.trim(),
                  visibility: visibility,
                  updatedAt: DateTime.now(),
                );
                _noteTags[note.id] = visibility == 'public' ? 'General' : 'Privado';
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
                if (mounted) Navigator.pop(context);
              },
              child: const Text('GUARDAR', style: TextStyle(fontWeight: FontWeight.w900)),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(bool isDesktop) {
    return Row(
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('TODAS LAS NOTAS', style: TextStyle(fontSize: isDesktop ? 26 : 22, fontWeight: FontWeight.w900, color: Colors.black, letterSpacing: -0.5)),
              const SizedBox(height: 4),
              Row(
                children: [
                  const Text('Búsqueda offline instantánea con FTS5', style: TextStyle(fontSize: 12.5, fontWeight: FontWeight.w700, color: Color(0xFF555555))),
                  const SizedBox(width: 8),
                  if (_useMemoryFallback)
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                      decoration: BoxDecoration(color: const Color(0xFFFFCC00), border: Border.all(color: Colors.black, width: 1.5)),
                      child: const Text('MODO OFFLINE', style: TextStyle(fontSize: 9, fontWeight: FontWeight.w900, color: Colors.black)),
                    ),
                ],
              ),
            ],
          ),
        ),
        IconButton(
          tooltip: _isGridView ? 'Vista lista' : 'Vista grilla',
          onPressed: () => setState(() => _isGridView = !_isGridView),
          icon: Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: Colors.white,
              border: Border.all(color: Colors.black, width: 2),
              borderRadius: BorderRadius.zero,
            ),
            child: Icon(_isGridView ? Icons.view_list_rounded : Icons.grid_view_rounded, color: Colors.black, size: 20),
          ),
        ),
      ],
    );
  }

  Widget _buildSearchBar() {
    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: Colors.black, width: 2),
        boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: TextField(
        controller: _searchController,
        onChanged: _onSearchChanged,
        decoration: InputDecoration(
          hintText: 'Buscar por título o contenido... (ej: "calculo*", "joins")',
          hintStyle: const TextStyle(color: Color(0xFF555555), fontWeight: FontWeight.w600, fontSize: 13),
          prefixIcon: const Icon(Icons.search_rounded, color: Colors.black),
          suffixIcon: ValueListenableBuilder<TextEditingValue>(
            valueListenable: _searchController,
            builder: (context, value, child) {
              if (value.text.isEmpty) return const SizedBox.shrink();
              return IconButton(
                icon: const Icon(Icons.clear_rounded, color: Color(0xFF555555)),
                onPressed: () {
                  _searchController.clear();
                  _onSearchChanged('');
                },
              );
            },
          ),
          border: InputBorder.none,
          contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        ),
        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14, color: Colors.black),
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
                padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                decoration: BoxDecoration(
                  color: selected ? const Color(0xFFFFCC00) : Colors.white,
                  border: Border.all(color: Colors.black, width: 2),
                  borderRadius: BorderRadius.zero,
                  boxShadow: selected ? const [BoxShadow(color: Colors.black, offset: Offset(2, 2), blurRadius: 0)] : null,
                ),
                child: Text(c['label']!, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: selected ? Colors.black : Colors.black, letterSpacing: 0.5)),
              ),
            ),
          );
        }).toList(),
      ),
    );
  }

  Widget _buildBody(bool isDesktop) {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator(color: Colors.black));
    }
    if (_error != null) {
      return Center(
        child: Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: Colors.white,
            border: Border.all(color: Colors.black, width: 2),
            boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(4, 4), blurRadius: 0)],
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded, size: 36, color: Color(0xFFE63B2E)),
              const SizedBox(height: 12),
              Text(_error!, textAlign: TextAlign.center, style: const TextStyle(fontWeight: FontWeight.w700, color: Colors.black, fontSize: 13)),
              const SizedBox(height: 14),
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  style: ElevatedButton.styleFrom(backgroundColor: Colors.black, foregroundColor: Colors.white, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero)),
                  onPressed: _retry,
                  child: const Text('REINTENTAR', style: TextStyle(fontWeight: FontWeight.w900)),
                ),
              ),
            ],
          ),
        ),
      );
    }
    if (_filtered.isEmpty) {
      final isSearching = _searchController.text.trim().isNotEmpty || _selectedFilter != 'all';
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
                  boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(3, 3), blurRadius: 0)],
                ),
                child: const Icon(Icons.search_off_rounded, size: 36, color: Color(0xFF555555)),
              ),
              const SizedBox(height: 14),
              const Text('SIN RESULTADOS', style: TextStyle(fontWeight: FontWeight.w900, color: Colors.black)),
              const SizedBox(height: 6),
              const Text('Prueba con otra palabra clave o prefijo (ej: prog*)', style: TextStyle(fontWeight: FontWeight.w600, color: Color(0xFF555555), fontSize: 12)),
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
                boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(3, 3), blurRadius: 0)],
              ),
              child: const Icon(Icons.note_add_rounded, size: 36, color: Colors.black),
            ),
            const SizedBox(height: 14),
            const Text('NO TIENES NOTAS CREADAS TODAVÍA', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, color: Colors.black, fontSize: 14)),
            const SizedBox(height: 6),
            const Text('Crea tu primera nota y aparecerá aquí', style: TextStyle(fontWeight: FontWeight.w600, color: Color(0xFF555555), fontSize: 12)),
            const SizedBox(height: 16),
            ElevatedButton.icon(
              style: ElevatedButton.styleFrom(backgroundColor: Colors.black, foregroundColor: Colors.white, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero), side: const BorderSide(color: Colors.black, width: 2), padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12)),
              onPressed: _showCreateDialog,
              icon: const Icon(Icons.add_rounded, size: 18),
              label: const Text('CREAR PRIMERA NOTA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
            ),
          ],
        ),
      );
    }

    if (_isGridView) {
      final width = MediaQuery.of(context).size.width;
      final isSmall = width < 600;
      return GridView.builder(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 90),
        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: 2,
          crossAxisSpacing: 14,
          mainAxisSpacing: 14,
          childAspectRatio: isSmall ? 0.68 : 1.45,
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

    // Lista: en desktop 1 columna, en mobile también 1 - con padding inferior para FAB
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 90),
      itemCount: _filtered.length,
      separatorBuilder: (_, __) => const SizedBox(height: 12),
      itemBuilder: (context, i) {
        final note = _filtered[i];
        return _NoteCard(
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
        );
      },
    );
  }
}

class _VisibilityOption extends StatelessWidget {
  final String label;
  final bool active;
  final VoidCallback onTap;
  const _VisibilityOption({required this.label, required this.active, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 10),
        decoration: BoxDecoration(
          color: active ? const Color(0xFF0055FF) : const Color(0xFFF5F0E8),
          border: Border.all(color: Colors.black, width: 2),
          boxShadow: active ? const [BoxShadow(color: Colors.black, offset: Offset(2, 2), blurRadius: 0)] : null,
        ),
        child: Center(child: Text(label, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: active ? Colors.white : Colors.black))),
      ),
    );
  }
}

class _NoteCard extends StatelessWidget {
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
  const _NoteCard({required this.note, required this.isGrid, required this.likes, required this.isLiked, required this.isSaved, required this.tag, required this.onTap, required this.onLike, required this.onSave, required this.onShare});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: Colors.white,
          border: Border.all(color: Colors.black, width: 2),
          boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(3, 3), blurRadius: 0)],
        ),
        child: isGrid
            ? Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildBadgesRow(),
                  const SizedBox(height: 6),
                  Text(note.title, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: Colors.black, height: 1.2)),
                  const SizedBox(height: 4),
                  Expanded(
                    child: Text(note.content, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 11, color: Color(0xFF4A4A4A), height: 1.3)),
                  ),
                  const SizedBox(height: 8),
                  _buildBottomActions(isGrid: true),
                ],
              )
            : Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  _buildBadgesRow(),
                  const SizedBox(height: 6),
                  Text(note.title, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: Colors.black, height: 1.2)),
                  const SizedBox(height: 4),
                  Text(note.content, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w500, color: Color(0xFF4A4A4A), height: 1.3)),
                  const SizedBox(height: 12),
                  _buildBottomActions(isGrid: false),
                ],
              ),
      ),
    );
  }

  Widget _buildBadgesRow() {
    return Row(
      children: [
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
          decoration: BoxDecoration(
            color: note.visibility == 'public' ? const Color(0xFF0055FF) : const Color(0xFFFFCC00),
            border: Border.all(color: Colors.black, width: 1.5),
          ),
          child: Text(
            note.visibility == 'public' ? 'PUB' : 'PRIV',
            style: TextStyle(fontSize: 9, fontWeight: FontWeight.w900, color: note.visibility == 'public' ? Colors.white : Colors.black),
          ),
        ),
        const SizedBox(width: 4),
        Expanded(
          child: Text(
            _formatDate(note.updatedAt),
            textAlign: TextAlign.end,
            style: const TextStyle(fontSize: 9, fontWeight: FontWeight.w700, color: Color(0xFF555555)),
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
      children: [
        InkWell(
          onTap: onLike,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
            decoration: BoxDecoration(
              color: isLiked ? const Color(0xFFE63B2E) : Colors.white,
              border: Border.all(color: Colors.black, width: 1.5),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.favorite_rounded, size: 12, color: isLiked ? Colors.white : Colors.black),
                const SizedBox(width: 3),
                Text('$likes', style: TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: isLiked ? Colors.white : Colors.black)),
              ],
            ),
          ),
        ),
        InkWell(
          onTap: onSave,
          child: Container(
            padding: const EdgeInsets.all(4),
            decoration: BoxDecoration(
              color: isSaved ? const Color(0xFFFFCC00) : Colors.white,
              border: Border.all(color: Colors.black, width: 1.5),
            ),
            child: Icon(isSaved ? Icons.bookmark_rounded : Icons.bookmark_border_rounded, size: 13, color: Colors.black),
          ),
        ),
        InkWell(
          onTap: onShare,
          child: Container(
            padding: EdgeInsets.symmetric(horizontal: isGrid ? 6 : 10, vertical: isGrid ? 4 : 6),
            decoration: BoxDecoration(
              color: Colors.white,
              border: Border.all(color: Colors.black, width: 1.5),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.share_rounded, size: 13, color: Colors.black),
                if (!isGrid) ...[
                  const SizedBox(width: 4),
                  const Text('Compartir', style: TextStyle(fontSize: 11, fontWeight: FontWeight.w800, color: Colors.black)),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }

  String _formatDate(DateTime d) => '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';
}

class _ActionButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool isActive;
  final Color activeColor;
  final VoidCallback onTap;
  const _ActionButton({required this.icon, required this.label, this.isActive = false, this.activeColor = Colors.white, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(color: isActive ? activeColor : Colors.white, border: Border.all(color: Colors.black, width: 1.5)),
        child: Row(children: [Icon(icon, size: 14, color: isActive ? Colors.white : Colors.black), const SizedBox(width: 4), Text(label, style: TextStyle(fontSize: 11, fontWeight: FontWeight.w800, color: isActive ? Colors.white : Colors.black))]),
      ),
    );
  }
}
