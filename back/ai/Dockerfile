# IA microservicio - back/ai
FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /app
# deps
COPY go.mod go.sum ./
RUN go mod download
# código
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ai-server ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /ai-server .
EXPOSE 8084
ENV PORT=8084
CMD ["./ai-server"]
