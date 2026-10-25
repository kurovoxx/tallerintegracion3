import 'dart:convert';
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
            version: Value(note.version),
            title: Value(note.title),
            content: Value(note.content),
            visibility: Value(note.visibility),
            updatedAt: Value(note.updatedAt),
            ownerUserId: Value(note.ownerUserId),
            needsSync: Value(note.needsSync),
            // null = "no tocar": una edición normal no debe borrar una clave de
            // creación pendiente ya persistida. La limpieza explícita se hace
            // con clearPendingCreate().
            createKey: note.createKey == null
                ? const Value.absent()
                : Value(note.createKey),
            createSnapshot: note.createSnapshot == null
                ? const Value.absent()
                : Value(note.createSnapshot),
          ),
        );
    if (!note.needsSync && !note.content.contains('attachment-pending://')) {
      await (db.delete(
        db.pendingNoteAttachments,
      )..where((t) => t.noteId.equals(note.id))).go();
    }
  }

  /// Persiste la clave de idempotencia y el snapshot del payload original de la
  /// intención de creación (Fase 1 §P1). Se reutilizan en cada reintento para
  /// que el backend haga replay en lugar de crear una segunda nota/archivo.
  Future<void> setPendingCreate(
    String id, {
    required String key,
    required String snapshot,
  }) async {
    await (db.update(db.localNotes)..where((t) => t.id.equals(id))).write(
      LocalNotesCompanion(
        createKey: Value(key),
        createSnapshot: Value(snapshot),
      ),
    );
  }

  /// Nota por id (o null si no existe).
  Future<LocalNote?> getNoteById(String id) {
    return (db.select(
      db.localNotes,
    )..where((t) => t.id.equals(id))).getSingleOrNull();
  }

  /// Promueve de forma ATÓMICA la fila temporal [tempId] (id local epoch-ms)
  /// al UUID remoto [realId] una vez que el backend confirmó la creación.
  ///
  /// En una sola transacción: copia el contenido local vigente (el usuario
  /// pudo editar mientras el POST seguía en vuelo) sobre la fila [realId]
  /// —que un GET /notes/me concurrente pudo haber insertado ya—, limpia la
  /// intención de creación y borra la fila temporal. Así nunca coexisten la
  /// fila temporal y la remota (causa del duplicado visible) ni se pierde la
  /// nota si el proceso muere entre ambos pasos.
  ///
  /// Devuelve la fila resultante, o null si [tempId] ya no existe (otra
  /// sincronización la promovió antes o el usuario la borró).
  Future<LocalNote?> promoteTempNote(
    String tempId,
    String realId, {
    int? version,
  }) {
    return db.transaction(() async {
      final temp = await getNoteById(tempId);
      if (temp == null) return null;
      final existing = await getNoteById(realId);
      var edited = temp.needsSync;
      try {
        final original = jsonDecode(temp.createSnapshot ?? '{}') as Map;
        edited =
            edited ||
            original['title'] != temp.title ||
            original['content'] != temp.content ||
            original['visibility'] != temp.visibility;
      } catch (_) {
        edited = true;
      }
      final promoted = LocalNote(
        id: realId,
        version: version ?? existing?.version ?? temp.version,
        title: temp.title,
        content: temp.content,
        visibility: temp.visibility,
        updatedAt: temp.updatedAt,
        ownerUserId: temp.ownerUserId ?? existing?.ownerUserId,
        needsSync: edited,
      );
      await (db.update(db.pendingNoteAttachments)
            ..where((t) => t.noteId.equals(tempId)))
          .write(PendingNoteAttachmentsCompanion(noteId: Value(realId)));
      await (db.delete(db.localNotes)..where((t) => t.id.equals(tempId))).go();
      await db
          .into(db.localNotes)
          .insertOnConflictUpdate(
            LocalNotesCompanion(
              id: Value(promoted.id),
              version: Value(promoted.version),
              title: Value(promoted.title),
              content: Value(promoted.content),
              visibility: Value(promoted.visibility),
              updatedAt: Value(promoted.updatedAt),
              ownerUserId: Value(promoted.ownerUserId),
              needsSync: Value(promoted.needsSync),
              createKey: const Value(null),
              createSnapshot: const Value(null),
            ),
          );
      return promoted;
    });
  }

  /// true si existe un tombstone para [id].
  Future<bool> hasTombstone(String id) async {
    final row = await (db.select(
      db.noteTombstones,
    )..where((t) => t.id.equals(id))).getSingleOrNull();
    return row != null;
  }

  /// Limpia la clave/snapshot de creación una vez que la nota ya existe remoto.
  Future<void> clearPendingCreate(String id) async {
    await (db.update(db.localNotes)..where((t) => t.id.equals(id))).write(
      const LocalNotesCompanion(
        createKey: Value(null),
        createSnapshot: Value(null),
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
        SELECT n.id, n.title, n.content, n.visibility, n.updated_at, n.owner_user_id, n.version, n.needs_sync,
               n.create_key, n.create_snapshot,
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
          version: row.readNullable<int>('version'),
          title: row.read<String>('title'),
          content: row.read<String>('content'),
          visibility: row.read<String>('visibility'),
          updatedAt: row.read<DateTime>('updated_at'),
          ownerUserId: row.readNullable<String>('owner_user_id'),
          createKey: row.readNullable<String>('create_key'),
          createSnapshot: row.readNullable<String>('create_snapshot'),
          needsSync: row.read<bool>('needs_sync'),
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
    await db.transaction(() async {
      await (db.delete(
        db.pendingNoteAttachments,
      )..where((t) => t.noteId.equals(id))).go();
      await (db.delete(db.localNotes)..where((t) => t.id.equals(id))).go();
    });
  }

  /// Tope por archivo pendiente: el mismo que acepta el backend (10 MB).
  static const int maxPendingAttachmentBytes = 10 * 1024 * 1024;

  /// Tope total de bytes pendientes por cuenta en SQLite. Evita que una sesión
  /// offline larga haga crecer la base sin límite.
  static const int maxPendingBytesPerOwner = 100 * 1024 * 1024;

  /// Bytes de adjuntos pendientes ya guardados para [ownerId].
  Future<int> pendingAttachmentBytes(String? ownerId) async {
    final row = await db
        .customSelect(
          'SELECT COALESCE(SUM(length(bytes)), 0) AS total '
          'FROM pending_note_attachments WHERE owner_user_id IS ?',
          variables: [Variable<String>(ownerId)],
          readsFrom: {db.pendingNoteAttachments},
        )
        .getSingle();
    return row.read<int>('total');
  }

  /// true si [incomingBytes] más lo ya pendiente cabe en la cuota del dueño.
  Future<bool> canStorePendingAttachments(
    String? ownerId,
    Iterable<int> incomingBytes,
  ) async {
    var incoming = 0;
    for (final size in incomingBytes) {
      if (size > maxPendingAttachmentBytes) return false;
      incoming += size;
    }
    if (incoming == 0) return true;
    return await pendingAttachmentBytes(ownerId) + incoming <=
        maxPendingBytesPerOwner;
  }

  /// Persiste los bytes de un adjunto seleccionado antes de que la nota exista
  /// en el backend, para que sobreviva a un reinicio. Rechaza archivos sobre
  /// [maxPendingAttachmentBytes] y escrituras que superen la cuota por dueño.
  Future<void> savePendingAttachment({
    required String placeholder,
    required String noteId,
    required String? ownerId,
    required String fileName,
    required String mime,
    required Uint8List bytes,
  }) async {
    if (bytes.length > maxPendingAttachmentBytes) {
      throw ArgumentError.value(
        bytes.length,
        'bytes',
        'El adjunto supera el límite de 10 MB',
      );
    }
    final existing = await (db.select(
      db.pendingNoteAttachments,
    )..where((t) => t.placeholder.equals(placeholder))).getSingleOrNull();
    final already = await pendingAttachmentBytes(ownerId);
    final replaced = existing?.ownerUserId == ownerId
        ? (existing?.bytes.length ?? 0)
        : 0;
    if (already - replaced + bytes.length > maxPendingBytesPerOwner) {
      throw StateError('Cuota de adjuntos offline excedida');
    }
    await db
        .into(db.pendingNoteAttachments)
        .insertOnConflictUpdate(
          PendingNoteAttachmentsCompanion.insert(
            placeholder: placeholder,
            noteId: noteId,
            ownerUserId: Value(ownerId),
            fileName: fileName,
            mime: mime,
            bytes: bytes,
          ),
        );
  }

  /// Adjuntos pendientes de una nota (diagnóstico y tests de integridad).
  Future<List<PendingNoteAttachment>> pendingAttachmentsForNote(String noteId) {
    return (db.select(
      db.pendingNoteAttachments,
    )..where((t) => t.noteId.equals(noteId))).get();
  }

  Future<PendingNoteAttachment?> pendingAttachment(
    String placeholder,
    String? ownerId,
  ) {
    return (db.select(db.pendingNoteAttachments)..where(
          (t) =>
              t.placeholder.equals(placeholder) &
              (ownerId == null
                  ? t.ownerUserId.isNull()
                  : t.ownerUserId.equals(ownerId)),
        ))
        .getSingleOrNull();
  }

  Future<void> recordAttachmentUpload(
    String placeholder,
    String attachmentId,
  ) async {
    await (db.update(
      db.pendingNoteAttachments,
    )..where((t) => t.placeholder.equals(placeholder))).write(
      PendingNoteAttachmentsCompanion(uploadedId: Value(attachmentId)),
    );
  }

  Future<void> removePendingAttachment(String placeholder) async {
    await (db.delete(
      db.pendingNoteAttachments,
    )..where((t) => t.placeholder.equals(placeholder))).go();
  }

  /// Limpia todo (para tests o logout).
  /// Opera sobre `local_notes`; los triggers mantienen el índice consistente.
  Future<void> clearAll() async {
    await db.delete(db.pendingNoteAttachments).go();
    await db.delete(db.localNotes).go();
  }

  /// Limpia solo las filas de un dueño (logout selectivo).
  Future<void> clearOwner(String ownerId) async {
    await (db.delete(
      db.pendingNoteAttachments,
    )..where((t) => t.ownerUserId.equals(ownerId))).go();
    await (db.delete(
      db.localNotes,
    )..where((t) => t.ownerUserId.equals(ownerId))).go();
  }

  /// Registra (upsert) un tombstone de borrado persistente (Fase 1 §P3).
  /// [remotePending] indica si el borrado remoto aún debe reintentarse.
  Future<void> addTombstone({
    required String id,
    String? ownerUserId,
    bool remotePending = true,
    String? lastError,
  }) async {
    await db
        .into(db.noteTombstones)
        .insertOnConflictUpdate(
          NoteTombstonesCompanion.insert(
            id: id,
            ownerUserId: Value(ownerUserId),
            deletedAt: DateTime.now(),
            remotePending: Value(remotePending),
            lastError: Value(lastError),
          ),
        );
  }

  /// Actualiza el estado del tombstone tras un intento de borrado remoto.
  Future<void> updateTombstone(
    String id, {
    required bool remotePending,
    String? lastError,
  }) async {
    await (db.update(db.noteTombstones)..where((t) => t.id.equals(id))).write(
      NoteTombstonesCompanion(
        remotePending: Value(remotePending),
        lastError: Value(lastError),
      ),
    );
  }

  /// Elimina el tombstone de [id] (el backend ya no devuelve la nota).
  Future<void> removeTombstone(String id) async {
    await (db.delete(db.noteTombstones)..where((t) => t.id.equals(id))).go();
  }

  /// Lista los tombstones del dueño (o los legacy sin dueño si [ownerId] nulo).
  Future<List<NoteTombstoneEntry>> getTombstones({String? ownerId}) async {
    final query = db.select(db.noteTombstones);
    if (ownerId == null) {
      query.where((t) => t.ownerUserId.isNull());
    } else {
      query.where((t) => t.ownerUserId.equals(ownerId));
    }
    return query.get();
  }

  /// Elimina todos los tombstones del dueño (logout selectivo).
  Future<void> clearOwnerTombstones(String ownerId) async {
    await (db.delete(
      db.noteTombstones,
    )..where((t) => t.ownerUserId.equals(ownerId))).go();
  }
}
