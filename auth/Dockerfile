# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/auth ./cmd/

# Stage 2: Distroless runtime
FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=builder /bin/auth /app/auth
COPY --from=builder /app/migrations /app/migrations
USER app
EXPOSE 8080 8081
ENTRYPOINT ["/app/auth"]
