FROM golang:1.22-alpine
WORKDIR /tests
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Tests are run with -timeout to handle missing-source 2s window + buffer
CMD ["go", "test", "-v", "-timeout", "120s", "./..."]
