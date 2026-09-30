import 'package:drift/drift.dart';

import 'app_database.dart';

class LocalNotesRepository {
  LocalNotesRepository(this.db);

  final AppDatabase db;

  /// Inserta o reemplaza una nota local (upsert por id), incluyendo su dueño.
  /// Opera exclusivamente sobre `local_notes`; los triggers FTS5 se encargan
  /// de sincronizar `local_notes_fts` automáticamente.
  Future<void> upsertNote(LocalNote note) async {
    await db
        .into(db.localNotes)
        .insertOnConflictUpdate(
          LocalNotesCompanion(
            id: Value(note.id),
            title: Value(note.title),
            content: Value(note.content),
            visibility: Value(note.visibility),
            updatedAt: Value(note.updatedAt),
            ownerUserId: Value(note.ownerUserId),
          ),
        );
  }

  /// Búsqueda offline FTS5 instantánea con ranking por relevancia,
  /// filtrada por dueño ([ownerId]).
  /// - Con sesión ([ownerId] no nulo): solo filas de ese dueño. Las filas
  ///   legacy sin dueño NUNCA se muestran como propias.
  /// - Sin sesión ([ownerId] nulo): solo filas sin dueño (locales del equipo).
  /// Si query está vacío, retorna todas (del dueño).
  /// Si `db.ftsAvailable` es falso (FTS5 no disponible), hace fallback directo
  /// a LIKE %query% sin intentar consultar FTS.
  Future<List<LocalNote>> searchNotesFts(
    String query, {
    String? ownerId,
  }) async {
    final q = query.trim();
    if (q.isEmpty) {
      return getAllNotes(ownerId: ownerId);
    }

    // Normalizar para FTS5: escapar comillas y agregar prefijo * para búsqueda por prefijo
    final tokens = q
        .split(RegExp(r'\s+'))
        .where((t) => t.isNotEmpty)
        .map((t) => '"${t.replaceAll('"', '""')}"*')
        .join(' ');

    if (tokens.isEmpty) return getAllNotes(ownerId: ownerId);

    // Determinismo: si FTS5 no está disponible, fallback estructurado LIKE
    if (!db.ftsAvailable) {
      final likeQuery = '%$q%';
      final fallback =
          await (db.select(db.localNotes)
                ..where(
                  (t) =>
                      _ownerFilter(t, ownerId) &
                      (t.title.like(likeQuery) | t.content.like(likeQuery)),
                )
                ..orderBy([(t) => OrderingTerm.desc(t.updatedAt)]))
              .get();
      return fallback;
    }

    try {
      final Object ownerClause;
      final List<Variable> vars;
      if (ownerId == null) {
        ownerClause = 'n.owner_user_id IS NULL';
        vars = [Variable.withString(tokens)];
      } else {
        ownerClause = 'n.owner_user_id = ?';
        vars = [Variable.withString(tokens), Variable.withString(ownerId)];
      }
      final rows = await db
          .customSelect(
            '''
        SELECT n.id, n.title, n.content, n.visibility, n.updated_at, n.owner_user_id,
               bm25(local_notes_fts, 2.0, 1.0) AS score
        FROM local_notes n
        JOIN local_notes_fts fts ON n.rowid = fts.rowid
        WHERE local_notes_fts MATCH ? AND $ownerClause
        ORDER BY score ASC
        ''',
            variables: vars,
            readsFrom: {db.localNotes, db.localNotesFts},
          )
          .get();

      return rows.map((row) {
        return LocalNote(
          id: row.read<String>('id'),
          title: row.read<String>('title'),
          content: row.read<String>('content'),
          visibility: row.read<String>('visibility'),
          updatedAt: row.read<DateTime>('updated_at'),
          ownerUserId: row.readNullable<String>('owner_user_id'),
        );
      }).toList();
    } catch (e) {
      // Fallback en caliente si FTS5 falla en runtime (Windows sin sqlite3 adecuado)
      // Búsqueda básica LIKE %query% en título y contenido para no congelar UI
      final likeQuery = '%$q%';
      final fallback =
          await (db.select(db.localNotes)
                ..where(
                  (t) =>
                      _ownerFilter(t, ownerId) &
                      (t.title.like(likeQuery) | t.content.like(likeQuery)),
                )
                ..orderBy([(t) => OrderingTerm.desc(t.updatedAt)]))
              .get();
      return fallback;
    }
  }

  Expression<bool> _ownerFilter($LocalNotesTable t, String? ownerId) {
    if (ownerId == null) return t.ownerUserId.isNull();
    return t.ownerUserId.equals(ownerId);
  }

  /// Lista las notas del dueño ordenadas por updatedAt descendente.
  Future<List<LocalNote>> getAllNotes({String? ownerId}) async {
    final query = db.select(db.localNotes)
      ..where((t) => _ownerFilter(t, ownerId))
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

  /// Limpia solo las filas de un dueño (logout selectivo).
  Future<void> clearOwner(String ownerId) async {
    await (db.delete(
      db.localNotes,
    )..where((t) => t.ownerUserId.equals(ownerId))).go();
  }
}
