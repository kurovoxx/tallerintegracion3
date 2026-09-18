# Auth microservicio - back/auth
FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /app
# deps
COPY go.mod go.sum ./
RUN go mod download
# código
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-server ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /auth-server .
EXPOSE 8080
ENV PORT=8080
CMD ["./auth-server"]
