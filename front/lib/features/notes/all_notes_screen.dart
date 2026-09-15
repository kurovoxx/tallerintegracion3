import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import '../../core/database/app_database.dart';
import '../../core/database/local_notes_repository.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';

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

  // Estado guardado para FutureBuilder pattern correcto (no crear Future en build)
  late Future<List<LocalNote>> _notesFuture;
  List<LocalNote> _filtered = [];
  List<LocalNote> _memoryFallback = [];
  bool _isLoading = true;
  bool _isGrid = false;
  String? _error;
  bool _useMemoryFallback = false;
  bool _isSyncingBackend = false;
  String _currentQuery = '';

  List<LocalNote> _demoNotes() {
    final now = DateTime.now();
    return [
      LocalNote(id: '1', title: 'Cálculo - Límites y derivadas', content: 'Apuntes de cálculo diferencial: límites, continuidad y reglas de derivación.', visibility: 'private', updatedAt: now),
      LocalNote(id: '2', title: 'Estructuras de Datos - Árboles', content: 'Árboles binarios, AVL y recorridos preorden, inorden, postorden.', visibility: 'private', updatedAt: now.subtract(const Duration(hours: 2))),
      LocalNote(id: '3', title: 'Bases de Datos - SQL Joins', content: 'Joins internos, externos, subconsultas y optimización con índices.', visibility: 'public', updatedAt: now.subtract(const Duration(days: 1))),
      LocalNote(id: '4', title: 'Redes - Modelo OSI', content: 'Capas OSI y TCP/IP, encapsulamiento y protocolos.', visibility: 'public', updatedAt: now.subtract(const Duration(days: 2))),
    ];
  }

  @override
  void initState() {
    super.initState();
    // Guardar Future en estado para evitar rebuild infinito (no crear Future en build)
    _notesFuture = _initAndLoad();
    // No usar addListener que dispare setState durante hit-test; usar onChanged con debounce
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
        _filtered = List.from(_memoryFallback);
        if (mounted) setState(() => _isLoading = false);
        return _filtered;
      }

      final existing = await _repo!.getAllNotes();
      if (existing.isEmpty) {
        final demo = _demoNotes();
        for (final n in demo) {
          await _repo!.upsertNote(n);
        }
        await _syncFromBackend();
      } else {
        // Sync en background sin bloquear UI
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
            _filtered = List.from(_memoryFallback);
            _error = null;
          }
        });
      }
      return _filtered;
    }
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
          .get(Uri.parse('http://localhost:8082/notes/me?limit=50'), headers: headers)
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
      List<LocalNote> res;
      if (_useMemoryFallback) {
        if (query.trim().isEmpty) {
          res = List.from(_memoryFallback);
        } else {
          final lower = query.toLowerCase();
          res = _memoryFallback.where((n) => n.title.toLowerCase().contains(lower) || n.content.toLowerCase().contains(lower)).toList();
        }
      } else {
        res = await _repo!.searchNotesFts(query);
      }
      if (mounted) setState(() => _filtered = res);
    } catch (e) {
      debugPrint('Search error: $e');
      if (mounted) setState(() => _filtered = []);
    }
  }

  Future<void> _retry() async {
    _searchController.clear();
    _currentQuery = '';
    _notesFuture = _initAndLoad();
    await _notesFuture;
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
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
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
                const LinearProgressIndicator(color: AppColors.border, backgroundColor: AppColors.bg),
              ],
              const SizedBox(height: 12),
              _buildFilterChips(),
              const SizedBox(height: 16),
              Expanded(child: _buildBody(isDesktop)),
            ],
          ),
        ),
      ),
      floatingActionButton: FloatingActionButton.extended(
        backgroundColor: AppColors.border,
        foregroundColor: Colors.white,
        onPressed: () => _showCreateDialog(),
        icon: const Icon(Icons.add_rounded, color: Colors.white),
        label: const Text('NUEVA NOTA', style: TextStyle(fontWeight: FontWeight.w900, letterSpacing: 0.5)),
      ),
    );
  }

  void _showCreateDialog() {
    final titleCtrl = TextEditingController();
    final contentCtrl = TextEditingController();
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        backgroundColor: AppColors.surface,
        shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero, side: BorderSide(color: Colors.black, width: 2)),
        title: const Text('NUEVA NOTA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(controller: titleCtrl, decoration: const InputDecoration(hintText: 'Título', filled: true, fillColor: AppColors.bg, border: OutlineInputBorder(borderRadius: BorderRadius.zero, borderSide: BorderSide(color: Colors.black, width: 2)))),
            const SizedBox(height: 12),
            TextField(controller: contentCtrl, maxLines: 4, decoration: const InputDecoration(hintText: 'Contenido Markdown', filled: true, fillColor: AppColors.bg, border: OutlineInputBorder(borderRadius: BorderRadius.zero, borderSide: BorderSide(color: Colors.black, width: 2)))),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('CANCELAR', style: TextStyle(color: AppColors.text, fontWeight: FontWeight.w800))),
          ElevatedButton(
            style: ElevatedButton.styleFrom(backgroundColor: Colors.black, foregroundColor: Colors.white, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero)),
            onPressed: () async {
              if (titleCtrl.text.trim().isEmpty) return;
              final note = LocalNote(id: DateTime.now().millisecondsSinceEpoch.toString(), title: titleCtrl.text.trim(), content: contentCtrl.text.trim(), visibility: 'private', updatedAt: DateTime.now());
              if (_useMemoryFallback) {
                setState(() {
                  _memoryFallback.insert(0, note);
                });
                _applySearch(_currentQuery);
              } else {
                try {
                  await _repo!.upsertNote(note);
                  await _applySearch(_currentQuery);
                } catch (e) {
                  setState(() {
                    _useMemoryFallback = true;
                    _memoryFallback = [note, ..._memoryFallback];
                  });
                  _applySearch(_currentQuery);
                }
              }
              if (mounted) Navigator.pop(context);
            },
            child: const Text('CREAR', style: TextStyle(fontWeight: FontWeight.w900)),
          ),
        ],
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
              Text('TODAS LAS NOTAS', style: TextStyle(fontSize: isDesktop ? 26 : 22, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: -0.5)),
              const SizedBox(height: 4),
              Row(
                children: [
                  const Text('Búsqueda offline instantánea con FTS5', style: TextStyle(fontSize: 12.5, fontWeight: FontWeight.w700, color: AppColors.muted)),
                  const SizedBox(width: 8),
                  if (_useMemoryFallback)
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                      decoration: BoxDecoration(color: const Color(0xFFFFD700), border: Border.all(color: Colors.black, width: 1.5)),
                      child: const Text('MODO OFFLINE', style: TextStyle(fontSize: 9, fontWeight: FontWeight.w900, color: AppColors.text)),
                    ),
                ],
              ),
            ],
          ),
        ),
        // Sin MouseRegion: IconButton ya maneja hover internamente, no envolvemos en MouseRegion
        IconButton(
          tooltip: _isGrid ? 'Vista lista' : 'Vista grilla',
          onPressed: () => setState(() => _isGrid = !_isGrid),
          icon: Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: AppColors.surface,
              border: Border.all(color: AppColors.border, width: 2),
              borderRadius: BorderRadius.circular(6),
            ),
            child: Icon(_isGrid ? Icons.view_list_rounded : Icons.grid_view_rounded, color: AppColors.text, size: 20),
          ),
        ),
      ],
    );
  }

  Widget _buildSearchBar() {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: TextField(
        controller: _searchController,
        onChanged: _onSearchChanged,
        decoration: InputDecoration(
          hintText: 'Buscar por título o contenido... (ej: "calculo*", "joins")',
          hintStyle: const TextStyle(color: AppColors.muted, fontWeight: FontWeight.w600, fontSize: 13),
          prefixIcon: const Icon(Icons.search_rounded, color: AppColors.text),
          // Evitar rebuild con controller.text: usar ValueListenableBuilder solo para suffix
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
          contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        ),
        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14, color: AppColors.text),
      ),
    );
  }

  Widget _buildFilterChips() {
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        children: [
          _FilterChip(label: 'Todas', selected: true, onTap: () {}),
          const SizedBox(width: 8),
          _FilterChip(label: 'Públicas', selected: false, onTap: () {}),
          const SizedBox(width: 8),
          _FilterChip(label: 'Privadas', selected: false, onTap: () {}),
          const SizedBox(width: 8),
          _FilterChip(label: 'Recientes', selected: false, onTap: () {}),
        ],
      ),
    );
  }

  Widget _buildBody(bool isDesktop) {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator(color: AppColors.text));
    }
    if (_error != null) {
      return Center(
        child: Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(color: AppColors.border, width: 2),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded, size: 36, color: AppColors.error),
              const SizedBox(height: 12),
              Text(_error!, textAlign: TextAlign.center, style: const TextStyle(fontWeight: FontWeight.w700, color: AppColors.text, fontSize: 13)),
              const SizedBox(height: 14),
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  style: ElevatedButton.styleFrom(backgroundColor: AppColors.border, foregroundColor: Colors.white, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero)),
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
      final isSearching = _searchController.text.trim().isNotEmpty;
      if (isSearching) {
        return Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Container(
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(
                  color: AppColors.surface,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: const Icon(Icons.search_off_rounded, size: 36, color: AppColors.muted),
              ),
              const SizedBox(height: 14),
              const Text('SIN RESULTADOS', style: TextStyle(fontWeight: FontWeight.w900, color: AppColors.text)),
              const SizedBox(height: 6),
              const Text('Prueba con otra palabra clave o prefijo (ej: prog*)', style: TextStyle(fontWeight: FontWeight.w600, color: AppColors.muted, fontSize: 12)),
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
                color: AppColors.surface,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(12),
                boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
              ),
              child: const Icon(Icons.note_add_rounded, size: 36, color: AppColors.text),
            ),
            const SizedBox(height: 14),
            const Text('NO TIENES NOTAS CREADAS TODAVÍA', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, color: AppColors.text, fontSize: 14)),
            const SizedBox(height: 6),
            const Text('Crea tu primera nota y aparecerá aquí', style: TextStyle(fontWeight: FontWeight.w600, color: AppColors.muted, fontSize: 12)),
            const SizedBox(height: 16),
            ElevatedButton.icon(
              style: ElevatedButton.styleFrom(backgroundColor: AppColors.border, foregroundColor: Colors.white, shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero), side: const BorderSide(color: Colors.black, width: 2), padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12)),
              onPressed: _showCreateDialog,
              icon: const Icon(Icons.add_rounded, size: 18),
              label: const Text('CREAR PRIMERA NOTA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12)),
            ),
          ],
        ),
      );
    }

    if (_isGrid) {
      return GridView.builder(
        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: isDesktop ? 3 : 2,
          crossAxisSpacing: 14,
          mainAxisSpacing: 14,
          childAspectRatio: isDesktop ? 1.1 : 0.95,
        ),
        itemCount: _filtered.length,
        itemBuilder: (context, i) => _NoteCard(note: _filtered[i], isGrid: true),
      );
    }

    return ListView.separated(
      itemCount: _filtered.length,
      separatorBuilder: (_, __) => const SizedBox(height: 12),
      itemBuilder: (context, i) => _NoteCard(note: _filtered[i], isGrid: false),
    );
  }
}

