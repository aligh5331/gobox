Feature: Auth Service — User Registration, Authentication, and Session Management

  As a user of GoBox
  I want to register, log in, and manage my account and sessions
  So that I can securely access my files and shares

  Background:
    Given the RSA key manager is initialized with an active signing key
    And the User repository is empty
    And the Session repository is empty

  # ========================================================================== #
  # Use case: Register
  # Input: email, name, password
  # Output: User, AccessToken, RefreshToken
  # ========================================================================== #

  # SPEC: auth/register
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Register a new user successfully
    Given a user does not exist with email "alice@example.com"
    When the Register use case is called with email "alice@example.com", name "Alice", and password "ValidPass1!"
    Then a User is created with email "alice@example.com" and name "Alice"
    And the User's password hash is a valid bcrypt hash of "ValidPass1!"
    And an AccessToken is returned with a sub claim matching the new User's id
    And the AccessToken has claims: email="alice@example.com", name="Alice", iat set, exp=iat+900s, jti present, sid present
    And a RefreshToken is returned as a 43-character base64url opaque string
    And a Session is created for the new User with expires_at = now + 30 days
    And the Session's refresh_token_hash is a valid bcrypt hash of the returned RefreshToken

  # SPEC: auth/register
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Register with a duplicate email fails
    Given a user already exists with email "alice@example.com"
    When the Register use case is called with email "alice@example.com", name "Bob", and password "AnotherPass1!"
    Then the use case returns an error "EMAIL_ALREADY_EXISTS"
    And no User, AccessToken, or RefreshToken is returned

  # SPEC: auth/register
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Register with a weak password fails
    Given password policy requires minimum 8 characters, at least one uppercase letter, one lowercase letter, and one digit
    When the Register use case is called with email "bob@example.com", name "Bob", and password "short"
    Then the use case returns an error "WEAK_PASSWORD"
    And no User is created

  # ========================================================================== #
  # Use case: Login
  # Input: email, password
  # Output: User, AccessToken, RefreshToken, Session
  # ========================================================================== #

  # SPEC: auth/login
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Login with valid credentials succeeds
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    When the Login use case is called with email "alice@example.com" and password "ValidPass1!"
    Then the User's email is "alice@example.com"
    And an AccessToken is returned with sub matching the User's id
    And the AccessToken has claims: email="alice@example.com", iat set, exp=iat+900s, jti present, sid present
    And a RefreshToken is returned as a 43-character base64url opaque string
    And a Session is returned with user_id matching the User's id
    And the Session's expires_at = now + 30 days
    And the Session's refresh_token_hash is a valid bcrypt hash of the returned RefreshToken

  # SPEC: auth/login
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Login with a wrong password fails
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    When the Login use case is called with email "alice@example.com" and password "WrongPass1!"
    Then the use case returns an error "INVALID_CREDENTIALS"
    And no AccessToken, RefreshToken, or Session is returned

  # SPEC: auth/login
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Login with an unknown email fails
    Given no user exists with email "unknown@example.com"
    When the Login use case is called with email "unknown@example.com" and password "AnyPass1!"
    Then the use case returns an error "INVALID_CREDENTIALS"
    And no AccessToken, RefreshToken, or Session is returned

  # ========================================================================== #
  # Use case: RefreshToken
  # Input: refresh_token
  # Output: new AccessToken, new RefreshToken (rotation)
  # ========================================================================== #

  # SPEC: auth/refresh
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Refresh a valid token successfully rotates credentials
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And a Session exists for the user with a valid RefreshToken "rt_original"
    When the RefreshToken use case is called with RefreshToken "rt_original"
    Then the old Session with id matching the original sid is deleted from the database
    And a new Session is created with a new id
    And a new AccessToken is returned with a new jti and a new sid
    And a new RefreshToken "rt_rotated" is returned
    And the new Session's refresh_token_hash is a valid bcrypt hash of "rt_rotated"
    And the AccessToken has exp = iat + 900s

  # SPEC: auth/refresh
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Refresh with an expired session fails
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And a Session exists for the user with a valid RefreshToken "rt_expired"
    And the Session's expires_at is in the past (now - 1 day)
    When the RefreshToken use case is called with RefreshToken "rt_expired"
    Then the use case returns an error "SESSION_EXPIRED"
    And no new AccessToken or RefreshToken is returned
    And the expired Session remains in the database unchanged

  # SPEC: auth/refresh
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Refresh with a revoked session fails
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And a Session exists for the user with a valid RefreshToken "rt_revoked"
    And the Session is marked as revoked
    When the RefreshToken use case is called with RefreshToken "rt_revoked"
    Then the use case returns an error "SESSION_REVOKED"
    And no new AccessToken or RefreshToken is returned

  # SPEC: auth/refresh
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Reusing an already-rotated refresh token is detected as token theft
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And a Session exists for the user with RefreshToken "rt_stolen"
    And the RefreshToken use case has already been called with "rt_stolen" (rotating it to "rt_new")
    When the RefreshToken use case is called again with RefreshToken "rt_stolen"
    Then the use case returns an error "TOKEN_THEFT_DETECTED"
    And no new tokens are issued
    And the Session associated with "rt_new" remains valid

  # ========================================================================== #
  # Use case: Logout
  # Input: session_id
  # Output: —
  # ========================================================================== #

  # SPEC: auth/logout
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Logout revokes the active session
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And a Session exists for the user with id "session-1"
    And the Session is not revoked
    When the Logout use case is called with session_id "session-1"
    Then the Session with id "session-1" is marked as revoked
    And the Session's revoked flag is TRUE

  # SPEC: auth/logout
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: Logout with an already-revoked session fails
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And a Session exists for the user with id "session-1"
    And the Session is already revoked
    When the Logout use case is called with session_id "session-1"
    Then the use case returns an error "SESSION_ALREADY_REVOKED"

  # ========================================================================== #
  # Use case: LogoutAll
  # Input: user_id
  # Output: — (revoke all sessions)
  # ========================================================================== #

  # SPEC: auth/logoutall
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: LogoutAll revokes all active sessions for a user
    Given a user exists with email "alice@example.com" and password "ValidPass1!"
    And the user has 3 active sessions (ids "s1", "s2", "s3")
    And none of the sessions are revoked
    When the LogoutAll use case is called with the user's id
    Then all 3 Sessions for the user are marked as revoked
    And the user has 0 active (non-revoked) sessions remaining

  # SPEC: auth/logoutall
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: LogoutAll with no active sessions succeeds as a no-op
    Given a user exists with email "bob@example.com" and password "ValidPass1!"
    And the user has no active sessions
    When the LogoutAll use case is called with the user's id
    Then the use case returns successfully with no error

  # ========================================================================== #
  # Use case: GetUser
  # Input: user_id
  # Output: User
  # ========================================================================== #

  # SPEC: auth/getuser
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: GetUser returns the user for a valid user_id
    Given a user exists with email "alice@example.com", name "Alice", and password "ValidPass1!"
    When the GetUser use case is called with the user's id
    Then the returned User has email "alice@example.com" and name "Alice"

  # SPEC: auth/getuser
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: GetUser with an unknown user_id fails
    Given no user exists with id "00000000-0000-0000-0000-000000000000"
    When the GetUser use case is called with id "00000000-0000-0000-0000-000000000000"
    Then the use case returns an error "USER_NOT_FOUND"

  # ========================================================================== #
  # Use case: UpdateProfile
  # Input: user_id, name
  # Output: User
  # ========================================================================== #

  # SPEC: auth/updateprofile
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: UpdateProfile changes the user's display name
    Given a user exists with email "alice@example.com", name "Alice", and password "ValidPass1!"
    When the UpdateProfile use case is called with the user's id and name "Alice Updated"
    Then the returned User has name "Alice Updated"
    And the User's email remains "alice@example.com"

  # SPEC: auth/updateprofile
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: UpdateProfile with an empty name fails
    Given a user exists with email "alice@example.com", name "Alice", and password "ValidPass1!"
    When the UpdateProfile use case is called with the user's id and name ""
    Then the use case returns an error "INVALID_NAME"
    And the User's name remains "Alice"

  # ========================================================================== #
  # Use case: ChangePassword
  # Input: user_id, old_pass, new_pass
  # Output: —
  # ========================================================================== #

  # SPEC: auth/changepassword
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: ChangePassword with correct old password succeeds
    Given a user exists with email "alice@example.com", name "Alice", and password "OldPass1!"
    When the ChangePassword use case is called with the user's id, old_pass "OldPass1!", and new_pass "NewPass2!"
    Then the User's password hash is a valid bcrypt hash of "NewPass2!"
    And the password hash is no longer a valid bcrypt hash of "OldPass1!"

  # SPEC: auth/changepassword
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: ChangePassword with wrong old password fails
    Given a user exists with email "alice@example.com", name "Alice", and password "OldPass1!"
    When the ChangePassword use case is called with the user's id, old_pass "WrongPass1!", and new_pass "NewPass2!"
    Then the use case returns an error "INVALID_PASSWORD"
    And the User's password hash remains a valid hash of "OldPass1!"

  # SPEC: auth/changepassword
  # BUDGET: 1 session
  # SCOPE: domain + usecase layer only
  # STATUS: draft
  Scenario: ChangePassword with a weak new password fails
    Given a user exists with email "alice@example.com", name "Alice", and password "OldPass1!"
    When the ChangePassword use case is called with the user's id, old_pass "OldPass1!", and new_pass "short"
    Then the use case returns an error "WEAK_PASSWORD"
    And the User's password hash remains a valid hash of "OldPass1!"
