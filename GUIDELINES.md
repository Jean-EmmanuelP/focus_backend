# Engineering Guidelines & Standards

This document establishes the strict coding standards, architectural patterns, and workflows for the **Firelevel Backend**. All contributors must adhere to these rules to ensure maintainability, security, and performance.

---

## 1. Project Structure

We follow the standard Go project layout. Do not deviate from this structure without a compelling architectural reason.

-   **`cmd/api/`**: Entry point for the application. Contains `main.go`. Logic here should be minimal: load config, setup DB, wire routes, start server.
-   **`internal/`**: Private application code. This code cannot be imported by other projects.
    -   **`internal/database/`**: Database connection logic (pgx pool setup).
    -   **`internal/auth/`**: Authentication middleware and helpers.
    -   **`internal/<domain/`**: Feature-specific logic (e.g., `internal/users/`, `internal/projects/`).
-   **`.env`**: Local environment variables (ignored by Git).
-   **`GUIDELINES.md`**: This file.

---

## 2. Go Coding Standards

### 2.1 Formatting & Linting
-   **Format on Save:** Code **MUST** be formatted using `gofmt` (or `goimports`).
-   **Unused Imports:** **MUST NOT** exist. The build will fail.
-   **Linting:** Run `go vet` and standard static analysis tools before committing.

### 2.2 Naming Conventions
-   **Variables:** CamelCase (`userID`, `dbPool`). Short names (`r`, `w`, `ctx`) are acceptable only within small scopes (like HTTP handlers).
-   **Exported Names:** Must start with a Capital letter.
-   **Interfaces:** Single-method interfaces should end in `-er` (e.g., `Reader`, `Writer`).

### 2.3 Error Handling
-   **Never Panic:** Do not use `panic()` in HTTP handlers. It crashes the server. Return a generic 500 error to the user and log the actual error.
-   **Wrap Errors:** When returning an error from a lower level, wrap it to provide context.
    ```go
    // BAD
    return err
    // GOOD
    return fmt.Errorf("failed to scan user row: %w", err)
    ```
-   **Check Errors:** Never ignore an error (using `_`) unless you are absolutely certain it is safe (e.g., closing a readonly file).

---

## 3. Database & SQL (pgx)

We use **Raw SQL** with the `pgx/v5` driver. We **DO NOT** use ORMs (like GORM).

### 3.1 Connections
-   Use `*pgxpool.Pool` for thread-safe, persistent connections.
-   Pass `context.Context` to **all** database calls.

### 3.2 Queries
-   **Parameter Substitution:** **NEVER** use `fmt.Sprintf` to build SQL strings with user input. This causes SQL Injection vulnerabilities.
    ```go
    // CRITICAL: NEVER DO THIS
    query := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", userID)

    // CORRECT
    query := "SELECT * FROM users WHERE id = $1"
    row := db.QueryRow(ctx, query, userID)
    ```
-   **Transactions:** Use `pgx.BeginTx` for operations that modify multiple tables or rows.

---

## 4. HTTP & API Design

We use `chi` for routing.

### 4.1 Request Handling
-   **Context:** Always propagate `r.Context()`.
-   **JSON Decoding:** Use `json.NewDecoder(r.Body).Decode(&struct)`.

### 4.2 DTOs (Data Transfer Objects)
-   Separate Database Models from API Request/Response structs.
-   **Partial Updates (PATCH):** Use **Pointers** in your request struct to distinguish between "missing field" (nil) and "empty value" (pointer to empty string).
    ```go
    type UpdateUserRequest struct {
        Name *string `json:"name"` // nil = do not update; "" = clear name
    }
    ```

### 4.3 Response Handling
-   **Standard Format:** Responses should be valid JSON.
-   **Status Codes:**
    -   `200 OK`: Successful synchronous call.
    -   `201 Created`: Resource created.
    -   `400 Bad Request`: Validation failure / Invalid JSON.
    -   `401 Unauthorized`: Missing or invalid JWT.
    -   `403 Forbidden`: Valid JWT, but insufficient permissions.
    -   `404 Not Found`: Resource does not exist.
    -   `500 Internal Server Error`: Something went wrong server-side.

---

## 5. Authentication & Security

### 5.1 JWT Verification
-   Tokens are verified using the **Supabase JWT Secret** (HMAC).
-   Do not implement your own token parsing logic; use the established middleware in `internal/auth/middleware.go`.

### 5.2 Context
-   User ID is extracted from the token and placed into the request context.
-   Retrieve it using `r.Context().Value(auth.UserContextKey).(string)`.

### 5.3 Secrets
-   **NEVER** commit API Keys, Secrets, or `.env` files to Git.
-   Use `.env.example` to document required variables without values.

---

## 6. Git Workflow

-   **Atomic Commits:** One feature or fix per commit.
-   **Commit Messages:** Use the imperative mood.
    -   *Bad:* "Fixed the bug"
    -   *Good:* "Fix nil pointer exception in user handler"
-   **Branching:** Create feature branches (e.g., `feature/add-projects`, `fix/user-update`). Do not push directly to `main`.

---

## 7. Logging

-   **Request Logging:** Relied on `chi` middleware logger for standard HTTP access logs.
-   **Error Logging:** Log genuine errors using `log.Println` or `log.Printf` before returning HTTP 500.
-   **Debug Logging:** **REMOVE** manual `fmt.Printf` timers or debug statements before committing code.

---

**Compliance with these guidelines is mandatory.** Code reviews will reject PRs that violate these standards.