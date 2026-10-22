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
          ..write('createSnapshot: $createSnapshot')
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
          other.createSnapshot == this.createSnapshot);
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

abstract class _$AppDatabase extends GeneratedDatabase {
  _$AppDatabase(QueryExecutor e) : super(e);
  $AppDatabaseManager get managers => $AppDatabaseManager(this);
  late final $LocalNotesTable localNotes = $LocalNotesTable(this);
  late final $LocalNotesFtsTable localNotesFts = $LocalNotesFtsTable(this);
  late final $NoteTombstonesTable noteTombstones = $NoteTombstonesTable(this);
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    localNotes,
    localNotesFts,
    noteTombstones,
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

class $AppDatabaseManager {
  final _$AppDatabase _db;
  $AppDatabaseManager(this._db);
  $$LocalNotesTableTableManager get localNotes =>
      $$LocalNotesTableTableManager(_db, _db.localNotes);
  $$LocalNotesFtsTableTableManager get localNotesFts =>
      $$LocalNotesFtsTableTableManager(_db, _db.localNotesFts);
  $$NoteTombstonesTableTableManager get noteTombstones =>
      $$NoteTombstonesTableTableManager(_db, _db.noteTombstones);
}
