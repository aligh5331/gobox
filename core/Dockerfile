# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/core ./cmd/

# Stage 2: Runtime
FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=builder /bin/core /app/core
USER app
ENTRYPOINT ["/app/core"]
