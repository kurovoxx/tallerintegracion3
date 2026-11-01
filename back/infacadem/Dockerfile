# Academic microservicio - back/infacadem
FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /app
# deps
COPY go.mod go.sum ./
RUN go mod download
# código
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /academic-server ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /academic-server .
EXPOSE 8081 9081
ENV PORT=8081
CMD ["./academic-server"]
