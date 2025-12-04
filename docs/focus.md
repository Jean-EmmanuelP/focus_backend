# Focus Sessions API Documentation

Focus Sessions allow users to track "Deep Work" or Pomodoro-style sessions. These sessions can optionally be linked to a Quest to track time spent on specific goals.

## Data Model

```json
{
  "id": "uuid",
  "quest_id": "uuid (optional, foreign key to quests)",
  "description": "string (optional)",
  "duration_minutes": "integer (required, target duration)",
  "status": "string ('active', 'completed', 'cancelled')",
  "started_at": "timestamp (ISO 8601)",
  "completed_at": "timestamp (ISO 8601, null if active)"
}
```

---

## Endpoints

### 1. Start Session
Creates a new active focus session.

- **URL:** `/focus-sessions`
- **Method:** `POST`
- **Auth:** Required
- **Body:**
  ```json
  {
    "quest_id": "q1...", 
    "description": "Writing Chapter 1",
    "duration_minutes": 25
  }
  ```
- **Response:** `200 OK` (Returns the created object with status='active')

### 2. Update Session (Complete/Cancel)
Updates the session status. Typically used to mark a session as 'completed' when the timer ends, or 'cancelled' if interrupted.

- **URL:** `/focus-sessions/{id}`
- **Method:** `PATCH`
- **Auth:** Required
- **Body:**
  ```json
  {
    "status": "completed" 
  }
  ```
  *(Note: When status is set to 'completed', the backend automatically sets `completed_at` to the current time.)*

- **Response:** `200 OK` (Returns the updated object)

### 3. List Sessions (History)
Retrieves a history of focus sessions. Useful for analytics ("How many hours did I work today?").

- **URL:** `/focus-sessions`
- **Method:** `GET`
- **Auth:** Required
- **Query Params:**
  - `quest_id={uuid}` (Optional)
  - `status={string}` (Optional, e.g., 'completed')
  - `limit={int}` (Default: 20)
- **Response:** `200 OK`
  ```json
  [
    {
      "id": "f1...",
      "description": "Writing Chapter 1",
      "duration_minutes": 25,
      "status": "completed",
      "started_at": "2023-10-27T10:00:00Z",
      "completed_at": "2023-10-27T10:25:00Z"
    }
  ]
  ```

### 4. Delete Session
Deletes a session history record.

- **URL:** `/focus-sessions/{id}`
- **Method:** `DELETE`
- **Auth:** Required
- **Response:** `200 OK`
