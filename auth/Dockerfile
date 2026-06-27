# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Copy gobox-proto from named build context (see docker-compose additional_contexts)
COPY --from=goboxproto . /gobox-proto

COPY go.mod go.sum ./
# Replace private module with local copy inside the build container
RUN go mod edit -replace git.0lab.ir/aligh/gobox-proto=/gobox-proto && \
    go mod download

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
