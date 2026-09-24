# Front Flutter - build web y sirve con nginx
# Etapa 1: build web con Flutter
FROM ghcr.io/cirruslabs/flutter:stable AS builder
ARG BASE_URL=http://localhost:8085
ARG NOTES_BASE_URL=http://localhost:8082
WORKDIR /app
COPY pubspec.yaml pubspec.lock ./
RUN flutter pub get
COPY . .
# Asegura wasm assets para drift web (si no están en web/, los recupera del pub-cache)
RUN if [ ! -f web/sqlite3.wasm ]; then \
      echo "sqlite3.wasm no encontrado en web/, intentando recuperar..."; \
      find /root -name sqlite3.wasm -path "*drift*build*sqlite3.wasm" -exec cp {} web/sqlite3.wasm \; 2>/dev/null || \
      find /home -name sqlite3.wasm -path "*drift*build*sqlite3.wasm" -exec cp {} web/sqlite3.wasm \; 2>/dev/null || true; \
      ls -lh web/sqlite3.wasm || echo "ADVERTENCIA: sqlite3.wasm no disponible, build puede fallar en runtime"; \
    fi
RUN if [ ! -f web/drift_worker.js ]; then \
      find /root -name drift_worker.js -path "*drift*drift_worker.js" -exec cp {} web/drift_worker.js \; 2>/dev/null || \
      find /home -name drift_worker.js -path "*drift*drift_worker.js" -exec cp {} web/drift_worker.js \; 2>/dev/null || true; \
      ls -lh web/drift_worker.js || echo "drift_worker.js opcional para WasmDatabase.open"; \
    fi
# BASE_URL = auth, NOTES_BASE_URL = notes (ver front/lib/core/services/api_config.dart).
# Un valor que empieza con "/" (ej. /api-auth) es relativo al origen: usa el proxy de nginx.conf.
RUN flutter build web --release \
    --dart-define=API_BASE_URL=$BASE_URL \
    --dart-define=NOTES_BASE_URL=$NOTES_BASE_URL

# Etapa 2: nginx para servir
FROM nginx:alpine
COPY --from=builder /app/build/web /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
