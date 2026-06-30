# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/auth ./cmd/

# Stage 2: Runtime
FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app && \
    apk add --no-cache ca-certificates tzdata openssl
WORKDIR /app
COPY --from=builder /bin/auth /app/auth
COPY --from=builder /app/migrations /app/migrations
COPY entrypoint.sh /entrypoint.sh
RUN mkdir -p /app/keys && chown app:app /app/keys && chmod +x /entrypoint.sh
USER app
EXPOSE 8080 8081
ENTRYPOINT ["/entrypoint.sh"]
