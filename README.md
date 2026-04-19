# Zipserver

The goal is to create a simple Go application that serves `mdbook` builds directly from zip archives located in a specific directory.

## Requirements

1. **Source Directory:** The application monitors a folder containing multiple `.zip` files.
2. **Index Page (`/`):** 
   - Displays a list of all available zip files in the source directory.
   - Files are sorted by their modification timestamp (newest first).
3. **Subdirectory Serving:** 
   - Each zip file is served from a path matching its filename (e.g., `/my-book/` serves the contents of `my-book.zip`).
   - Content is served directly from the zip archive without extracting to disk.
4. **HTTP Server:** 
   - Implement using standard `net/http`.
   - Correctly handle MIME types for all assets (HTML, CSS, JS, etc.).
   - Handle the root of the book (e.g., `/my-book/` should serve `index.html` from the zip).
5. **Authentication & Authorization:**
   - Supports Google OAuth 2.0.
   - Access is restricted to users matching patterns in `allowed_users`.
   - Patterns support exact emails (`user@example.com`) or domain wildcards (`*@company.com`).
6. **Configuration:** 
   - Configured via a YAML file (default `config.yaml`).
   - Supports `-config` command-line flag to specify a different config file path.

## Configuration (config.yaml)

```yaml
port: "8080"
zip_dir: "./publish"
auth:
  enabled: true
  client_id: "YOUR_GOOGLE_CLIENT_ID"
  client_secret: "YOUR_GOOGLE_CLIENT_SECRET"
  redirect_url: "http://localhost:8080/callback"
  allowed_users:
    - "*@company.com"
    - "user@thirdparty.com"
  session_key: "super-secret-session-key"
```

## Building

```bash
make build
```

This creates `zipserver` in the repository root.

## Running

```bash
./zipserver -config config.yaml
```
