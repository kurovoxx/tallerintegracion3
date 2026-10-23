/// Ruta que recibe el microservicio después de que nginx quita su prefijo.
/// Los mocks de contrato deben funcionar tanto con las bases nativas por defecto
/// (/api-notes, etc.) como con puertos locales definidos en --dart-define.
extension ServiceUri on Uri {
  String get servicePath {
    const prefixes = <String>{
      '/api-auth',
      '/api-notes',
      '/api-social',
      '/api-academic',
    };
    for (final prefix in prefixes) {
      if (path.startsWith('$prefix/')) return path.substring(prefix.length);
    }
    return path;
  }
}
