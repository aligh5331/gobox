# SPEC: FileUpload — Direct-to-S3 Upload Flow
# BUDGET: medium 5-10K
# SCOPE: fileupload/**
# STATUS: draft

Feature: File Upload Center
  As a user of GoBox
  I want to upload, manage, and download files
  So that I can store and share my data through S3 presigned URLs

  Background:
    Given the FileUpload gRPC service is running and healthy
    And the Postgres database is connected and migrated
    And the S3/MinIO backend is accessible
    And a user exists with ID "user-abc-123"
    And another user exists with ID "user-xyz-789"

  # ---------------------------------------------------------------------------
  # InitiateUpload
  # ---------------------------------------------------------------------------

  Scenario: Initiate a file upload successfully
    When the user "user-abc-123" initiates an upload with:
      | field     | value                     |
      | name      | report.pdf                |
      | size      | 1048576                   |
      | mime_type | application/pdf           |
    Then the gRPC response has status code OK
    And the response field "file_id" is a valid UUID
    And the response field "upload_url" is a presigned S3 PUT URL
    And the file record has status "pending"
    And the file record has storage_key "uploads/user-abc-123/{file_id}/report.pdf"

  Scenario: InitiateUpload rejects an empty filename
    When the user "user-abc-123" initiates an upload with:
      | field | value    |
      | name  |          |
      | size  | 1048576  |
    Then the gRPC response has status code InvalidArgument
    And the error message indicates the filename is required

  Scenario: InitiateUpload rejects a zero-size file
    When the user "user-abc-123" initiates an upload with:
      | field | value      |
      | name  | empty.txt  |
      | size  | 0          |
    Then the gRPC response has status code InvalidArgument
    And the error message indicates the size must be positive

  # ---------------------------------------------------------------------------
  # ConfirmUpload
  # ---------------------------------------------------------------------------

  Scenario: Confirm a completed upload successfully
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | pending                   |
      | deleted_at | NULL                      |
    And the S3 object at key "uploads/user-abc-123/file-111/report.pdf" exists
    And the S3 object's Content-Length is 1048576
    And the S3 object's Content-Type is "application/pdf"
    When the user "user-abc-123" confirms the upload for file_id "file-111"
    Then the gRPC response has status code OK
    And the response field "status" is "ready"
    And the file record has status "ready"
    And the file record's updated_at is refreshed

  Scenario: ConfirmUpload is idempotent on an already-ready file
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NULL                      |
    When the user "user-abc-123" confirms the upload for file_id "file-111"
    Then the gRPC response has status code OK
    And the response field "status" is "ready"
    And no HEAD request is made to S3

  Scenario: ConfirmUpload rejects a nonexistent file_id
    Given no file record exists with id "file-999"
    When the user "user-abc-123" confirms the upload for file_id "file-999"
    Then the gRPC response has status code NotFound
    And the error message indicates the file was not found

  # ---------------------------------------------------------------------------
  # GetFile
  # ---------------------------------------------------------------------------

  Scenario: Retrieve file metadata for an existing file
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NULL                      |
    When the user "user-abc-123" calls GetFile for file_id "file-111"
    Then the gRPC response has status code OK
    And the response field "id" is "file-111"
    And the response field "name" is "report.pdf"
    And the response field "size" is 1048576
    And the response field "mime_type" is "application/pdf"
    And the response field "status" is "ready"
    And the response field "user_id" is "user-abc-123"

  Scenario: GetFile returns NotFound for a nonexistent file
    Given no file record exists with id "file-999"
    When the user "user-abc-123" calls GetFile for file_id "file-999"
    Then the gRPC response has status code NotFound
    And the error message indicates the file was not found

  Scenario: GetFile hides files owned by a different user
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NULL                      |
    When the user "user-xyz-789" calls GetFile for file_id "file-111"
    Then the gRPC response has status code NotFound
    And the error message does not reveal the owner's identity

  # ---------------------------------------------------------------------------
  # ListFiles
  # ---------------------------------------------------------------------------

  Scenario: ListFiles returns a paginated page of files
    Given the user "user-abc-123" has 25 file records with status "ready" and deleted_at NULL
    When the user "user-abc-123" calls ListFiles with page_size 10
    Then the gRPC response has status code OK
    And the response contains exactly 10 file records
    And the response field "next_page_token" is not empty

  Scenario: ListFiles returns an empty list when the user has no files
    Given the user "user-abc-123" has no file records
    When the user "user-abc-123" calls ListFiles with page_size 10
    Then the gRPC response has status code OK
    And the response contains exactly 0 file records
    And the response field "next_page_token" is empty

  Scenario: ListFiles navigates pages using page_token
    Given the user "user-abc-123" has 25 file records with status "ready" and deleted_at NULL
    When the user "user-abc-123" calls ListFiles with page_size 10
    And the user "user-abc-123" calls ListFiles with page_size 10 and page_token from the previous response
    Then the second response contains exactly 10 file records
    And the second response's file IDs differ from the first page
    And the second response field "next_page_token" is not empty
    When the user "user-abc-123" calls ListFiles with page_size 10 and page_token from the second response
    Then the third response contains exactly 5 file records
    And the third response field "next_page_token" is empty

  # ---------------------------------------------------------------------------
  # DeleteFile
  # ---------------------------------------------------------------------------

  Scenario: DeleteFile soft-deletes a file owned by the caller
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NULL                      |
    When the user "user-abc-123" calls DeleteFile for file_id "file-111"
    Then the gRPC response has status code OK
    And the file record's deleted_at is now set (IS NOT NULL)
    And a background goroutine is enqueued to remove the S3 object

  Scenario: DeleteFile returns NotFound for a file owned by a different user
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NULL                      |
    When the user "user-xyz-789" calls DeleteFile for file_id "file-111"
    Then the gRPC response has status code NotFound
    And the error message does not reveal the owner's identity

  # ---------------------------------------------------------------------------
  # GetDownloadURL
  # ---------------------------------------------------------------------------

  Scenario: GetDownloadURL returns a presigned GET URL for a ready file
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NULL                      |
    When the user "user-abc-123" calls GetDownloadURL for file_id "file-111"
    Then the gRPC response has status code OK
    And the response field "download_url" is a presigned S3 GET URL
    And the presigned URL is valid for PRESIGN_DOWNLOAD_TTL_MINUTES (default 60)

  Scenario: GetDownloadURL returns FailedPrecondition for a pending file
    Given a file record exists with:
      | field      | value                     |
      | id         | file-222                  |
      | user_id    | user-abc-123              |
      | name       | uploading.txt             |
      | size       | 512                       |
      | mime_type  | text/plain                |
      | status     | pending                   |
      | deleted_at | NULL                      |
    When the user "user-abc-123" calls GetDownloadURL for file_id "file-222"
    Then the gRPC response has status code FailedPrecondition
    And the error message indicates the file is not yet ready

  Scenario: GetDownloadURL returns NotFound for a soft-deleted file
    Given a file record exists with:
      | field      | value                     |
      | id         | file-111                  |
      | user_id    | user-abc-123              |
      | name       | report.pdf                |
      | size       | 1048576                   |
      | mime_type  | application/pdf           |
      | status     | ready                     |
      | deleted_at | NOW()                     |
    When the user "user-abc-123" calls GetDownloadURL for file_id "file-111"
    Then the gRPC response has status code NotFound
    And the error message indicates the file was not found
