// ignore_for_file: depend_on_referenced_packages

import 'dart:async';

import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_platform_interface.dart';
import 'package:shared_preferences_platform_interface/types.dart';

class ControlledPreferencesStore extends SharedPreferencesStorePlatform {
  final Map<String, Object> persisted = {};
  int tokenPairWriteAttempts = 0;
  int tokenPairFailuresRemaining = 0;
  bool throwOnTokenPairWrite = false;
  bool holdSessionClear = false;
  final clearStarted = Completer<void>();
  final releaseClear = Completer<void>();

  void install() {
    SharedPreferencesStorePlatform.instance = this;
    SharedPreferences.resetStatic();
  }

  @override
  Future<bool> setValue(String valueType, String key, Object value) async {
    if (key == 'flutter.auth_tokens_v1') {
      tokenPairWriteAttempts++;
      if (throwOnTokenPairWrite) {
        throw StateError('Simulated token-pair storage failure');
      }
      if (tokenPairFailuresRemaining > 0) {
        tokenPairFailuresRemaining--;
        return false;
      }
    }
    persisted[key] = value;
    return true;
  }

  @override
  Future<bool> remove(String key) async {
    if (holdSessionClear && key == 'flutter.auth_remember_me') {
      if (!clearStarted.isCompleted) clearStarted.complete();
      await releaseClear.future;
    }
    persisted.remove(key);
    return true;
  }

  @override
  Future<bool> clear() => clearWithParameters(
    ClearParameters(filter: PreferencesFilter(prefix: 'flutter.')),
  );

  @override
  Future<bool> clearWithParameters(ClearParameters parameters) async {
    final filter = parameters.filter;
    persisted.removeWhere((key, _) {
      return key.startsWith(filter.prefix) &&
          (filter.allowList == null || filter.allowList!.contains(key));
    });
    return true;
  }

  @override
  Future<Map<String, Object>> getAll() => getAllWithParameters(
    GetAllParameters(filter: PreferencesFilter(prefix: 'flutter.')),
  );

  @override
  Future<Map<String, Object>> getAllWithParameters(
    GetAllParameters parameters,
  ) async {
    final filter = parameters.filter;
    return Map<String, Object>.fromEntries(
      persisted.entries.where((entry) {
        return entry.key.startsWith(filter.prefix) &&
            (filter.allowList == null || filter.allowList!.contains(entry.key));
      }),
    );
  }
}
