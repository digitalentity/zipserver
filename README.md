# Zipserver

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

## URL Structure

| Path | Description |
|------|-------------|
| `/` | Lists all available **Books**. |
| `/{book}/` | Lists all available **Versions** for the selected book. |
| `/{book}/latest/` | Permanent link to the **latest** version of the book. |
| `/{book}/{version}/` | Serves the `index.html` from the version's zip archive. |
| `/{book}/{version}/{path}` | Serves the specific file from the version's zip archive. |
| `/_/upload?book=X&version=Y` | **POST/PUT**: Uploads a new zip version for a book. |

## Authentication

### Web UI (Google OAuth 2.0)
Access to the browsing interface is restricted via Google OAuth. Only users matching the `allowed_users` patterns (exact email or `*@domain.com`) can log in. System routes like login and callback are prefixed with `/_/` (e.g., `/_/login`, `/_/callback`) to avoid collisions with book names.

### API (Bearer Token)
The `/_/upload` endpoint requires an `Authorization: Bearer <token>` header.
```bash
curl -X POST "http://localhost:8080/_/upload?book=docs&version=v1.2.0" \
     -H "Authorization: Bearer your-secret-token" \
     --data-binary @build.zip
```

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
