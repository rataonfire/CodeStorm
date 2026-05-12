FROM golang:1.22-alpine AS builder
ENV GOPROXY=https://proxy.golang.org,direct
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o emulator-bank .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/emulator-bank .
CMD ["./emulator-bank"]
