#!/bin/sh
set -e

# Auto-generate RSA key pair if the private key does not exist.
# This enables zero-friction local development — no need to run openssl manually.
# For production, mount keys via a volume or secret at JWT_PRIVATE_KEY_PATH.

KEY_PATH="${JWT_PRIVATE_KEY_PATH:-/app/keys/private.pem}"

if [ ! -f "$KEY_PATH" ]; then
    KEY_DIR=$(dirname "$KEY_PATH")
    echo "entrypoint: generating RSA key pair at $KEY_PATH"
    mkdir -p "$KEY_DIR"
    openssl genrsa -out "$KEY_PATH" 2048
    openssl rsa -in "$KEY_PATH" -pubout -out "${KEY_DIR}/public.pem"
    echo "entrypoint: RSA key pair generated"
fi

exec /app/auth "$@"
