# Front Flutter - build web y sirve con nginx
# Etapa 1: build web con Flutter
FROM ghcr.io/cirruslabs/flutter:stable AS builder
ARG BASE_URL=http://localhost:8080
WORKDIR /app
COPY pubspec.yaml pubspec.lock ./
RUN flutter pub get
COPY . .
# Inyecta BASE_URL en auth_service.dart si existe (opcional, usa 8080 para docker)
RUN sed -i "s|http://localhost:8082|$BASE_URL|g" lib/core/services/auth_service.dart || true
RUN sed -i "s|http://localhost:8080|$BASE_URL|g" lib/core/services/auth_service.dart || true
RUN flutter build web --release

# Etapa 2: nginx para servir
FROM nginx:alpine
COPY --from=builder /app/build/web /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
