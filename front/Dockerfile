# Front Flutter - build web y sirve con nginx
# Etapa 1: build web con Flutter
FROM ghcr.io/cirruslabs/flutter:stable AS builder
ARG BASE_URL=http://localhost:8085
ARG NOTES_BASE_URL=http://localhost:8082
ARG SOCIAL_BASE_URL=http://localhost:8083
ARG GOOGLE_CLIENT_ID=
ARG GOOGLE_REDIRECT_URI=http://localhost:8081/auth/google/callback
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
# Local por defecto http://localhost:8085; para dominio usar --dart-define o .env.prod (ver docker-compose.yml y front/.env.prod.example).
RUN flutter build web --release \
    --dart-define=API_BASE_URL=$BASE_URL \
    --dart-define=NOTES_BASE_URL=$NOTES_BASE_URL \
    --dart-define=SOCIAL_BASE_URL=$SOCIAL_BASE_URL \
    --dart-define=GOOGLE_CLIENT_ID=$GOOGLE_CLIENT_ID \
    --dart-define=GOOGLE_REDIRECT_URI=$GOOGLE_REDIRECT_URI

# Etapa 2: nginx para servir
FROM nginx:alpine
COPY --from=builder /app/build/web /usr/share/nginx/html
# El entrypoint de nginx:alpine pasa templates/*.template por envsubst hacia conf.d/.
# NGINX_ENTRYPOINT_LOCAL_RESOLVERS llena NGINX_LOCAL_RESOLVERS desde /etc/resolv.conf.
ENV NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1 UPSTREAM_SUFFIX=""
COPY nginx.conf /etc/nginx/templates/default.conf.template
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
