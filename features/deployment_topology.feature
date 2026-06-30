# SPEC: Deployment Topology — Network Segmentation, Reverse Proxy, and Service Discovery
# BUDGET: medium 5-10K
# SCOPE: docker-compose.yml (root), auth/docker-compose.yml, fileupload/docker-compose.yml,
#        shortener/docker-compose.yml, thumbgen/docker-compose.yml,
#        core/.env.example, auth/.env.example, fileupload/.env.example, shortener/.env.example,
#        core/Dockerfile, auth/Dockerfile, fileupload/Dockerfile, shortener/Dockerfile,
#        core/pkg/config/config.go, shortener/pkg/config/config.go,
#        core/internal/infrastructure/grpcclient/thumbgen/stub.go,
#        all *.go source files (for hardcoded-address grep)
# STATUS: draft

Feature: Deployment Topology
  As an operator of GoBox
  I want the Docker Compose network topology, reverse-proxy routing, and
    service-discovery mechanism to follow a strict, auditable contract
  So that lateral movement is prevented, public ingress is controlled, and
    cross-service addresses are never hardcoded.

  Background:
    Given the root compose file at docker-compose.yml (project root) defines
      these internal networks per ADR-001:
      | Network | Members |
      |---------|---------|
      | net_edge | nginx-proxy-manager, core, shortener |
      | net_core_auth | core, auth |
      | net_core_fileupload | core, fileupload |
      | net_core_shortener | core, shortener |
      | net_shortener_fileupload | shortener, fileupload |
      | net_auth_data | auth, auth-postgres |
      | net_fileupload_data | fileupload, fileupload-postgres, minio |
      | net_shortener_data | shortener, shortener-postgres, shortener-redis |
    And the network net_core_thumbgen (core, thumbgen) is documented in
      ADR-001 but does NOT exist in any docker-compose.yml because ThumbGen
      is built in Phase 5
    And every declared network except net_edge is marked internal: true
    And net_edge is marked internal: false (NPM needs internet access for
      ACME / Let's Encrypt)

  # ---------------------------------------------------------------------------
  # NETWORK ISOLATION — DATA STORES
  # ---------------------------------------------------------------------------

  Scenario: fileupload-postgres is isolated from core, auth, and shortener
    Given the compose stack is running with all networks from the Background
    When the fileupload container executes
      nc -z -w3 fileupload-postgres 5432
    Then the exit code is 0 — fileupload shares net_fileupload_data with
      fileupload-postgres, so DNS resolves and the TCP handshake succeeds
    When the core container executes
      nc -z -w3 fileupload-postgres 5432
    Then the exit code is non-zero and the error is a DNS resolution failure
      (stderr contains "Name does not resolve" or "bad address") — NOT
      "Connection refused" and NOT "timed out".  DNS fails immediately
      because core does not share net_fileupload_data with
      fileupload-postgres
    When the auth container executes
      nc -z -w3 fileupload-postgres 5432
    Then the exit code is non-zero and the error is a DNS resolution failure
      — auth does not share net_fileupload_data
    When the shortener container executes
      nc -z -w3 fileupload-postgres 5432
    Then the exit code is non-zero and the error is a DNS resolution failure
      — shortener does not share net_fileupload_data

  Scenario: auth-postgres is isolated from core, fileupload, and shortener
    Given the compose stack is running with all networks from the Background
    When the auth container executes
      nc -z -w3 auth-postgres 5432
    Then the exit code is 0 — auth shares net_auth_data with auth-postgres
    When the core container executes
      nc -z -w3 auth-postgres 5432
    Then the exit code is non-zero and the error is a DNS resolution failure
      — core does not share net_auth_data
    When the fileupload container executes
      nc -z -w3 auth-postgres 5432
    Then the exit code is non-zero and the error is a DNS resolution failure
      — fileupload does not share net_auth_data
    When the shortener container executes
      nc -z -w3 auth-postgres 5432
    Then the exit code is non-zero and the error is a DNS resolution failure
      — shortener does not share net_auth_data

  Scenario: shortener-redis is isolated from core, auth, and fileupload
    Given the compose stack is running with all networks from the Background
    When the shortener container executes
      nc -z -w3 shortener-redis 6379
    Then the exit code is 0 — shortener shares net_shortener_data with
      shortener-redis
    When the core container executes
      nc -z -w3 shortener-redis 6379
    Then the exit code is non-zero and the error is a DNS resolution failure
      — core does not share net_shortener_data
    When the auth container executes
      nc -z -w3 shortener-redis 6379
    Then the exit code is non-zero and the error is a DNS resolution failure
      — auth does not share net_shortener_data
    When the fileupload container executes
      nc -z -w3 shortener-redis 6379
    Then the exit code is non-zero and the error is a DNS resolution failure
      — fileupload does not share net_shortener_data

  # ---------------------------------------------------------------------------
  # G RPC REACHABILITY — POSITIVE
  # ---------------------------------------------------------------------------

  Scenario: core can reach auth, fileupload, and shortener gRPC
    Given the compose stack is running with all networks from the Background
    When the core container executes
      grpcurl -plaintext -connect-timeout 3 auth:8081 list
    Then the exit code is 0 — core shares net_core_auth with auth; auth's
      gRPC server is listening on port 8081 (AUTH_GRPC_PORT)
    When the core container executes
      grpcurl -plaintext -connect-timeout 3 fileupload:9090 list
    Then the exit code is 0 — core shares net_core_fileupload with fileupload;
      fileupload's gRPC server is listening on port 9090
      (FILEUPLOAD_GRPC_PORT)
    When the core container executes
      grpcurl -plaintext -connect-timeout 3 shortener:9091 list
    Then the exit code is 0 — core shares net_core_shortener with shortener;
      shortener's gRPC server is listening on port 9091
      (SHORTENER_GRPC_PORT)

  Scenario: shortener can reach fileupload gRPC
    Given the compose stack is running with all networks from the Background
    When the shortener container executes
      grpcurl -plaintext -connect-timeout 3 fileupload:9090 list
    Then the exit code is 0 — shortener shares net_shortener_fileupload with
      fileupload; fileupload's gRPC server is listening on port 9090
      (FILEUPLOAD_GRPC_PORT)

  # ---------------------------------------------------------------------------
  # G RPC NON-REACHABILITY — FILEUPLOAD HAS NO OUTBOUND DEPS
  # ---------------------------------------------------------------------------

  Scenario: fileupload cannot reach shortener or auth
    # FILEUPLOAD → AUTH: no shared network exists at all.
    Given the compose stack is running with all networks from the Background
    When the fileupload container executes
      nc -z -w3 auth 8081
    Then the exit code is non-zero and the error is a DNS resolution failure
      — fileupload shares no network with auth (fileupload is on
      net_core_fileupload, net_shortener_fileupload, net_fileupload_data;
      auth is on net_core_auth, net_auth_data — zero overlap).

    # FILEUPLOAD → SHORTENER: a shared network (net_shortener_fileupload)
    # exists, so DNS resolution succeeds.  The constraint is at the code
    # level: fileupload must never construct a shortener gRPC client.
    When a recursive grep is run over the fileupload/ source tree for:
      - any import of github.com/aligh5331/gobox-proto/gen/shortener
      - any call to grpc.Dial or grpc.NewClient whose argument matches
        the FILEUPLOAD_GRPC_ADDR pattern but targets shortener
    Then zero matches are found — fileupload has zero outbound service
      dependencies per GOBOX_SPEC.md §5.3

  # ---------------------------------------------------------------------------
  # THUMBGEN ABSENCE — CORE OPERATES WITHOUT THUMBGEN
  # ---------------------------------------------------------------------------

  Scenario: core starts and serves all non-thumbnail routes without thumbgen
    Given the compose stack is started with NO thumbgen container defined
      and NO net_core_thumbgen network defined
    And core's environment includes THUMBGEN_GRPC_ADDR=thumbgen:9092 (the
      env var is set but the target is unreachable)
    Then core's health endpoint responds 200:
      docker compose exec core wget -qO- http://localhost:8080/health
      returns 200 and body contains "ok" — the nil-guarded thumbgen client
      stub at core/internal/infrastructure/grpcclient/thumbgen/stub.go
      allows core to compile and boot without a running thumbgen binary
    When core receives a POST /api/v1/auth/register request
    Then the handler returns a non-500 response and the response body does
      NOT contain "thumbgen" or "thumbnail" — non-thumbnail operations
      never touch the thumbgen client
    When core receives a POST /api/v1/files/{id}/thumbnail request (the
      route that calls ThumbGenClient.EnqueueJob)
    Then the handler returns successfully without a 500 error — the stub
      returns a canned JOB_STATUS_QUEUED response and core continues
      serving other requests normally

  # ---------------------------------------------------------------------------
  # PUBLIC EDGE — ONLY CORE AND SHORTENER ON NET_EDGE
  # ---------------------------------------------------------------------------

  Scenario: only core's HTTP port and shortener's redirect HTTP port are
            reachable from net_edge; auth's HTTP port is not publicly routed
    Given the root compose file docker-compose.yml (project root) is the
      single source of truth for network membership
    When I inspect the networks: list for the core service
    Then core is listed under net_edge with its HTTP port (8080)
    When I inspect the networks: list for the shortener service
    Then shortener is listed under net_edge with its redirect HTTP port
      (8082)
    When I inspect the networks: list for the auth service
    Then auth is NOT listed under net_edge — auth joins only
      net_core_auth and net_auth_data; its HTTP port (8080) is reachable
      only from core on net_core_auth for JWKS fetching, never from the
      public internet
    When I inspect the networks: list for nginx-proxy-manager
    Then nginx-proxy-manager is listed under net_edge with published host
      ports 80, 443, and 81
    And NPM's proxy hosts (configured via admin UI, not checked
      automatically) must be:
      | Subdomain       | Target    | Port |
      | api.{{domain}}  | core      | 8080 |
      | s.{{domain}}    | shortener | 8082 |
    And there must be NO proxy host for auth.{{domain}}
    And there must be NO proxy host for any other GoBox service — in v1,
      only core and shortener publish public HTTP ports

  # ---------------------------------------------------------------------------
  # ENV VAR COMPLIANCE — NO HARDCODED ADDRESSES
  # ---------------------------------------------------------------------------

  Scenario: no Go source file or Dockerfile contains a hardcoded internal
            container hostname
    Given the source trees at auth/, core/, fileupload/, shortener/,
      and thumbgen/ (if present)
    When I run a recursive grep for the pattern
      "(auth|core|fileupload|shortener|thumbgen|auth-postgres|
        fileupload-postgres|shortener-postgres|shortener-redis|minio|
        nginx-proxy-manager):[0-9]+"
      across all *.go files and all Dockerfiles
    Then zero matches are found — internal Docker container hostnames must
      never appear as string literals in source code or Dockerfiles
    And the only files in the repository where container hostname patterns
      are allowed are docker-compose.yml and .env.example

  # ---------------------------------------------------------------------------
  # ENV VAR COMPLIANCE — .ENV.EXAMPLE COVERAGE
  # ---------------------------------------------------------------------------

  Scenario: every cross-service address listed in ADR-003 exists in the
            corresponding service's .env.example
    Given the cross-service address variable catalogue from ADR-003:
      | Service    | Variables required                                       |
      |------------|----------------------------------------------------------|
      | core       | AUTH_GRPC_ADDR, AUTH_HTTP_ADDR, FILEUPLOAD_GRPC_ADDR,   |
      |            | SHORTENER_GRPC_ADDR, THUMBGEN_GRPC_ADDR                  |
      | shortener  | FILEUPLOAD_GRPC_ADDR                                     |
      | auth       | (none — zero outbound deps)                              |
      | fileupload | (none — zero outbound deps)                              |
    When I read core/.env.example
    Then every variable name from the core row appears as a key
      (the text before "=" on a non-comment line)
    When I read shortener/.env.example
    Then FILEUPLOAD_GRPC_ADDR appears as a key
    When I read auth/.env.example
    Then none of the *_GRPC_ADDR or *_HTTP_ADDR variable names appear
      — auth has zero outbound service dependencies
    When I read fileupload/.env.example
    Then none of the *_GRPC_ADDR or *_HTTP_ADDR variable names appear
      — fileupload has zero outbound service dependencies
    # DUAL CONVENTION NOTE: ADR-003 specifies that .env.example values use
    # localhost:{host-mapped-port} for native development, while
    # docker-compose.yml uses {container-name}:{container-port} for Docker.
    # The Builder must ensure the values follow this convention even though
    # the automated check above only verifies variable name presence.
