# Stage 1: Build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /fileupload ./cmd/main.go

# Stage 2: Run
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /fileupload /usr/local/bin/fileupload

EXPOSE 9090

ENTRYPOINT ["fileupload"]
