# SPEC: Link Shortener
# BUDGET: medium 5-10K
# SCOPE: shortener/
# STATUS: draft

@shortener
Feature: Link Shortener

  The Shortener service creates short, shareable links to files stored in
  FileUpload. It exposes a gRPC API on port 9091 (JWT-authenticated, for Core
  API) and an HTTP endpoint on port 8082 (public, for anonymous redirects).

  Background:
    Given the Shortener service is running with Postgres and Redis connected
    And the gRPC server is listening on port 9091
    And the HTTP server is listening on port 8082

  # ──────────────────────────────────────────────
  # CreateLink — gRPC, authenticated
  # ──────────────────────────────────────────────

  @grpc @create-link
  Scenario: CreateLink happy path returns slug and short URL
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    And a file exists with id "660e8400-e29b-41d4-a716-446655440001" and status "ready"
    When the user sends a CreateLink request with file_id "660e8400-e29b-41d4-a716-446655440001"
    Then the response contains a slug of exactly 6 alphanumeric characters
    And the response contains a short_url that includes the slug
    And the response contains the file_id "660e8400-e29b-41d4-a716-446655440001"

  @grpc @create-link @validation
  Scenario: CreateLink with missing file_id returns validation error
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    When the user sends a CreateLink request with an empty file_id
    Then the response status code is INVALID_ARGUMENT
    And the error message describes the missing file_id field

  @grpc @create-link @collision
  Scenario: CreateLink duplicate slug collision is resolved by retry
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    And a file exists with id "660e8400-e29b-41d4-a716-446655440001" and status "ready"
    And the slug generator will produce colliding slugs on the first 3 attempts
    When the user sends a CreateLink request with file_id "660e8400-e29b-41d4-a716-446655440001"
    Then the response contains a slug of exactly 6 alphanumeric characters
    And the slug is different from any of the known colliding slugs

  # ──────────────────────────────────────────────
  # GetLink — gRPC, authenticated
  # ──────────────────────────────────────────────

  @grpc @get-link
  Scenario: GetLink found returns ShortLinkResponse
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    And a ShortLink exists with id "770e8400-e29b-41d4-a716-446655440002"
    And the ShortLink belongs to the authenticated user
    When the user sends a GetLink request with link_id "770e8400-e29b-41d4-a716-446655440002"
    Then the response contains a ShortLinkResponse with id "770e8400-e29b-41d4-a716-446655440002"
    And the response contains the slug, file_id, and created_at fields

  @grpc @get-link @not-found
  Scenario: GetLink with unknown link_id returns not found
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    And no ShortLink exists with id "99999999-9999-4999-9999-999999999999"
    When the user sends a GetLink request with link_id "99999999-9999-4999-9999-999999999999"
    Then the response status code is NOT_FOUND
    And the error message indicates the link was not found

  # ──────────────────────────────────────────────
  # DeleteLink — gRPC, authenticated
  # ──────────────────────────────────────────────

  @grpc @delete-link
  Scenario: DeleteLink removes the user's own link
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    And a ShortLink exists with id "770e8400-e29b-41d4-a716-446655440002"
    And the ShortLink belongs to the authenticated user
    When the user sends a DeleteLink request with link_id "770e8400-e29b-41d4-a716-446655440002"
    Then the response confirms successful deletion
    And a subsequent GetLink request for "770e8400-e29b-41d4-a716-446655440002" returns NOT_FOUND

  @grpc @delete-link @permission
  Scenario: DeleteLink on another user's link returns permission denied
    Given an authenticated user "Alice" with id "550e8400-e29b-41d4-a716-446655440000"
    And an authenticated user "Bob" with id "660e8400-e29b-41d4-a716-446655440001"
    And a ShortLink exists with id "770e8400-e29b-41d4-a716-446655440002" owned by Bob
    When Alice sends a DeleteLink request with link_id "770e8400-e29b-41d4-a716-446655440002"
    Then the response status code is PERMISSION_DENIED
    And the error message indicates the link does not belong to the requester

  # ──────────────────────────────────────────────
  # ListLinks — gRPC, authenticated
  # ──────────────────────────────────────────────

  @grpc @list-links @pagination
  Scenario: ListLinks returns paginated results with pagination metadata
    Given an authenticated user with id "550e8400-e29b-41d4-a716-446655440000"
    And the user owns 25 ShortLinks
    When the user sends a ListLinks request with page=1 and page_size=10
    Then the response contains exactly 10 links
    And the response contains pagination metadata with total_count=25
    And the response contains pagination metadata with next_page=2

  @grpc @list-links @filter
  Scenario: ListLinks filtered by owner returns only that owner's links
    Given an authenticated user "Alice" with id "550e8400-e29b-41d4-a716-446655440000"
    And an authenticated user "Bob" with id "660e8400-e29b-41d4-a716-446655440001"
    And Alice owns 3 ShortLinks
    And Bob owns 5 ShortLinks
    When Alice sends a ListLinks request with owner_filter "550e8400-e29b-41d4-a716-446655440000"
    Then the response contains exactly 3 links
    And every link in the response has owner "550e8400-e29b-41d4-a716-446655440000"

  # ──────────────────────────────────────────────
  # Redirect — HTTP, public (no auth)
  # ──────────────────────────────────────────────

  @http @redirect @cache-hit
  Scenario: Redirect cache hit returns 302 with presigned URL
    Given a ShortLink exists with slug "aB3kR9" and no expiry
    And Redis contains the mapping "slug:aB3kR9" -> file_id "660e8400-e29b-41d4-a716-446655440001"
    When a client sends GET /s/aB3kR9
    Then the response status code is 302
    And the Location header is a presigned URL for file "660e8400-e29b-41d4-a716-446655440001"
    And the hit_count for slug "aB3kR9" is incremented asynchronously

  @http @redirect @cache-miss
  Scenario: Redirect cache miss queries Postgres and populates Redis
    Given a ShortLink exists with slug "xYz789" and no expiry
    And Redis does NOT contain the mapping for "slug:xYz789"
    When a client sends GET /s/xYz789
    Then the response status code is 302
    And the Location header is a presigned URL
    And the slug-to-file_id mapping was resolved from Postgres
    And Redis now contains the mapping "slug:xYz789" -> file_id with TTL 300 seconds

  @http @redirect @not-found
  Scenario: Redirect with unknown slug returns 404
    Given no ShortLink exists with slug "nonexist"
    When a client sends GET /s/nonexist
    Then the response status code is 404
    And the response body is a JSON error with code "NOT_FOUND"

  @http @redirect @expired
  Scenario: Redirect with expired link returns 410 Gone
    Given a ShortLink exists with slug "expired1"
    And the ShortLink has expires_at set to a timestamp in the past
    When a client sends GET /s/expired1
    Then the response status code is 410
    And the response body is a JSON error with code "GONE"
