# Routine Completions API Documentation

The `routine_completions` table tracks the history of when a user completes a routine. While the primary interaction happens via the `/routines/{id}/complete` endpoints (documented in [Routines](./routines.md)), you may need direct access to the completion history for analytics, calendars, or "streaks" calculations.

## Data Model

```json
{
  "id": "uuid",
  "user_id": "uuid (foreign key to auth.users)",
  "routine_id": "uuid (foreign key to routines)",
  "completed_at": "timestamp (ISO 8601)"
}
```

---

## Endpoints

**Note:** These endpoints are strictly for reading history. Creating (completing) and Deleting (undoing) are handled via the main Routines API for better UX logic.

### 1. List Completions (History)
Retrieves a list of completion events. This is useful for populating a "contribution graph" or calendar view.

- **URL:** `/completions`
- **Method:** `GET`
- **Auth:** Required
- **Query Params:**
  - `routine_id={uuid}`: Filter history for a specific routine.
  - `from={timestamp}`: Get completions after this date (e.g., `2023-01-01`).
  - `to={timestamp}`: Get completions before this date.
- **Response:** `200 OK`
  ```json
  [
    {
      "id": "c1c2...",
      "routine_id": "r1r2...",
      "completed_at": "2023-10-27T14:30:00Z"
    },
    {
      "id": "c3c4...",
      "routine_id": "r1r2...",
      "completed_at": "2023-10-26T09:15:00Z"
    }
  ]
  ```
