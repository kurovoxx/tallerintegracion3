// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'app_database.dart';

// ignore_for_file: type=lint
class $LocalNotesTable extends LocalNotes
    with TableInfo<$LocalNotesTable, LocalNote> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $LocalNotesTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    additionalChecks: GeneratedColumn.checkTextLength(
      minTextLength: 1,
      maxTextLength: 300,
    ),
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _contentMeta = const VerificationMeta(
    'content',
  );
  @override
  late final GeneratedColumn<String> content = GeneratedColumn<String>(
    'content',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _visibilityMeta = const VerificationMeta(
    'visibility',
  );
  @override
  late final GeneratedColumn<String> visibility = GeneratedColumn<String>(
    'visibility',
    aliasedName,
    false,
    additionalChecks: GeneratedColumn.checkTextLength(
      minTextLength: 1,
      maxTextLength: 20,
    ),
    type: DriftSqlType.string,
    requiredDuringInsert: false,
    defaultValue: const Constant('private'),
  );
  static const VerificationMeta _updatedAtMeta = const VerificationMeta(
    'updatedAt',
  );
  @override
  late final GeneratedColumn<DateTime> updatedAt = GeneratedColumn<DateTime>(
    'updated_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _versionMeta = const VerificationMeta(
    'version',
  );
  @override
  late final GeneratedColumn<int> version = GeneratedColumn<int>(
    'version',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ownerUserIdMeta = const VerificationMeta(
    'ownerUserId',
  );
  @override
  late final GeneratedColumn<String> ownerUserId = GeneratedColumn<String>(
    'owner_user_id',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _createKeyMeta = const VerificationMeta(
    'createKey',
  );
  @override
  late final GeneratedColumn<String> createKey = GeneratedColumn<String>(
    'create_key',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _createSnapshotMeta = const VerificationMeta(
    'createSnapshot',
  );
  @override
  late final GeneratedColumn<String> createSnapshot = GeneratedColumn<String>(
    'create_snapshot',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _needsSyncMeta = const VerificationMeta(
    'needsSync',
  );
  @override
  late final GeneratedColumn<bool> needsSync = GeneratedColumn<bool>(
    'needs_sync',
    aliasedName,
    false,
    type: DriftSqlType.bool,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'CHECK ("needs_sync" IN (0, 1))',
    ),
    defaultValue: const Constant(false),
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    title,
    content,
    visibility,
    updatedAt,
    version,
    ownerUserId,
    createKey,
    createSnapshot,
    needsSync,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'local_notes';
  @override
  VerificationContext validateIntegrity(
    Insertable<LocalNote> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    } else if (isInserting) {
      context.missing(_titleMeta);
    }
    if (data.containsKey('content')) {
      context.handle(
        _contentMeta,
        content.isAcceptableOrUnknown(data['content']!, _contentMeta),
      );
    } else if (isInserting) {
      context.missing(_contentMeta);
    }
    if (data.containsKey('visibility')) {
      context.handle(
        _visibilityMeta,
        visibility.isAcceptableOrUnknown(data['visibility']!, _visibilityMeta),
      );
    }
    if (data.containsKey('updated_at')) {
      context.handle(
        _updatedAtMeta,
        updatedAt.isAcceptableOrUnknown(data['updated_at']!, _updatedAtMeta),
      );
    } else if (isInserting) {
      context.missing(_updatedAtMeta);
    }
    if (data.containsKey('version')) {
      context.handle(
        _versionMeta,
        version.isAcceptableOrUnknown(data['version']!, _versionMeta),
      );
    }
    if (data.containsKey('owner_user_id')) {
      context.handle(
        _ownerUserIdMeta,
        ownerUserId.isAcceptableOrUnknown(
          data['owner_user_id']!,
          _ownerUserIdMeta,
        ),
      );
    }
    if (data.containsKey('create_key')) {
      context.handle(
        _createKeyMeta,
        createKey.isAcceptableOrUnknown(data['create_key']!, _createKeyMeta),
      );
    }
    if (data.containsKey('create_snapshot')) {
      context.handle(
        _createSnapshotMeta,
        createSnapshot.isAcceptableOrUnknown(
          data['create_snapshot']!,
          _createSnapshotMeta,
        ),
      );
    }
    if (data.containsKey('needs_sync')) {
      context.handle(
        _needsSyncMeta,
        needsSync.isAcceptableOrUnknown(data['needs_sync']!, _needsSyncMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  LocalNote map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return LocalNote(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      content: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}content'],
      )!,
      visibility: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}visibility'],
      )!,
      updatedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}updated_at'],
      )!,
      version: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}version'],
      ),
      ownerUserId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}owner_user_id'],
      ),
      createKey: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}create_key'],
      ),
      createSnapshot: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}create_snapshot'],
      ),
      needsSync: attachedDatabase.typeMapping.read(
        DriftSqlType.bool,
        data['${effectivePrefix}needs_sync'],
      )!,
    );
  }

  @override
  $LocalNotesTable createAlias(String alias) {
    return $LocalNotesTable(attachedDatabase, alias);
  }
}

