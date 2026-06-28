# Stage 1: Build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /shortener ./cmd/main.go

# Stage 2: Run
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /shortener /usr/local/bin/shortener

EXPOSE 9091 8082

ENTRYPOINT ["shortener"]
