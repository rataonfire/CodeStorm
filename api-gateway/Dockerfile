# Многостадийная сборка для маленького образа
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o api-gateway .

# Финальный образ
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/api-gateway .

EXPOSE 8000

CMD ["./api-gateway"]