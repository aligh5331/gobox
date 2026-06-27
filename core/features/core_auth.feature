# SPEC: Core API Phase 2 — Auth & /me Endpoints
# BUDGET: medium (5-10K)
# SCOPE: core/**
# STATUS: draft

Feature: Core API — Auth and User Profile Endpoints
  The Core API exposes REST endpoints for user registration, authentication,
  session management, and profile operations. It acts as a thin gateway that
  validates JWT tokens locally (RS256, JWKS cache refreshed every 5 min) and
  proxies business logic to the Auth service via gRPC.

  Background:
    Given the Auth gRPC server is running on port 8081
    And the Core API HTTP server is running on port 8080
    And the Core API JWKS cache has loaded the Auth public key at startup
    And a registered user exists with:
      | Field    | Value                                |
      | id       | f47ac10b-58cc-4372-a567-0e02b2c3d479 |
      | email    | ali@example.com                      |
      | name     | Ali                                  |
      | password | correctPass1!                        |
    And the user "ali@example.com" has an active session with id "a47ac10b-58cc-4372-a567-0e02b2c3d479"
    And a valid JWT for user "f47ac10b-58cc-4372-a567-0e02b2c3d479" exists with:
      | Field | Value                                |
      | sub   | f47ac10b-58cc-4372-a567-0e02b2c3d479 |
      | sid   | a47ac10b-58cc-4372-a567-0e02b2c3d479 |
      | exp   | <future timestamp>                   |
    And an expired JWT for user "f47ac10b-58cc-4372-a567-0e02b2c3d479" exists with:
      | Field | Value                                |
      | sub   | f47ac10b-58cc-4372-a567-0e02b2c3d479 |
      | sid   | a47ac10b-58cc-4372-a567-0e02b2c3d479 |
      | exp   | <past timestamp>                     |

  # ---------------------------------------------------------------------------
  # POST /api/v1/auth/register
  # ---------------------------------------------------------------------------

  Scenario: Register a new user successfully
    Given the email "newuser@example.com" is not yet registered
    When a POST request is sent to "/api/v1/auth/register" with JSON body:
      """
      {
        "email": "newuser@example.com",
        "password": "SecurePass123!",
        "name": "New User"
      }
      """
    Then the response status code is 201
    And the response JSON has a "user" object with:
      | Field | Value               |
      | email | newuser@example.com |
      | name  | New User            |
    And the response JSON "user.id" is a valid UUID
    And the response JSON "user.created_at" is a non-empty string
    And the response JSON "user.updated_at" is a non-empty string
    And the response JSON has a "tokens" object
    And the response JSON "tokens.access_token" is a non-empty string
    And the response JSON "tokens.refresh_token" is a non-empty string
    And the response JSON "tokens.expires_in" is a positive integer
    And the response JSON has a "session" object
    And the response JSON "session.id" is a valid UUID
    And the response JSON "session.user_id" is a valid UUID

  Scenario: Register with an already-registered email
    Given the email "ali@example.com" is already registered
    When a POST request is sent to "/api/v1/auth/register" with JSON body:
      """
      {
        "email": "ali@example.com",
        "password": "SecurePass123!",
        "name": "Ali Again"
      }
      """
    Then the response status code is 409
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "CONFLICT"
    And the response JSON "error.message" is a non-empty string

  # ---------------------------------------------------------------------------
  # POST /api/v1/auth/login
  # ---------------------------------------------------------------------------

  Scenario: Login with valid credentials
    When a POST request is sent to "/api/v1/auth/login" with JSON body:
      """
      {
        "email": "ali@example.com",
        "password": "correctPass1!"
      }
      """
    Then the response status code is 200
    And the response JSON has a "user" object with:
      | Field | Value                                |
      | id    | f47ac10b-58cc-4372-a567-0e02b2c3d479 |
      | email | ali@example.com                      |
      | name  | Ali                                  |
    And the response JSON has a "tokens" object
    And the response JSON "tokens.access_token" is a non-empty string
    And the response JSON "tokens.refresh_token" is a non-empty string
    And the response JSON "tokens.expires_in" is a positive integer
    And the response JSON has a "session" object
    And the response JSON "session.id" is a valid UUID
    And the response JSON "session.user_id" is "f47ac10b-58cc-4372-a567-0e02b2c3d479"

  Scenario: Login with wrong password
    When a POST request is sent to "/api/v1/auth/login" with JSON body:
      """
      {
        "email": "ali@example.com",
        "password": "wrongPassword99!"
      }
      """
    Then the response status code is 401
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "UNAUTHORIZED"
    And the response JSON "error.message" is a non-empty string

  # ---------------------------------------------------------------------------
  # POST /api/v1/auth/refresh
  # ---------------------------------------------------------------------------

  Scenario: Refresh tokens with a valid refresh token
    Given a valid refresh token "valid-refresh-token-abc" exists for session "a47ac10b-58cc-4372-a567-0e02b2c3d479"
    When a POST request is sent to "/api/v1/auth/refresh" with JSON body:
      """
      {
        "refresh_token": "valid-refresh-token-abc"
      }
      """
    Then the response status code is 200
    And the response JSON has a "tokens" object
    And the response JSON "tokens.access_token" is a non-empty string
    And the response JSON "tokens.refresh_token" is a non-empty string
    And the response JSON "tokens.refresh_token" is not "valid-refresh-token-abc"
    And the response JSON "tokens.expires_in" is a positive integer

  Scenario: Refresh tokens with an expired refresh token
    Given the refresh token "expired-refresh-token-xyz" is expired
    When a POST request is sent to "/api/v1/auth/refresh" with JSON body:
      """
      {
        "refresh_token": "expired-refresh-token-xyz"
      }
      """
    Then the response status code is 401
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "UNAUTHORIZED"
    And the response JSON "error.message" is a non-empty string

  # ---------------------------------------------------------------------------
  # DELETE /api/v1/auth/logout
  # ---------------------------------------------------------------------------

  Scenario: Logout with a valid session
    Given the request includes an Authorization header with the valid JWT
    When a DELETE request is sent to "/api/v1/auth/logout" with JSON body:
      """
      {
        "session_id": "a47ac10b-58cc-4372-a567-0e02b2c3d479"
      }
      """
    Then the response status code is 204
    And the response body is empty

  Scenario: Logout without an Authorization header (missing token)
    Given the request has no Authorization header
    When a DELETE request is sent to "/api/v1/auth/logout" with JSON body:
      """
      {
        "session_id": "a47ac10b-58cc-4372-a567-0e02b2c3d479"
      }
      """
    Then the response status code is 401
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "UNAUTHORIZED"
    And the response JSON "error.message" is a non-empty string

  # ---------------------------------------------------------------------------
  # GET /api/v1/me
  # ---------------------------------------------------------------------------

  Scenario: Get own profile with a valid token
    Given the request includes an Authorization header with the valid JWT
    When a GET request is sent to "/api/v1/me"
    Then the response status code is 200
    And the response JSON "id" is "f47ac10b-58cc-4372-a567-0e02b2c3d479"
    And the response JSON "email" is "ali@example.com"
    And the response JSON "name" is "Ali"
    And the response JSON "created_at" is a non-empty string
    And the response JSON "updated_at" is a non-empty string

  Scenario: Get own profile with an expired token
    Given the request includes an Authorization header with the expired JWT
    When a GET request is sent to "/api/v1/me"
    Then the response status code is 401
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "UNAUTHORIZED"
    And the response JSON "error.message" is a non-empty string

  # ---------------------------------------------------------------------------
  # PUT /api/v1/me
  # ---------------------------------------------------------------------------

  Scenario: Update own profile with valid fields
    Given the request includes an Authorization header with the valid JWT
    When a PUT request is sent to "/api/v1/me" with JSON body:
      """
      {
        "name": "Ali Reza"
      }
      """
    Then the response status code is 200
    And the response JSON "id" is "f47ac10b-58cc-4372-a567-0e02b2c3d479"
    And the response JSON "email" is "ali@example.com"
    And the response JSON "name" is "Ali Reza"
    And the response JSON "updated_at" is a non-empty string

  Scenario: Update own profile with empty name
    Given the request includes an Authorization header with the valid JWT
    When a PUT request is sent to "/api/v1/me" with JSON body:
      """
      {
        "name": ""
      }
      """
    Then the response status code is 400
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "BAD_REQUEST"
    And the response JSON "error.message" is a non-empty string

  Scenario: Update own profile with missing name field
    Given the request includes an Authorization header with the valid JWT
    When a PUT request is sent to "/api/v1/me" with JSON body:
      """
      {}
      """
    Then the response status code is 400
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "BAD_REQUEST"
    And the response JSON "error.message" is a non-empty string

  # ---------------------------------------------------------------------------
  # PUT /api/v1/me/password
  # ---------------------------------------------------------------------------

  Scenario: Change password with correct old password
    Given the request includes an Authorization header with the valid JWT
    When a PUT request is sent to "/api/v1/me/password" with JSON body:
      """
      {
        "old_password": "correctPass1!",
        "new_password": "newSecurePass99!"
      }
      """
    Then the response status code is 204
    And the response body is empty

  Scenario: Change password with wrong old password
    Given the request includes an Authorization header with the valid JWT
    When a PUT request is sent to "/api/v1/me/password" with JSON body:
      """
      {
        "old_password": "wrongOldPass!",
        "new_password": "newSecurePass99!"
      }
      """
    Then the response status code is 403
    And the response JSON matches the error envelope
    And the response JSON "error.code" is "FORBIDDEN"
    And the response JSON "error.message" is a non-empty string
