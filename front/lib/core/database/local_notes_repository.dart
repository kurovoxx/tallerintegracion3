import 'package:drift/drift.dart';

import 'app_database.dart';

class LocalNotesRepository {
  LocalNotesRepository(this.db);

  final AppDatabase db;

  /// Inserta o reemplaza una nota local (upsert por id).
  /// Opera exclusivamente sobre `local_notes`; los triggers FTS5 se encargan
  /// de sincronizar `local_notes_fts` automáticamente.
  Future<void> upsertNote(LocalNote note) async {
    await db.into(db.localNotes).insertOnConflictUpdate(
          LocalNotesCompanion(
            id: Value(note.id),
            title: Value(note.title),
            content: Value(note.content),
            visibility: Value(note.visibility),
            updatedAt: Value(note.updatedAt),
          ),
        );
  }

  /// Búsqueda offline FTS5 instantánea con ranking por relevancia.
  /// Soporta palabras clave y prefijos (`query*`).
  /// Si query está vacío, retorna todas.
  /// Si `db.ftsAvailable` es falso (FTS5 no disponible), hace fallback directo
  /// a LIKE %query% sin intentar consultar FTS.
  Future<List<LocalNote>> searchNotesFts(String query) async {
    final q = query.trim();
    if (q.isEmpty) {
      return getAllNotes();
    }

    // Normalizar para FTS5: escapar comillas y agregar prefijo * para búsqueda por prefijo
    final tokens = q
        .split(RegExp(r'\s+'))
        .where((t) => t.isNotEmpty)
        .map((t) => '"${t.replaceAll('"', '""')}"*')
        .join(' ');

    if (tokens.isEmpty) return getAllNotes();

    // Determinismo: si FTS5 no está disponible, fallback estructurado LIKE
    if (!db.ftsAvailable) {
      final likeQuery = '%$q%';
      final fallback = await (db.select(db.localNotes)
            ..where((t) => t.title.like(likeQuery) | t.content.like(likeQuery))
            ..orderBy([(t) => OrderingTerm.desc(t.updatedAt)]))
          .get();
      return fallback;
    }

    try {
      final rows = await db.customSelect(
        '''
        SELECT n.id, n.title, n.content, n.visibility, n.updated_at,
               bm25(local_notes_fts, 2.0, 1.0) AS score
        FROM local_notes n
        JOIN local_notes_fts fts ON n.rowid = fts.rowid
        WHERE local_notes_fts MATCH ?
        ORDER BY score ASC
        ''',
        variables: [Variable.withString(tokens)],
        readsFrom: {db.localNotes, db.localNotesFts},
      ).get();

      return rows.map((row) {
        return LocalNote(
          id: row.read<String>('id'),
          title: row.read<String>('title'),
          content: row.read<String>('content'),
          visibility: row.read<String>('visibility'),
          updatedAt: row.read<DateTime>('updated_at'),
        );
      }).toList();
    } catch (e) {
      // Fallback en caliente si FTS5 falla en runtime (Windows sin sqlite3 adecuado)
      // Búsqueda básica LIKE %query% en título y contenido para no congelar UI
      final likeQuery = '%$q%';
      final fallback = await (db.select(db.localNotes)
            ..where((t) => t.title.like(likeQuery) | t.content.like(likeQuery))
            ..orderBy([(t) => OrderingTerm.desc(t.updatedAt)]))
          .get();
      return fallback;
    }
  }

  /// Lista todas las notas locales ordenadas por updatedAt descendente.
  Future<List<LocalNote>> getAllNotes() async {
    final query = db.select(db.localNotes)
      ..orderBy([(t) => OrderingTerm.desc(t.updatedAt)]);
    return query.get();
  }

  /// Elimina una nota por id (útil para sincronización).
  /// Opera sobre `local_notes`; el trigger AFTER DELETE limpia el índice FTS.
  Future<void> deleteNote(String id) async {
    await (db.delete(db.localNotes)..where((t) => t.id.equals(id))).go();
  }

  /// Limpia todo (para tests o logout).
  /// Opera sobre `local_notes`; los triggers mantienen el índice consistente.
  Future<void> clearAll() async {
    await db.delete(db.localNotes).go();
  }
}