class _FilterChip extends StatelessWidget {
  final String label;
  final bool selected;
  final VoidCallback onTap;
  const _FilterChip({required this.label, required this.selected, required this.onTap});

  @override
  Widget build(BuildContext context) {
    // GestureDetector sin MouseRegion explícito para evitar _debugDuringDeviceUpdate
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
        decoration: BoxDecoration(
          color: selected ? AppColors.accentYellow : AppColors.surface,
          border: Border.all(color: AppColors.border, width: 2),
          borderRadius: BorderRadius.circular(20),
          boxShadow: selected ? const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)] : null,
        ),
        child: Text(label, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text, letterSpacing: 0.5)),
      ),
    );
  }
}

class _NoteCard extends StatelessWidget {
  final LocalNote note;
  final bool isGrid;
  const _NoteCard({required this.note, required this.isGrid});

  @override
  Widget build(BuildContext context) {
    // Sin InkWell/MouseRegion redundante; Container puro evita hit-test reconstrucciones
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: note.visibility == 'public' ? AppColors.accentBlue : AppColors.accentYellow,
                  borderRadius: BorderRadius.circular(4),
                  border: Border.all(color: AppColors.border, width: 1.5),
                ),
                child: Text(note.visibility.toUpperCase(),
                    style: TextStyle(
                        fontSize: 9, fontWeight: FontWeight.w900, color: note.visibility == 'public' ? Colors.white : AppColors.text)),
              ),
              const Spacer(),
              Text(_formatDate(note.updatedAt), style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: AppColors.muted)),
            ],
          ),
          const SizedBox(height: 10),
          Text(note.title,
              maxLines: isGrid ? 2 : 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text, height: 1.2)),
          const SizedBox(height: 6),
          Text(note.content,
              maxLines: isGrid ? 4 : 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 12, color: AppColors.muted, height: 1.3)),
          const SizedBox(height: 10),
          Row(
            children: [
              const Icon(Icons.description_rounded, size: 14, color: AppColors.muted),
              const SizedBox(width: 4),
              const Text('Markdown', style: TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: AppColors.muted)),
              const Spacer(),
              const Icon(Icons.visibility_rounded, size: 14, color: AppColors.muted),
              const SizedBox(width: 4),
              Text(note.visibility, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: AppColors.muted)),
            ],
          ),
        ],
      ),
    );
  }

  String _formatDate(DateTime d) {
    return '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';
  }
}
