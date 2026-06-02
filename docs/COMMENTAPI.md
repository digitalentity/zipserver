# Custom Comments API Contract Specification (W3C Web Annotation Compliant)

This document details the minimal REST API contract for the custom commenting and annotation system. It adheres strictly to the **W3C Web Annotation Data Model** (JSON-LD format), meaning both original highlights and thread replies are stored as standard Annotation entities.

---

## 1. Authentication & Session

The comments system uses cookie-based session authentication for host pages.

### 1.1 Get Current User Session
Check if the user is logged in.

*   **Endpoint:** `GET /_/api/v1/auth/me`
*   **Response (Authenticated - 200 OK):**
    ```json
    {
      "authenticated": true,
      "user": {
        "id": "usr_902183",
        "name": "Alex Mercer",
        "avatarUrl": "https://avatars.githubusercontent.com/u/102938",
        "email": "alex.mercer@example.com"
      }
    }
    ```
*   **Response (Guest / Unauthenticated - 200 OK):**
    ```json
    {
      "authenticated": false,
      "user": null
    }
    ```

### 1.2 OAuth Redirection Endpoints
*   `GET /_/login?redirect=<url>`: Redirects user to backend OAuth authorization page.
*   `GET /_/logout?redirect=<url>`: Clears session and redirects back.

---

## 2. W3C Web Annotation Schemas

Every comment, highlight, and reply is modeled as an `Annotation` object.

### 2.1 Parent Annotation (Text Selection / Highlight)
Created when a user highlights text and adds an initial comment.

```json
{
  "@context": "http://www.w3.org/ns/anno.jsonld",
  "id": "anno_xyz789",
  "type": "Annotation",
  "created": "2026-05-30T14:20:00Z",
  "modified": "2026-05-30T14:20:00Z",
  "creator": {
    "id": "usr_902183",
    "type": "Person",
    "name": "Alex Mercer",
    "avatar": "https://avatars.githubusercontent.com/u/102938"
  },
  "body": {
    "type": "TextualBody",
    "value": "Is this APAS-specific?",
    "format": "text/markdown"
  },
  "target": {
    "source": "https://localhost:3000/system/architecture.html",
    "selector": [
      {
        "type": "TextQuoteSelector",
        "exact": "Triple Redundancy",
        "prefix": "The system features ",
        "suffix": " for high reliability."
      },
      {
        "type": "TextPositionSelector",
        "start": 20,
        "end": 37
      },
      {
        "type": "FragmentSelector",
        "value": "blockIdx=4&blockHash=f8a2bc49&tagName=P"
      }
    ]
  },
  "resolved": false
}
```

### 2.2 Reply Annotation (Thread Reply)
A reply is an annotation whose `target` is the ID of the parent annotation, and its `motivation` is set to `"replying"`. It does not need selection or position selectors.

```json
{
  "@context": "http://www.w3.org/ns/anno.jsonld",
  "id": "anno_abc123",
  "type": "Annotation",
  "created": "2026-05-30T14:25:00Z",
  "modified": "2026-05-30T14:25:00Z",
  "creator": {
    "id": "usr_881022",
    "type": "Person",
    "name": "Elena Rostova",
    "avatar": "https://..."
  },
  "body": {
    "type": "TextualBody",
    "value": "Yes, APAS runs on 3 physical hardware nodes with vote logic.",
    "format": "text/markdown"
  },
  "target": "anno_xyz789",
  "motivation": "replying"
}
```

---

## 3. Minimal Commenting CRUD API

The backend only needs to implement **four REST endpoints** to support the custom commenting engine.

### 3.1 Fetch Page Annotations (`GET /_/api/v1/comments`)
Retrieve all annotations (both parent threads and replies) for a specific page.

*   **Query Parameters:**
*   `page` (string, required): The relative path of the page (e.g. `/system/architecture.html`).
*   **Response (200 OK):**
    Returns an array of W3C Annotation objects. The client script will dynamically group replies with their parent annotations.

### 3.2 Create Annotation / Reply (`POST /_/api/v1/comments`)
Post a new annotation or thread reply.

*   **Payload (Annotation):**
    ```json
    {
      "body": {
        "value": "My comment text..."
      },
      "target": { ... } // Or string target ID for replies
    }
    ```
*   **Response (201 Created):** Returns the fully constructed W3C Annotation object.

### 3.3 Update Annotation (`PATCH /_/api/v1/comments/:id`)
Edit comment text or resolve/reopen a thread.

*   **Payload (Edit text):**
    ```json
    {
      "body": {
        "value": "Updated comment text..."
      }
    }
    ```
*   **Payload (Resolve/Reopen):**
    ```json
    {
      "resolved": true
    }
    ```
*   **Response (200 OK):** Returns the updated W3C Annotation object.

### 3.4 Delete Annotation (`DELETE /_/api/v1/comments/:id`)
Delete an annotation (only allowed if the active user matches the creator ID).

*   **Response (204 No Content):** Successful deletion. If it was a parent annotation, the backend should also delete (cascade) all reply annotations targeting it.

---

## 4. Mentions Autocomplete & Candidates Resolution

No additional database-backed search endpoints are required. Instead, autocompletion candidates are compiled client-side in the browser from existing endpoints:

1.  **Current User Info:** Extracted from the `GET /_/api/v1/auth/me` endpoint.
2.  **Page Participants:** Extracted from the array of existing annotations returned by `GET /_/api/v1/comments?page=...`. The client builds a set of unique users using the `creator.name` and `creator.email` fields.

When a user types `@` inside the comment/reply fields, matching user suggestions are filtered using both the email and name properties of these candidate sources.

