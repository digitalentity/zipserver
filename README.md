# Zipserver

[![Docker Image](https://img.shields.io/docker/pulls/kvsh/zipserver.svg)](https://hub.docker.com/r/kvsh/zipserver)

Zipserver is a simple Go application designed to serve static content (like `mdbook` builds) directly from zip archives. It features a hierarchical organization where **Books** represent projects and **Versions** represent specific builds (e.g., git commits or tags).

## Key Features

- **Hierarchical Content:** Organize content by `Book` and `Version`.
- **Latest Version Support:** Permanent links to the most recently uploaded version of any book.
- **Multi-Cloud Storage:**
  - **Local Filesystem:** Simple directory-based storage.
  - **Google Cloud Storage (GCS):** Serve from buckets with range-request optimization.
  - **Google Drive:** Serve from Drive folders using IDs or names.
- **Smart Local Caching:** Cloud-hosted zips are cached locally for near-instant access.
- **In-Memory Meta Caching:** Book and version lists are cached in memory with a configurable TTL to prevent backend rate limiting.
- **Zero-Extraction Serving:** Content is streamed directly from zip files without unzipping to disk.
- **Authenticated Uploads:** Dedicated `/_/upload` endpoint secured by Bearer tokens for CI/CD integration.
- **Web UI Authentication:** Integrated Google OAuth 2.0 with domain/user allow-listing.
- **Modern Architecture:** Modular Go implementation with dependency injection and clean separation of concerns.

## URL & API Endpoints

### Static Content & Uploads

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Lists all available **Books**. |
| `GET` | `/{book}/` | Lists all available **Versions** for the selected book. |
| `GET` | `/{book}/latest/` | 302 Redirect to the **latest** version of the book. |
| `GET` | `/{book}/{version}/` | Serves the `index.html` from the version's zip archive. |
| `GET` | `/{book}/{version}/{path}` | Serves the specific file from the version's zip archive. |
| `POST/PUT` | `/_/upload?book=X&version=Y` | Uploads a new zip version for a book. |

### Authentication API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/_/api/v1/auth/me` | Gets current user session and profile details. |
| `GET` | `/_/login?redirect=X` | Initiates Google OAuth flow, preserving the redirect target path. |
| `GET` | `/_/logout?redirect=X` | Clears the session cookie and redirects back. |
| `GET` | `/_/callback` | Google OAuth callback URL (processes authorization code). |

### Comments & Annotations API (W3C Web Annotation Compliant)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/_/api/v1/comments?page=X` | Retrieves all annotations and replies for a specific page. |
| `POST` | `/_/api/v1/comments` | Creates a new annotation or thread reply. |
| `PATCH` | `/_/api/v1/comments/{id}` | Updates comment text or resolves/reopens a thread. |
| `DELETE` | `/_/api/v1/comments/{id}` | Deletes an annotation and cascades delete to replies (owner only). |

## API Reference

### 1. File Upload API

#### Upload Book Version (`POST/PUT /_/upload`)
Uploads a compiled documentation zip archive for a specific book and version.
*   **Query Parameters**:
    *   `book` (string, required): The identifier of the book (e.g., `docs`).
    *   `version` (string, required): The version tag or git commit hash (e.g., `v1.2.0`).
*   **Headers**:
    *   `Authorization: Bearer <your-secret-token>` (Bearer authentication token from configuration).
*   **Request Body**: Raw binary data of the ZIP archive.
*   **Response (201 Created)**:
    ```json
    {
      "outcome": "success",
      "uri": "/docs/v1.2.0/"
    }
    ```

### 2. Authentication API

#### Get User Profile (`GET /_/api/v1/auth/me`)
Retrieves the logged-in user profile or unauthenticated status.
*   **Response (Authenticated - 200 OK)**:
    ```json
    {
      "authenticated": true,
      "user": {
        "id": "usr_102938102938102938",
        "name": "Alex Mercer",
        "avatarUrl": "https://lh3.googleusercontent.com/..."
      }
    }
    ```
*   **Response (Unauthenticated - 200 OK)**:
    ```json
    {
      "authenticated": false,
      "user": null
    }
    ```

#### Initiate Login (`GET /_/login`)
Redirects the user to Google's OAuth consent screen.
*   **Query Parameters**:
    *   `redirect` (string, optional): The URL path to redirect to after successful authentication.

#### Process Logout (`GET /_/logout`)
Destroys the active session cookie.
*   **Query Parameters**:
    *   `redirect` (string, optional): The URL path to redirect to after logout completes (defaults to `/`).

### 3. Comments & Annotations API

Every comment, highlight, and reply is modeled as a W3C Web Annotation entity.

#### Get Page Comments (`GET /_/api/v1/comments`)
Fetches all annotation threads (highlights and replies) for a specific book page.
*   **Query Parameters**:
    *   `page` (string, required): The relative URL path or absolute URL of the page (e.g., `/docs/v1.0/system/architecture.html`).
*   **Response (200 OK)**: JSON array of Annotation entities.

#### Create Comment or Reply (`POST /_/api/v1/comments`)
Creates a parent highlight annotation or a thread reply.
*   **Payload (Parent Highlight/Comment)**:
    ```json
    {
      "body": {
        "value": "My comment markdown..."
      },
      "target": {
        "source": "/docs/v1.0/system/architecture.html",
        "selector": [
          {
            "type": "TextQuoteSelector",
            "exact": "Triple Redundancy"
          }
        ]
      }
    }
    ```
*   **Payload (Thread Reply)**:
    ```json
    {
      "body": {
        "value": "My reply markdown..."
      },
      "target": "anno_xyz789",
      "motivation": "replying"
    }
    ```
*   **Response (201 Created)**: The fully constructed JSON W3C Annotation entity.

#### Update Comment (`PATCH /_/api/v1/comments/{id}`)
Modifies a comment's body text or resolves a thread.
*   **Payload (Edit Text - Creator Only)**:
    ```json
    {
      "body": {
        "value": "Updated text..."
      }
    }
    ```
*   **Payload (Resolve/Reopen)**:
    ```json
    {
      "resolved": true
    }
    ```
*   **Response (200 OK)**: The updated Annotation entity.

#### Delete Comment (`DELETE /_/api/v1/comments/{id}`)
Deletes an annotation. If a parent is deleted, all reply annotations targeting it are cascade-deleted.
*   **Authorization**: Allowed only for the creator of the comment.
*   **Response (204 No Content)**: Successful deletion.

## Configuration

Zipserver can be configured via `config.yaml` or Environment Variables.

### Environment Variables
Environment variables take precedence over the YAML file:
- `GOOGLE_CLIENT_ID`: OAuth Client ID
- `GOOGLE_CLIENT_SECRET`: OAuth Client Secret
- `GOOGLE_REDIRECT_URL`: OAuth Callback URL (e.g., `http://localhost:8080/_/callback`)
- `SESSION_KEY`: Gorilla Sessions key
- `UPLOAD_TOKEN`: Token for the `/_/upload` endpoint

### config.yaml Example
```yaml
port: "8080"
storage_type: "gcs" # "local", "gcs", or "drive"

# Caching settings
cache:
  dir: "./cache"   # Local disk cache for zips
  ttl: "5m"        # In-memory TTL for metadata (book/version lists)

# Backend specific settings
gcs:
  bucket: "my-docs-bucket"
  credentials_file: "gcs-sa.json" # Optional

drive:
  folder_id: "1abc...xyz"
  credentials_file: "drive-sa.json" # Optional

auth:
  enabled: true
  client_id: "..."
  client_secret: "..."
  redirect_url: "http://localhost:8080/_/callback"
  allowed_users:
    - "*@my-company.com"
  session_key: "change-me-to-something-random"

upload:
  enabled: true
  token: "your-secret-token"
```

## Getting Started

### Prerequisites
- Go 1.25 or higher.
- A Google Cloud Project with OAuth configured (for Web UI).
- Service Account credentials (for GCS/Drive).

### Building and Running
```bash
# Build the binary
make build

# Run with default config.yaml
./zipserver
```

### Docker

#### Official Image
The official image is available on Docker Hub: [kvsh/zipserver](https://hub.docker.com/r/kvsh/zipserver)

```bash
docker pull kvsh/zipserver:latest
```

#### Local Build and Run
```bash
# Build image
docker build -t zipserver .

# Run container
docker run -p 8080:8080 -v $(pwd)/config.yaml:/root/config.yaml zipserver
```

#### Pushing to Docker Hub
Ensure you are logged in via `docker login` first.

Using the Makefile:
```bash
# Build and push with default username (system user) and tag (latest)
make docker-release

# Specify custom username and version
make docker-release DOCKER_USER=myusername VERSION=v1.2.3
```

Manual steps:
```bash
docker build -t <username>/zipserver:latest .
docker push <username>/zipserver:latest
```

## License
MIT
