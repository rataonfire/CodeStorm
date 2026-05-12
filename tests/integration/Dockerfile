FROM golang:1.22-alpine
WORKDIR /tests
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["go", "test", "-v", "-timeout", "120s", "./..."]