class LocalNote extends DataClass implements Insertable<LocalNote> {
  final String id;
  final String title;
  final String content;
  final String visibility;
  final DateTime updatedAt;
  final int? version;
  final String? ownerUserId;
  final String? createKey;
  final String? createSnapshot;
  final bool needsSync;
  const LocalNote({
    required this.id,
    required this.title,
    required this.content,
    required this.visibility,
    required this.updatedAt,
    this.version,
    this.ownerUserId,
    this.createKey,
    this.createSnapshot,
    required this.needsSync,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['title'] = Variable<String>(title);
    map['content'] = Variable<String>(content);
    map['visibility'] = Variable<String>(visibility);
    map['updated_at'] = Variable<DateTime>(updatedAt);
    if (!nullToAbsent || version != null) {
      map['version'] = Variable<int>(version);
    }
    if (!nullToAbsent || ownerUserId != null) {
      map['owner_user_id'] = Variable<String>(ownerUserId);
    }
    if (!nullToAbsent || createKey != null) {
      map['create_key'] = Variable<String>(createKey);
    }
    if (!nullToAbsent || createSnapshot != null) {
      map['create_snapshot'] = Variable<String>(createSnapshot);
    }
    map['needs_sync'] = Variable<bool>(needsSync);
    return map;
  }

  LocalNotesCompanion toCompanion(bool nullToAbsent) {
    return LocalNotesCompanion(
      id: Value(id),
      title: Value(title),
      content: Value(content),
      visibility: Value(visibility),
      updatedAt: Value(updatedAt),
      version: version == null && nullToAbsent
          ? const Value.absent()
          : Value(version),
      ownerUserId: ownerUserId == null && nullToAbsent
          ? const Value.absent()
          : Value(ownerUserId),
      createKey: createKey == null && nullToAbsent
          ? const Value.absent()
          : Value(createKey),
      createSnapshot: createSnapshot == null && nullToAbsent
          ? const Value.absent()
          : Value(createSnapshot),
      needsSync: Value(needsSync),
    );
  }

  factory LocalNote.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return LocalNote(
      id: serializer.fromJson<String>(json['id']),
      title: serializer.fromJson<String>(json['title']),
      content: serializer.fromJson<String>(json['content']),
      visibility: serializer.fromJson<String>(json['visibility']),
      updatedAt: serializer.fromJson<DateTime>(json['updatedAt']),
      version: serializer.fromJson<int?>(json['version']),
      ownerUserId: serializer.fromJson<String?>(json['ownerUserId']),
      createKey: serializer.fromJson<String?>(json['createKey']),
      createSnapshot: serializer.fromJson<String?>(json['createSnapshot']),
      needsSync: serializer.fromJson<bool>(json['needsSync']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'title': serializer.toJson<String>(title),
      'content': serializer.toJson<String>(content),
      'visibility': serializer.toJson<String>(visibility),
      'updatedAt': serializer.toJson<DateTime>(updatedAt),
      'version': serializer.toJson<int?>(version),
      'ownerUserId': serializer.toJson<String?>(ownerUserId),
      'createKey': serializer.toJson<String?>(createKey),
      'createSnapshot': serializer.toJson<String?>(createSnapshot),
      'needsSync': serializer.toJson<bool>(needsSync),
    };
  }

  LocalNote copyWith({
    String? id,
    String? title,
    String? content,
    String? visibility,
    DateTime? updatedAt,
    Value<int?> version = const Value.absent(),
    Value<String?> ownerUserId = const Value.absent(),
    Value<String?> createKey = const Value.absent(),
    Value<String?> createSnapshot = const Value.absent(),
    bool? needsSync,
  }) => LocalNote(
    id: id ?? this.id,
    title: title ?? this.title,
    content: content ?? this.content,
    visibility: visibility ?? this.visibility,
    updatedAt: updatedAt ?? this.updatedAt,
    version: version.present ? version.value : this.version,
    ownerUserId: ownerUserId.present ? ownerUserId.value : this.ownerUserId,
    createKey: createKey.present ? createKey.value : this.createKey,
    createSnapshot: createSnapshot.present
        ? createSnapshot.value
        : this.createSnapshot,
    needsSync: needsSync ?? this.needsSync,
  );
  LocalNote copyWithCompanion(LocalNotesCompanion data) {
    return LocalNote(
      id: data.id.present ? data.id.value : this.id,
      title: data.title.present ? data.title.value : this.title,
      content: data.content.present ? data.content.value : this.content,
      visibility: data.visibility.present
          ? data.visibility.value
          : this.visibility,
      updatedAt: data.updatedAt.present ? data.updatedAt.value : this.updatedAt,
      version: data.version.present ? data.version.value : this.version,
      ownerUserId: data.ownerUserId.present
          ? data.ownerUserId.value
          : this.ownerUserId,
      createKey: data.createKey.present ? data.createKey.value : this.createKey,
      createSnapshot: data.createSnapshot.present
          ? data.createSnapshot.value
          : this.createSnapshot,
      needsSync: data.needsSync.present ? data.needsSync.value : this.needsSync,
    );
  }

  @override
  String toString() {
    return (StringBuffer('LocalNote(')
          ..write('id: $id, ')
          ..write('title: $title, ')
          ..write('content: $content, ')
          ..write('visibility: $visibility, ')
          ..write('updatedAt: $updatedAt, ')
          ..write('version: $version, ')
          ..write('ownerUserId: $ownerUserId, ')
          ..write('createKey: $createKey, ')
          ..write('createSnapshot: $createSnapshot, ')
          ..write('needsSync: $needsSync')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    title,
    content,
    visibility,
    updatedAt,
    version,
    ownerUserId,
    createKey,
    createSnapshot,
    needsSync,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is LocalNote &&
          other.id == this.id &&
          other.title == this.title &&
          other.content == this.content &&
          other.visibility == this.visibility &&
          other.updatedAt == this.updatedAt &&
          other.version == this.version &&
          other.ownerUserId == this.ownerUserId &&
          other.createKey == this.createKey &&
          other.createSnapshot == this.createSnapshot &&
          other.needsSync == this.needsSync);
}

class LocalNotesCompanion extends UpdateCompanion<LocalNote> {
  final Value<String> id;
  final Value<String> title;
  final Value<String> content;
  final Value<String> visibility;
  final Value<DateTime> updatedAt;
  final Value<int?> version;
  final Value<String?> ownerUserId;
  final Value<String?> createKey;
  final Value<String?> createSnapshot;
  final Value<bool> needsSync;
  final Value<int> rowid;
  const LocalNotesCompanion({
    this.id = const Value.absent(),
    this.title = const Value.absent(),
    this.content = const Value.absent(),
    this.visibility = const Value.absent(),
    this.updatedAt = const Value.absent(),
    this.version = const Value.absent(),
    this.ownerUserId = const Value.absent(),
    this.createKey = const Value.absent(),
    this.createSnapshot = const Value.absent(),
    this.needsSync = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  LocalNotesCompanion.insert({
    required String id,
    required String title,
    required String content,
    this.visibility = const Value.absent(),
    required DateTime updatedAt,
    this.version = const Value.absent(),
    this.ownerUserId = const Value.absent(),
    this.createKey = const Value.absent(),
    this.createSnapshot = const Value.absent(),
    this.needsSync = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       title = Value(title),
       content = Value(content),
       updatedAt = Value(updatedAt);
  static Insertable<LocalNote> custom({
    Expression<String>? id,
    Expression<String>? title,
    Expression<String>? content,
    Expression<String>? visibility,
    Expression<DateTime>? updatedAt,
    Expression<int>? version,
    Expression<String>? ownerUserId,
    Expression<String>? createKey,
    Expression<String>? createSnapshot,
    Expression<bool>? needsSync,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (title != null) 'title': title,
      if (content != null) 'content': content,
      if (visibility != null) 'visibility': visibility,
      if (updatedAt != null) 'updated_at': updatedAt,
      if (version != null) 'version': version,
      if (ownerUserId != null) 'owner_user_id': ownerUserId,
      if (createKey != null) 'create_key': createKey,
      if (createSnapshot != null) 'create_snapshot': createSnapshot,
      if (needsSync != null) 'needs_sync': needsSync,
      if (rowid != null) 'rowid': rowid,
    });
  }

  LocalNotesCompanion copyWith({
    Value<String>? id,
    Value<String>? title,
    Value<String>? content,
    Value<String>? visibility,
    Value<DateTime>? updatedAt,
    Value<int?>? version,
    Value<String?>? ownerUserId,
    Value<String?>? createKey,
    Value<String?>? createSnapshot,
    Value<bool>? needsSync,
    Value<int>? rowid,
  }) {
    return LocalNotesCompanion(
      id: id ?? this.id,
      title: title ?? this.title,
      content: content ?? this.content,
      visibility: visibility ?? this.visibility,
      updatedAt: updatedAt ?? this.updatedAt,
      version: version ?? this.version,
      ownerUserId: ownerUserId ?? this.ownerUserId,
      createKey: createKey ?? this.createKey,
      createSnapshot: createSnapshot ?? this.createSnapshot,
      needsSync: needsSync ?? this.needsSync,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (content.present) {
      map['content'] = Variable<String>(content.value);
    }
    if (visibility.present) {
      map['visibility'] = Variable<String>(visibility.value);
    }
    if (updatedAt.present) {
      map['updated_at'] = Variable<DateTime>(updatedAt.value);
    }
    if (version.present) {
      map['version'] = Variable<int>(version.value);
    }
    if (ownerUserId.present) {
      map['owner_user_id'] = Variable<String>(ownerUserId.value);
    }
    if (createKey.present) {
      map['create_key'] = Variable<String>(createKey.value);
    }
    if (createSnapshot.present) {
      map['create_snapshot'] = Variable<String>(createSnapshot.value);
    }
    if (needsSync.present) {
      map['needs_sync'] = Variable<bool>(needsSync.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('LocalNotesCompanion(')
          ..write('id: $id, ')
          ..write('title: $title, ')
          ..write('content: $content, ')
          ..write('visibility: $visibility, ')
          ..write('updatedAt: $updatedAt, ')
          ..write('version: $version, ')
          ..write('ownerUserId: $ownerUserId, ')
          ..write('createKey: $createKey, ')
          ..write('createSnapshot: $createSnapshot, ')
          ..write('needsSync: $needsSync, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $LocalNotesFtsTable extends LocalNotesFts
    with TableInfo<$LocalNotesFtsTable, LocalNoteFtsEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $LocalNotesFtsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _contentMeta = const VerificationMeta(
    'content',
  );
  @override
  late final GeneratedColumn<String> content = GeneratedColumn<String>(
    'content',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [title, content];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'local_notes_fts';
  @override
  VerificationContext validateIntegrity(
    Insertable<LocalNoteFtsEntry> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    } else if (isInserting) {
      context.missing(_titleMeta);
    }
    if (data.containsKey('content')) {
      context.handle(
        _contentMeta,
        content.isAcceptableOrUnknown(data['content']!, _contentMeta),
      );
    } else if (isInserting) {
      context.missing(_contentMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => const {};
  @override
  LocalNoteFtsEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return LocalNoteFtsEntry(
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      content: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}content'],
      )!,
    );
  }

  @override
  $LocalNotesFtsTable createAlias(String alias) {
    return $LocalNotesFtsTable(attachedDatabase, alias);
  }
}

class LocalNoteFtsEntry extends DataClass
    implements Insertable<LocalNoteFtsEntry> {
  final String title;
  final String content;
  const LocalNoteFtsEntry({required this.title, required this.content});
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['title'] = Variable<String>(title);
    map['content'] = Variable<String>(content);
    return map;
  }

  LocalNotesFtsCompanion toCompanion(bool nullToAbsent) {
    return LocalNotesFtsCompanion(title: Value(title), content: Value(content));
  }

  factory LocalNoteFtsEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return LocalNoteFtsEntry(
      title: serializer.fromJson<String>(json['title']),
      content: serializer.fromJson<String>(json['content']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'title': serializer.toJson<String>(title),
      'content': serializer.toJson<String>(content),
    };
  }

  LocalNoteFtsEntry copyWith({String? title, String? content}) =>
      LocalNoteFtsEntry(
        title: title ?? this.title,
        content: content ?? this.content,
      );
  LocalNoteFtsEntry copyWithCompanion(LocalNotesFtsCompanion data) {
    return LocalNoteFtsEntry(
      title: data.title.present ? data.title.value : this.title,
      content: data.content.present ? data.content.value : this.content,
    );
  }

  @override
  String toString() {
    return (StringBuffer('LocalNoteFtsEntry(')
          ..write('title: $title, ')
          ..write('content: $content')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(title, content);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is LocalNoteFtsEntry &&
          other.title == this.title &&
          other.content == this.content);
}

class LocalNotesFtsCompanion extends UpdateCompanion<LocalNoteFtsEntry> {
  final Value<String> title;
  final Value<String> content;
  final Value<int> rowid;
  const LocalNotesFtsCompanion({
    this.title = const Value.absent(),
    this.content = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  LocalNotesFtsCompanion.insert({
    required String title,
    required String content,
    this.rowid = const Value.absent(),
  }) : title = Value(title),
       content = Value(content);
  static Insertable<LocalNoteFtsEntry> custom({
    Expression<String>? title,
    Expression<String>? content,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (title != null) 'title': title,
      if (content != null) 'content': content,
      if (rowid != null) 'rowid': rowid,
    });
  }

  LocalNotesFtsCompanion copyWith({
    Value<String>? title,
    Value<String>? content,
    Value<int>? rowid,
  }) {
    return LocalNotesFtsCompanion(
      title: title ?? this.title,
      content: content ?? this.content,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (content.present) {
      map['content'] = Variable<String>(content.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('LocalNotesFtsCompanion(')
          ..write('title: $title, ')
          ..write('content: $content, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $NoteTombstonesTable extends NoteTombstones
    with TableInfo<$NoteTombstonesTable, NoteTombstoneEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $NoteTombstonesTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _ownerUserIdMeta = const VerificationMeta(
    'ownerUserId',
  );
  @override
  late final GeneratedColumn<String> ownerUserId = GeneratedColumn<String>(
    'owner_user_id',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _deletedAtMeta = const VerificationMeta(
    'deletedAt',
  );
  @override
  late final GeneratedColumn<DateTime> deletedAt = GeneratedColumn<DateTime>(
    'deleted_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _remotePendingMeta = const VerificationMeta(
    'remotePending',
  );
  @override
  late final GeneratedColumn<bool> remotePending = GeneratedColumn<bool>(
    'remote_pending',
    aliasedName,
    false,
    type: DriftSqlType.bool,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'CHECK ("remote_pending" IN (0, 1))',
    ),
    defaultValue: const Constant(true),
  );
  static const VerificationMeta _lastErrorMeta = const VerificationMeta(
    'lastError',
  );
  @override
  late final GeneratedColumn<String> lastError = GeneratedColumn<String>(
    'last_error',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    ownerUserId,
    deletedAt,
    remotePending,
    lastError,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'note_tombstones';
  @override
  VerificationContext validateIntegrity(
    Insertable<NoteTombstoneEntry> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('owner_user_id')) {
      context.handle(
        _ownerUserIdMeta,
        ownerUserId.isAcceptableOrUnknown(
          data['owner_user_id']!,
          _ownerUserIdMeta,
        ),
      );
    }
    if (data.containsKey('deleted_at')) {
      context.handle(
        _deletedAtMeta,
        deletedAt.isAcceptableOrUnknown(data['deleted_at']!, _deletedAtMeta),
      );
    } else if (isInserting) {
      context.missing(_deletedAtMeta);
    }
    if (data.containsKey('remote_pending')) {
      context.handle(
        _remotePendingMeta,
        remotePending.isAcceptableOrUnknown(
          data['remote_pending']!,
          _remotePendingMeta,
        ),
      );
    }
    if (data.containsKey('last_error')) {
      context.handle(
        _lastErrorMeta,
        lastError.isAcceptableOrUnknown(data['last_error']!, _lastErrorMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  NoteTombstoneEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return NoteTombstoneEntry(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      ownerUserId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}owner_user_id'],
      ),
      deletedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}deleted_at'],
      )!,
      remotePending: attachedDatabase.typeMapping.read(
        DriftSqlType.bool,
        data['${effectivePrefix}remote_pending'],
      )!,
      lastError: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}last_error'],
      ),
    );
  }

  @override
  $NoteTombstonesTable createAlias(String alias) {
    return $NoteTombstonesTable(attachedDatabase, alias);
  }
}

class NoteTombstoneEntry extends DataClass
    implements Insertable<NoteTombstoneEntry> {
  final String id;
  final String? ownerUserId;
  final DateTime deletedAt;
  final bool remotePending;
  final String? lastError;
  const NoteTombstoneEntry({
    required this.id,
    this.ownerUserId,
    required this.deletedAt,
    required this.remotePending,
    this.lastError,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    if (!nullToAbsent || ownerUserId != null) {
      map['owner_user_id'] = Variable<String>(ownerUserId);
    }
    map['deleted_at'] = Variable<DateTime>(deletedAt);
    map['remote_pending'] = Variable<bool>(remotePending);
    if (!nullToAbsent || lastError != null) {
      map['last_error'] = Variable<String>(lastError);
    }
    return map;
  }

  NoteTombstonesCompanion toCompanion(bool nullToAbsent) {
    return NoteTombstonesCompanion(
      id: Value(id),
      ownerUserId: ownerUserId == null && nullToAbsent
          ? const Value.absent()
          : Value(ownerUserId),
      deletedAt: Value(deletedAt),
      remotePending: Value(remotePending),
      lastError: lastError == null && nullToAbsent
          ? const Value.absent()
          : Value(lastError),
    );
  }

  factory NoteTombstoneEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return NoteTombstoneEntry(
      id: serializer.fromJson<String>(json['id']),
      ownerUserId: serializer.fromJson<String?>(json['ownerUserId']),
      deletedAt: serializer.fromJson<DateTime>(json['deletedAt']),
      remotePending: serializer.fromJson<bool>(json['remotePending']),
      lastError: serializer.fromJson<String?>(json['lastError']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'ownerUserId': serializer.toJson<String?>(ownerUserId),
      'deletedAt': serializer.toJson<DateTime>(deletedAt),
      'remotePending': serializer.toJson<bool>(remotePending),
      'lastError': serializer.toJson<String?>(lastError),
    };
  }

  NoteTombstoneEntry copyWith({
    String? id,
    Value<String?> ownerUserId = const Value.absent(),
    DateTime? deletedAt,
    bool? remotePending,
    Value<String?> lastError = const Value.absent(),
  }) => NoteTombstoneEntry(
    id: id ?? this.id,
    ownerUserId: ownerUserId.present ? ownerUserId.value : this.ownerUserId,
    deletedAt: deletedAt ?? this.deletedAt,
    remotePending: remotePending ?? this.remotePending,
    lastError: lastError.present ? lastError.value : this.lastError,
  );
  NoteTombstoneEntry copyWithCompanion(NoteTombstonesCompanion data) {
    return NoteTombstoneEntry(
      id: data.id.present ? data.id.value : this.id,
      ownerUserId: data.ownerUserId.present
          ? data.ownerUserId.value
          : this.ownerUserId,
      deletedAt: data.deletedAt.present ? data.deletedAt.value : this.deletedAt,
      remotePending: data.remotePending.present
          ? data.remotePending.value
          : this.remotePending,
      lastError: data.lastError.present ? data.lastError.value : this.lastError,
    );
  }

  @override
  String toString() {
    return (StringBuffer('NoteTombstoneEntry(')
          ..write('id: $id, ')
          ..write('ownerUserId: $ownerUserId, ')
          ..write('deletedAt: $deletedAt, ')
          ..write('remotePending: $remotePending, ')
          ..write('lastError: $lastError')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode =>
      Object.hash(id, ownerUserId, deletedAt, remotePending, lastError);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is NoteTombstoneEntry &&
          other.id == this.id &&
          other.ownerUserId == this.ownerUserId &&
          other.deletedAt == this.deletedAt &&
          other.remotePending == this.remotePending &&
          other.lastError == this.lastError);
}

class NoteTombstonesCompanion extends UpdateCompanion<NoteTombstoneEntry> {
  final Value<String> id;
  final Value<String?> ownerUserId;
  final Value<DateTime> deletedAt;
  final Value<bool> remotePending;
  final Value<String?> lastError;
  final Value<int> rowid;
  const NoteTombstonesCompanion({
    this.id = const Value.absent(),
    this.ownerUserId = const Value.absent(),
    this.deletedAt = const Value.absent(),
    this.remotePending = const Value.absent(),
    this.lastError = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  NoteTombstonesCompanion.insert({
    required String id,
    this.ownerUserId = const Value.absent(),
    required DateTime deletedAt,
    this.remotePending = const Value.absent(),
    this.lastError = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       deletedAt = Value(deletedAt);
  static Insertable<NoteTombstoneEntry> custom({
    Expression<String>? id,
    Expression<String>? ownerUserId,
    Expression<DateTime>? deletedAt,
    Expression<bool>? remotePending,
    Expression<String>? lastError,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (ownerUserId != null) 'owner_user_id': ownerUserId,
      if (deletedAt != null) 'deleted_at': deletedAt,
      if (remotePending != null) 'remote_pending': remotePending,
      if (lastError != null) 'last_error': lastError,
      if (rowid != null) 'rowid': rowid,
    });
  }

  NoteTombstonesCompanion copyWith({
    Value<String>? id,
    Value<String?>? ownerUserId,
    Value<DateTime>? deletedAt,
    Value<bool>? remotePending,
    Value<String?>? lastError,
    Value<int>? rowid,
  }) {
    return NoteTombstonesCompanion(
      id: id ?? this.id,
      ownerUserId: ownerUserId ?? this.ownerUserId,
      deletedAt: deletedAt ?? this.deletedAt,
      remotePending: remotePending ?? this.remotePending,
      lastError: lastError ?? this.lastError,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (ownerUserId.present) {
      map['owner_user_id'] = Variable<String>(ownerUserId.value);
    }
    if (deletedAt.present) {
      map['deleted_at'] = Variable<DateTime>(deletedAt.value);
    }
    if (remotePending.present) {
      map['remote_pending'] = Variable<bool>(remotePending.value);
    }
    if (lastError.present) {
      map['last_error'] = Variable<String>(lastError.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('NoteTombstonesCompanion(')
          ..write('id: $id, ')
          ..write('ownerUserId: $ownerUserId, ')
          ..write('deletedAt: $deletedAt, ')
          ..write('remotePending: $remotePending, ')
          ..write('lastError: $lastError, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $PendingNoteAttachmentsTable extends PendingNoteAttachments
    with TableInfo<$PendingNoteAttachmentsTable, PendingNoteAttachment> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $PendingNoteAttachmentsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _placeholderMeta = const VerificationMeta(
    'placeholder',
  );
  @override
  late final GeneratedColumn<String> placeholder = GeneratedColumn<String>(
    'placeholder',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _noteIdMeta = const VerificationMeta('noteId');
  @override
  late final GeneratedColumn<String> noteId = GeneratedColumn<String>(
    'note_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _ownerUserIdMeta = const VerificationMeta(
    'ownerUserId',
  );
  @override
  late final GeneratedColumn<String> ownerUserId = GeneratedColumn<String>(
    'owner_user_id',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _fileNameMeta = const VerificationMeta(
    'fileName',
  );
  @override
  late final GeneratedColumn<String> fileName = GeneratedColumn<String>(
    'file_name',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _mimeMeta = const VerificationMeta('mime');
  @override
  late final GeneratedColumn<String> mime = GeneratedColumn<String>(
    'mime',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _bytesMeta = const VerificationMeta('bytes');
  @override
  late final GeneratedColumn<Uint8List> bytes = GeneratedColumn<Uint8List>(
    'bytes',
    aliasedName,
    false,
    type: DriftSqlType.blob,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _uploadedIdMeta = const VerificationMeta(
    'uploadedId',
  );
  @override
  late final GeneratedColumn<String> uploadedId = GeneratedColumn<String>(
    'uploaded_id',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  @override
  List<GeneratedColumn> get $columns => [
    placeholder,
    noteId,
    ownerUserId,
    fileName,
    mime,
    bytes,
    uploadedId,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'pending_note_attachments';
  @override
  VerificationContext validateIntegrity(
    Insertable<PendingNoteAttachment> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('placeholder')) {
      context.handle(
        _placeholderMeta,
        placeholder.isAcceptableOrUnknown(
          data['placeholder']!,
          _placeholderMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_placeholderMeta);
    }
    if (data.containsKey('note_id')) {
      context.handle(
        _noteIdMeta,
        noteId.isAcceptableOrUnknown(data['note_id']!, _noteIdMeta),
      );
    } else if (isInserting) {
      context.missing(_noteIdMeta);
    }
    if (data.containsKey('owner_user_id')) {
      context.handle(
        _ownerUserIdMeta,
        ownerUserId.isAcceptableOrUnknown(
          data['owner_user_id']!,
          _ownerUserIdMeta,
        ),
      );
    }
    if (data.containsKey('file_name')) {
      context.handle(
        _fileNameMeta,
        fileName.isAcceptableOrUnknown(data['file_name']!, _fileNameMeta),
      );
    } else if (isInserting) {
      context.missing(_fileNameMeta);
    }
    if (data.containsKey('mime')) {
      context.handle(
        _mimeMeta,
        mime.isAcceptableOrUnknown(data['mime']!, _mimeMeta),
      );
    } else if (isInserting) {
      context.missing(_mimeMeta);
    }
    if (data.containsKey('bytes')) {
      context.handle(
        _bytesMeta,
        bytes.isAcceptableOrUnknown(data['bytes']!, _bytesMeta),
      );
    } else if (isInserting) {
      context.missing(_bytesMeta);
    }
    if (data.containsKey('uploaded_id')) {
      context.handle(
        _uploadedIdMeta,
        uploadedId.isAcceptableOrUnknown(data['uploaded_id']!, _uploadedIdMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {placeholder};
  @override
  PendingNoteAttachment map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return PendingNoteAttachment(
      placeholder: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}placeholder'],
      )!,
      noteId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}note_id'],
      )!,
      ownerUserId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}owner_user_id'],
      ),
      fileName: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}file_name'],
      )!,
      mime: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}mime'],
      )!,
      bytes: attachedDatabase.typeMapping.read(
        DriftSqlType.blob,
        data['${effectivePrefix}bytes'],
      )!,
      uploadedId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}uploaded_id'],
      ),
    );
  }

  @override
  $PendingNoteAttachmentsTable createAlias(String alias) {
    return $PendingNoteAttachmentsTable(attachedDatabase, alias);
  }
}

class PendingNoteAttachment extends DataClass
    implements Insertable<PendingNoteAttachment> {
  final String placeholder;
  final String noteId;
  final String? ownerUserId;
  final String fileName;
  final String mime;
  final Uint8List bytes;
  final String? uploadedId;
  const PendingNoteAttachment({
    required this.placeholder,
    required this.noteId,
    this.ownerUserId,
    required this.fileName,
    required this.mime,
    required this.bytes,
    this.uploadedId,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['placeholder'] = Variable<String>(placeholder);
    map['note_id'] = Variable<String>(noteId);
    if (!nullToAbsent || ownerUserId != null) {
      map['owner_user_id'] = Variable<String>(ownerUserId);
    }
    map['file_name'] = Variable<String>(fileName);
    map['mime'] = Variable<String>(mime);
    map['bytes'] = Variable<Uint8List>(bytes);
    if (!nullToAbsent || uploadedId != null) {
      map['uploaded_id'] = Variable<String>(uploadedId);
    }
    return map;
  }

  PendingNoteAttachmentsCompanion toCompanion(bool nullToAbsent) {
    return PendingNoteAttachmentsCompanion(
      placeholder: Value(placeholder),
      noteId: Value(noteId),
      ownerUserId: ownerUserId == null && nullToAbsent
          ? const Value.absent()
          : Value(ownerUserId),
      fileName: Value(fileName),
      mime: Value(mime),
      bytes: Value(bytes),
      uploadedId: uploadedId == null && nullToAbsent
          ? const Value.absent()
          : Value(uploadedId),
    );
  }

  factory PendingNoteAttachment.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return PendingNoteAttachment(
      placeholder: serializer.fromJson<String>(json['placeholder']),
      noteId: serializer.fromJson<String>(json['noteId']),
      ownerUserId: serializer.fromJson<String?>(json['ownerUserId']),
      fileName: serializer.fromJson<String>(json['fileName']),
      mime: serializer.fromJson<String>(json['mime']),
      bytes: serializer.fromJson<Uint8List>(json['bytes']),
      uploadedId: serializer.fromJson<String?>(json['uploadedId']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'placeholder': serializer.toJson<String>(placeholder),
      'noteId': serializer.toJson<String>(noteId),
      'ownerUserId': serializer.toJson<String?>(ownerUserId),
      'fileName': serializer.toJson<String>(fileName),
      'mime': serializer.toJson<String>(mime),
      'bytes': serializer.toJson<Uint8List>(bytes),
      'uploadedId': serializer.toJson<String?>(uploadedId),
    };
  }

  PendingNoteAttachment copyWith({
    String? placeholder,
    String? noteId,
    Value<String?> ownerUserId = const Value.absent(),
    String? fileName,
    String? mime,
    Uint8List? bytes,
    Value<String?> uploadedId = const Value.absent(),
  }) => PendingNoteAttachment(
    placeholder: placeholder ?? this.placeholder,
    noteId: noteId ?? this.noteId,
    ownerUserId: ownerUserId.present ? ownerUserId.value : this.ownerUserId,
    fileName: fileName ?? this.fileName,
    mime: mime ?? this.mime,
    bytes: bytes ?? this.bytes,
    uploadedId: uploadedId.present ? uploadedId.value : this.uploadedId,
  );
  PendingNoteAttachment copyWithCompanion(
    PendingNoteAttachmentsCompanion data,
  ) {
    return PendingNoteAttachment(
      placeholder: data.placeholder.present
          ? data.placeholder.value
          : this.placeholder,
      noteId: data.noteId.present ? data.noteId.value : this.noteId,
      ownerUserId: data.ownerUserId.present
          ? data.ownerUserId.value
          : this.ownerUserId,
      fileName: data.fileName.present ? data.fileName.value : this.fileName,
      mime: data.mime.present ? data.mime.value : this.mime,
      bytes: data.bytes.present ? data.bytes.value : this.bytes,
      uploadedId: data.uploadedId.present
          ? data.uploadedId.value
          : this.uploadedId,
    );
  }

  @override
  String toString() {
    return (StringBuffer('PendingNoteAttachment(')
          ..write('placeholder: $placeholder, ')
          ..write('noteId: $noteId, ')
          ..write('ownerUserId: $ownerUserId, ')
          ..write('fileName: $fileName, ')
          ..write('mime: $mime, ')
          ..write('bytes: $bytes, ')
          ..write('uploadedId: $uploadedId')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    placeholder,
    noteId,
    ownerUserId,
    fileName,
    mime,
    $driftBlobEquality.hash(bytes),
    uploadedId,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is PendingNoteAttachment &&
          other.placeholder == this.placeholder &&
          other.noteId == this.noteId &&
          other.ownerUserId == this.ownerUserId &&
          other.fileName == this.fileName &&
          other.mime == this.mime &&
          $driftBlobEquality.equals(other.bytes, this.bytes) &&
          other.uploadedId == this.uploadedId);
}

class PendingNoteAttachmentsCompanion
    extends UpdateCompanion<PendingNoteAttachment> {
  final Value<String> placeholder;
  final Value<String> noteId;
  final Value<String?> ownerUserId;
  final Value<String> fileName;
  final Value<String> mime;
  final Value<Uint8List> bytes;
  final Value<String?> uploadedId;
  final Value<int> rowid;
  const PendingNoteAttachmentsCompanion({
    this.placeholder = const Value.absent(),
    this.noteId = const Value.absent(),
    this.ownerUserId = const Value.absent(),
    this.fileName = const Value.absent(),
    this.mime = const Value.absent(),
    this.bytes = const Value.absent(),
    this.uploadedId = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  PendingNoteAttachmentsCompanion.insert({
    required String placeholder,
    required String noteId,
    this.ownerUserId = const Value.absent(),
    required String fileName,
    required String mime,
    required Uint8List bytes,
    this.uploadedId = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : placeholder = Value(placeholder),
       noteId = Value(noteId),
       fileName = Value(fileName),
       mime = Value(mime),
       bytes = Value(bytes);
  static Insertable<PendingNoteAttachment> custom({
    Expression<String>? placeholder,
    Expression<String>? noteId,
    Expression<String>? ownerUserId,
    Expression<String>? fileName,
    Expression<String>? mime,
    Expression<Uint8List>? bytes,
    Expression<String>? uploadedId,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (placeholder != null) 'placeholder': placeholder,
      if (noteId != null) 'note_id': noteId,
      if (ownerUserId != null) 'owner_user_id': ownerUserId,
      if (fileName != null) 'file_name': fileName,
      if (mime != null) 'mime': mime,
      if (bytes != null) 'bytes': bytes,
      if (uploadedId != null) 'uploaded_id': uploadedId,
      if (rowid != null) 'rowid': rowid,
    });
  }

  PendingNoteAttachmentsCompanion copyWith({
    Value<String>? placeholder,
    Value<String>? noteId,
    Value<String?>? ownerUserId,
    Value<String>? fileName,
    Value<String>? mime,
    Value<Uint8List>? bytes,
    Value<String?>? uploadedId,
    Value<int>? rowid,
  }) {
    return PendingNoteAttachmentsCompanion(
      placeholder: placeholder ?? this.placeholder,
      noteId: noteId ?? this.noteId,
      ownerUserId: ownerUserId ?? this.ownerUserId,
      fileName: fileName ?? this.fileName,
      mime: mime ?? this.mime,
      bytes: bytes ?? this.bytes,
      uploadedId: uploadedId ?? this.uploadedId,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (placeholder.present) {
      map['placeholder'] = Variable<String>(placeholder.value);
    }
    if (noteId.present) {
      map['note_id'] = Variable<String>(noteId.value);
    }
    if (ownerUserId.present) {
      map['owner_user_id'] = Variable<String>(ownerUserId.value);
    }
    if (fileName.present) {
      map['file_name'] = Variable<String>(fileName.value);
    }
    if (mime.present) {
      map['mime'] = Variable<String>(mime.value);
    }
    if (bytes.present) {
      map['bytes'] = Variable<Uint8List>(bytes.value);
    }
    if (uploadedId.present) {
      map['uploaded_id'] = Variable<String>(uploadedId.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('PendingNoteAttachmentsCompanion(')
          ..write('placeholder: $placeholder, ')
          ..write('noteId: $noteId, ')
          ..write('ownerUserId: $ownerUserId, ')
          ..write('fileName: $fileName, ')
          ..write('mime: $mime, ')
          ..write('bytes: $bytes, ')
          ..write('uploadedId: $uploadedId, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

abstract class _$AppDatabase extends GeneratedDatabase {
  _$AppDatabase(QueryExecutor e) : super(e);
  $AppDatabaseManager get managers => $AppDatabaseManager(this);
  late final $LocalNotesTable localNotes = $LocalNotesTable(this);
  late final $LocalNotesFtsTable localNotesFts = $LocalNotesFtsTable(this);
  late final $NoteTombstonesTable noteTombstones = $NoteTombstonesTable(this);
  late final $PendingNoteAttachmentsTable pendingNoteAttachments =
      $PendingNoteAttachmentsTable(this);
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    localNotes,
    localNotesFts,
    noteTombstones,
    pendingNoteAttachments,
  ];
}

typedef $$LocalNotesTableCreateCompanionBuilder =
    LocalNotesCompanion Function({
      required String id,
      required String title,
      required String content,
      Value<String> visibility,
      required DateTime updatedAt,
      Value<int?> version,
      Value<String?> ownerUserId,
      Value<String?> createKey,
      Value<String?> createSnapshot,
      Value<bool> needsSync,
      Value<int> rowid,
    });
typedef $$LocalNotesTableUpdateCompanionBuilder =
    LocalNotesCompanion Function({
      Value<String> id,
      Value<String> title,
      Value<String> content,
      Value<String> visibility,
      Value<DateTime> updatedAt,
      Value<int?> version,
      Value<String?> ownerUserId,
      Value<String?> createKey,
      Value<String?> createSnapshot,
      Value<bool> needsSync,
      Value<int> rowid,
    });

class $$LocalNotesTableFilterComposer
    extends Composer<_$AppDatabase, $LocalNotesTable> {
  $$LocalNotesTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get content => $composableBuilder(
    column: $table.content,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get visibility => $composableBuilder(
    column: $table.visibility,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get version => $composableBuilder(
    column: $table.version,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get createKey => $composableBuilder(
    column: $table.createKey,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get createSnapshot => $composableBuilder(
    column: $table.createSnapshot,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<bool> get needsSync => $composableBuilder(
    column: $table.needsSync,
    builder: (column) => ColumnFilters(column),
  );
}

class $$LocalNotesTableOrderingComposer
    extends Composer<_$AppDatabase, $LocalNotesTable> {
  $$LocalNotesTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get content => $composableBuilder(
    column: $table.content,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get visibility => $composableBuilder(
    column: $table.visibility,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get version => $composableBuilder(
    column: $table.version,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get createKey => $composableBuilder(
    column: $table.createKey,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get createSnapshot => $composableBuilder(
    column: $table.createSnapshot,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<bool> get needsSync => $composableBuilder(
    column: $table.needsSync,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$LocalNotesTableAnnotationComposer
    extends Composer<_$AppDatabase, $LocalNotesTable> {
  $$LocalNotesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get content =>
      $composableBuilder(column: $table.content, builder: (column) => column);

  GeneratedColumn<String> get visibility => $composableBuilder(
    column: $table.visibility,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get updatedAt =>
      $composableBuilder(column: $table.updatedAt, builder: (column) => column);

  GeneratedColumn<int> get version =>
      $composableBuilder(column: $table.version, builder: (column) => column);

  GeneratedColumn<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get createKey =>
      $composableBuilder(column: $table.createKey, builder: (column) => column);

  GeneratedColumn<String> get createSnapshot => $composableBuilder(
    column: $table.createSnapshot,
    builder: (column) => column,
  );

  GeneratedColumn<bool> get needsSync =>
      $composableBuilder(column: $table.needsSync, builder: (column) => column);
}

class $$LocalNotesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $LocalNotesTable,
          LocalNote,
          $$LocalNotesTableFilterComposer,
          $$LocalNotesTableOrderingComposer,
          $$LocalNotesTableAnnotationComposer,
          $$LocalNotesTableCreateCompanionBuilder,
          $$LocalNotesTableUpdateCompanionBuilder,
          (
            LocalNote,
            BaseReferences<_$AppDatabase, $LocalNotesTable, LocalNote>,
          ),
          LocalNote,
          PrefetchHooks Function()
        > {
  $$LocalNotesTableTableManager(_$AppDatabase db, $LocalNotesTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$LocalNotesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$LocalNotesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$LocalNotesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> title = const Value.absent(),
                Value<String> content = const Value.absent(),
                Value<String> visibility = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
                Value<int?> version = const Value.absent(),
                Value<String?> ownerUserId = const Value.absent(),
                Value<String?> createKey = const Value.absent(),
                Value<String?> createSnapshot = const Value.absent(),
                Value<bool> needsSync = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => LocalNotesCompanion(
                id: id,
                title: title,
                content: content,
                visibility: visibility,
                updatedAt: updatedAt,
                version: version,
                ownerUserId: ownerUserId,
                createKey: createKey,
                createSnapshot: createSnapshot,
                needsSync: needsSync,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String title,
                required String content,
                Value<String> visibility = const Value.absent(),
                required DateTime updatedAt,
                Value<int?> version = const Value.absent(),
                Value<String?> ownerUserId = const Value.absent(),
                Value<String?> createKey = const Value.absent(),
                Value<String?> createSnapshot = const Value.absent(),
                Value<bool> needsSync = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => LocalNotesCompanion.insert(
                id: id,
                title: title,
                content: content,
                visibility: visibility,
                updatedAt: updatedAt,
                version: version,
                ownerUserId: ownerUserId,
                createKey: createKey,
                createSnapshot: createSnapshot,
                needsSync: needsSync,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$LocalNotesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $LocalNotesTable,
      LocalNote,
      $$LocalNotesTableFilterComposer,
      $$LocalNotesTableOrderingComposer,
      $$LocalNotesTableAnnotationComposer,
      $$LocalNotesTableCreateCompanionBuilder,
      $$LocalNotesTableUpdateCompanionBuilder,
      (LocalNote, BaseReferences<_$AppDatabase, $LocalNotesTable, LocalNote>),
      LocalNote,
      PrefetchHooks Function()
    >;
typedef $$LocalNotesFtsTableCreateCompanionBuilder =
    LocalNotesFtsCompanion Function({
      required String title,
      required String content,
      Value<int> rowid,
    });
typedef $$LocalNotesFtsTableUpdateCompanionBuilder =
    LocalNotesFtsCompanion Function({
      Value<String> title,
      Value<String> content,
      Value<int> rowid,
    });

class $$LocalNotesFtsTableFilterComposer
    extends Composer<_$AppDatabase, $LocalNotesFtsTable> {
  $$LocalNotesFtsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get content => $composableBuilder(
    column: $table.content,
    builder: (column) => ColumnFilters(column),
  );
}

class $$LocalNotesFtsTableOrderingComposer
    extends Composer<_$AppDatabase, $LocalNotesFtsTable> {
  $$LocalNotesFtsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get content => $composableBuilder(
    column: $table.content,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$LocalNotesFtsTableAnnotationComposer
    extends Composer<_$AppDatabase, $LocalNotesFtsTable> {
  $$LocalNotesFtsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get content =>
      $composableBuilder(column: $table.content, builder: (column) => column);
}

class $$LocalNotesFtsTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $LocalNotesFtsTable,
          LocalNoteFtsEntry,
          $$LocalNotesFtsTableFilterComposer,
          $$LocalNotesFtsTableOrderingComposer,
          $$LocalNotesFtsTableAnnotationComposer,
          $$LocalNotesFtsTableCreateCompanionBuilder,
          $$LocalNotesFtsTableUpdateCompanionBuilder,
          (
            LocalNoteFtsEntry,
            BaseReferences<
              _$AppDatabase,
              $LocalNotesFtsTable,
              LocalNoteFtsEntry
            >,
          ),
          LocalNoteFtsEntry,
          PrefetchHooks Function()
        > {
  $$LocalNotesFtsTableTableManager(_$AppDatabase db, $LocalNotesFtsTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$LocalNotesFtsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$LocalNotesFtsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$LocalNotesFtsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> title = const Value.absent(),
                Value<String> content = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => LocalNotesFtsCompanion(
                title: title,
                content: content,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String title,
                required String content,
                Value<int> rowid = const Value.absent(),
              }) => LocalNotesFtsCompanion.insert(
                title: title,
                content: content,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$LocalNotesFtsTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $LocalNotesFtsTable,
      LocalNoteFtsEntry,
      $$LocalNotesFtsTableFilterComposer,
      $$LocalNotesFtsTableOrderingComposer,
      $$LocalNotesFtsTableAnnotationComposer,
      $$LocalNotesFtsTableCreateCompanionBuilder,
      $$LocalNotesFtsTableUpdateCompanionBuilder,
      (
        LocalNoteFtsEntry,
        BaseReferences<_$AppDatabase, $LocalNotesFtsTable, LocalNoteFtsEntry>,
      ),
      LocalNoteFtsEntry,
      PrefetchHooks Function()
    >;
typedef $$NoteTombstonesTableCreateCompanionBuilder =
    NoteTombstonesCompanion Function({
      required String id,
      Value<String?> ownerUserId,
      required DateTime deletedAt,
      Value<bool> remotePending,
      Value<String?> lastError,
      Value<int> rowid,
    });
typedef $$NoteTombstonesTableUpdateCompanionBuilder =
    NoteTombstonesCompanion Function({
      Value<String> id,
      Value<String?> ownerUserId,
      Value<DateTime> deletedAt,
      Value<bool> remotePending,
      Value<String?> lastError,
      Value<int> rowid,
    });

class $$NoteTombstonesTableFilterComposer
    extends Composer<_$AppDatabase, $NoteTombstonesTable> {
  $$NoteTombstonesTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get deletedAt => $composableBuilder(
    column: $table.deletedAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<bool> get remotePending => $composableBuilder(
    column: $table.remotePending,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get lastError => $composableBuilder(
    column: $table.lastError,
    builder: (column) => ColumnFilters(column),
  );
}

class $$NoteTombstonesTableOrderingComposer
    extends Composer<_$AppDatabase, $NoteTombstonesTable> {
  $$NoteTombstonesTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get deletedAt => $composableBuilder(
    column: $table.deletedAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<bool> get remotePending => $composableBuilder(
    column: $table.remotePending,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get lastError => $composableBuilder(
    column: $table.lastError,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$NoteTombstonesTableAnnotationComposer
    extends Composer<_$AppDatabase, $NoteTombstonesTable> {
  $$NoteTombstonesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get deletedAt =>
      $composableBuilder(column: $table.deletedAt, builder: (column) => column);

  GeneratedColumn<bool> get remotePending => $composableBuilder(
    column: $table.remotePending,
    builder: (column) => column,
  );

  GeneratedColumn<String> get lastError =>
      $composableBuilder(column: $table.lastError, builder: (column) => column);
}

class $$NoteTombstonesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $NoteTombstonesTable,
          NoteTombstoneEntry,
          $$NoteTombstonesTableFilterComposer,
          $$NoteTombstonesTableOrderingComposer,
          $$NoteTombstonesTableAnnotationComposer,
          $$NoteTombstonesTableCreateCompanionBuilder,
          $$NoteTombstonesTableUpdateCompanionBuilder,
          (
            NoteTombstoneEntry,
            BaseReferences<
              _$AppDatabase,
              $NoteTombstonesTable,
              NoteTombstoneEntry
            >,
          ),
          NoteTombstoneEntry,
          PrefetchHooks Function()
        > {
  $$NoteTombstonesTableTableManager(
    _$AppDatabase db,
    $NoteTombstonesTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$NoteTombstonesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$NoteTombstonesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$NoteTombstonesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String?> ownerUserId = const Value.absent(),
                Value<DateTime> deletedAt = const Value.absent(),
                Value<bool> remotePending = const Value.absent(),
                Value<String?> lastError = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => NoteTombstonesCompanion(
                id: id,
                ownerUserId: ownerUserId,
                deletedAt: deletedAt,
                remotePending: remotePending,
                lastError: lastError,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                Value<String?> ownerUserId = const Value.absent(),
                required DateTime deletedAt,
                Value<bool> remotePending = const Value.absent(),
                Value<String?> lastError = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => NoteTombstonesCompanion.insert(
                id: id,
                ownerUserId: ownerUserId,
                deletedAt: deletedAt,
                remotePending: remotePending,
                lastError: lastError,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$NoteTombstonesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $NoteTombstonesTable,
      NoteTombstoneEntry,
      $$NoteTombstonesTableFilterComposer,
      $$NoteTombstonesTableOrderingComposer,
      $$NoteTombstonesTableAnnotationComposer,
      $$NoteTombstonesTableCreateCompanionBuilder,
      $$NoteTombstonesTableUpdateCompanionBuilder,
      (
        NoteTombstoneEntry,
        BaseReferences<_$AppDatabase, $NoteTombstonesTable, NoteTombstoneEntry>,
      ),
      NoteTombstoneEntry,
      PrefetchHooks Function()
    >;
typedef $$PendingNoteAttachmentsTableCreateCompanionBuilder =
    PendingNoteAttachmentsCompanion Function({
      required String placeholder,
      required String noteId,
      Value<String?> ownerUserId,
      required String fileName,
      required String mime,
      required Uint8List bytes,
      Value<String?> uploadedId,
      Value<int> rowid,
    });
typedef $$PendingNoteAttachmentsTableUpdateCompanionBuilder =
    PendingNoteAttachmentsCompanion Function({
      Value<String> placeholder,
      Value<String> noteId,
      Value<String?> ownerUserId,
      Value<String> fileName,
      Value<String> mime,
      Value<Uint8List> bytes,
      Value<String?> uploadedId,
      Value<int> rowid,
    });

class $$PendingNoteAttachmentsTableFilterComposer
    extends Composer<_$AppDatabase, $PendingNoteAttachmentsTable> {
  $$PendingNoteAttachmentsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get placeholder => $composableBuilder(
    column: $table.placeholder,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get noteId => $composableBuilder(
    column: $table.noteId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get fileName => $composableBuilder(
    column: $table.fileName,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mime => $composableBuilder(
    column: $table.mime,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<Uint8List> get bytes => $composableBuilder(
    column: $table.bytes,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get uploadedId => $composableBuilder(
    column: $table.uploadedId,
    builder: (column) => ColumnFilters(column),
  );
}

class $$PendingNoteAttachmentsTableOrderingComposer
    extends Composer<_$AppDatabase, $PendingNoteAttachmentsTable> {
  $$PendingNoteAttachmentsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get placeholder => $composableBuilder(
    column: $table.placeholder,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get noteId => $composableBuilder(
    column: $table.noteId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get fileName => $composableBuilder(
    column: $table.fileName,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mime => $composableBuilder(
    column: $table.mime,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<Uint8List> get bytes => $composableBuilder(
    column: $table.bytes,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get uploadedId => $composableBuilder(
    column: $table.uploadedId,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$PendingNoteAttachmentsTableAnnotationComposer
    extends Composer<_$AppDatabase, $PendingNoteAttachmentsTable> {
  $$PendingNoteAttachmentsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get placeholder => $composableBuilder(
    column: $table.placeholder,
    builder: (column) => column,
  );

  GeneratedColumn<String> get noteId =>
      $composableBuilder(column: $table.noteId, builder: (column) => column);

  GeneratedColumn<String> get ownerUserId => $composableBuilder(
    column: $table.ownerUserId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get fileName =>
      $composableBuilder(column: $table.fileName, builder: (column) => column);

  GeneratedColumn<String> get mime =>
      $composableBuilder(column: $table.mime, builder: (column) => column);

  GeneratedColumn<Uint8List> get bytes =>
      $composableBuilder(column: $table.bytes, builder: (column) => column);

  GeneratedColumn<String> get uploadedId => $composableBuilder(
    column: $table.uploadedId,
    builder: (column) => column,
  );
}

class $$PendingNoteAttachmentsTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $PendingNoteAttachmentsTable,
          PendingNoteAttachment,
          $$PendingNoteAttachmentsTableFilterComposer,
          $$PendingNoteAttachmentsTableOrderingComposer,
          $$PendingNoteAttachmentsTableAnnotationComposer,
          $$PendingNoteAttachmentsTableCreateCompanionBuilder,
          $$PendingNoteAttachmentsTableUpdateCompanionBuilder,
          (
            PendingNoteAttachment,
            BaseReferences<
              _$AppDatabase,
              $PendingNoteAttachmentsTable,
              PendingNoteAttachment
            >,
          ),
          PendingNoteAttachment,
          PrefetchHooks Function()
        > {
  $$PendingNoteAttachmentsTableTableManager(
    _$AppDatabase db,
    $PendingNoteAttachmentsTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$PendingNoteAttachmentsTableFilterComposer(
                $db: db,
                $table: table,
              ),
          createOrderingComposer: () =>
              $$PendingNoteAttachmentsTableOrderingComposer(
                $db: db,
                $table: table,
              ),
          createComputedFieldComposer: () =>
              $$PendingNoteAttachmentsTableAnnotationComposer(
                $db: db,
                $table: table,
              ),
          updateCompanionCallback:
              ({
                Value<String> placeholder = const Value.absent(),
                Value<String> noteId = const Value.absent(),
                Value<String?> ownerUserId = const Value.absent(),
                Value<String> fileName = const Value.absent(),
                Value<String> mime = const Value.absent(),
                Value<Uint8List> bytes = const Value.absent(),
                Value<String?> uploadedId = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => PendingNoteAttachmentsCompanion(
                placeholder: placeholder,
                noteId: noteId,
                ownerUserId: ownerUserId,
                fileName: fileName,
                mime: mime,
                bytes: bytes,
                uploadedId: uploadedId,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String placeholder,
                required String noteId,
                Value<String?> ownerUserId = const Value.absent(),
                required String fileName,
                required String mime,
                required Uint8List bytes,
                Value<String?> uploadedId = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => PendingNoteAttachmentsCompanion.insert(
                placeholder: placeholder,
                noteId: noteId,
                ownerUserId: ownerUserId,
                fileName: fileName,
                mime: mime,
                bytes: bytes,
                uploadedId: uploadedId,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$PendingNoteAttachmentsTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $PendingNoteAttachmentsTable,
      PendingNoteAttachment,
      $$PendingNoteAttachmentsTableFilterComposer,
      $$PendingNoteAttachmentsTableOrderingComposer,
      $$PendingNoteAttachmentsTableAnnotationComposer,
      $$PendingNoteAttachmentsTableCreateCompanionBuilder,
      $$PendingNoteAttachmentsTableUpdateCompanionBuilder,
      (
        PendingNoteAttachment,
        BaseReferences<
          _$AppDatabase,
          $PendingNoteAttachmentsTable,
          PendingNoteAttachment
        >,
      ),
      PendingNoteAttachment,
      PrefetchHooks Function()
    >;

class $AppDatabaseManager {
  final _$AppDatabase _db;
  $AppDatabaseManager(this._db);
  $$LocalNotesTableTableManager get localNotes =>
      $$LocalNotesTableTableManager(_db, _db.localNotes);
  $$LocalNotesFtsTableTableManager get localNotesFts =>
      $$LocalNotesFtsTableTableManager(_db, _db.localNotesFts);
  $$NoteTombstonesTableTableManager get noteTombstones =>
      $$NoteTombstonesTableTableManager(_db, _db.noteTombstones);
  $$PendingNoteAttachmentsTableTableManager get pendingNoteAttachments =>
      $$PendingNoteAttachmentsTableTableManager(
        _db,
        _db.pendingNoteAttachments,
      );
}
