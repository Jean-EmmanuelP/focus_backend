# Routines API Documentation

Routines represent recurring habits (e.g., "Drink Water", "Go to Gym"). They are linked to an Area and can be "completed" multiple times.

## Data Model

```json
{
  "id": "uuid",
  "area_id": "uuid (foreign key to areas)",
  "title": "string (required)",
  "frequency": "string (default: 'daily')",
  "icon": "string (optional)"
}
```

---

## Endpoints

### 1. List Routines
Retrieves all routines for the user. Optionally filter by Area.

- **URL:** `/routines`
- **Method:** `GET`
- **Query Params:** `?area_id={uuid}` (Optional)
- **Auth:** Required
- **Response:** `200 OK`
  ```json
  [
    {
      "id": "r1r2r3...",
      "area_id": "a1b2...",
      "title": "Morning Yoga",
      "frequency": "daily",
      "icon": "yoga"
    }
  ]
  ```

### 2. Create Routine
Creates a new habit to track.

- **URL:** `/routines`
- **Method:** `POST`
- **Auth:** Required
- **Body:**
  ```json
  {
    "area_id": "a1b2...",
    "title": "Drink 2L Water",
    "frequency": "daily",
    "icon": "water"
  }
  ```
- **Response:** `200 OK` (Returns the created object)

### 3. Update Routine
Updates routine details.

- **URL:** `/routines/{id}`
- **Method:** `PATCH`
- **Auth:** Required
- **Body:**
  ```json
  {
    "title": "Drink 3L Water"
  }
  ```
- **Response:** `200 OK` (Returns the updated object)

### 4. Delete Routine
Deletes a routine and **cascades** to delete all its completion history.

- **URL:** `/routines/{id}`
- **Method:** `DELETE`
- **Auth:** Required
- **Response:** `200 OK`

---

## Routine Completions

### 5. Complete Routine (Check In)
Logs a completion for the routine at the current timestamp.
**Note:** If a completion already exists for the same Routine+User at the exact same timestamp, it does nothing (idempotent).

- **URL:** `/routines/{id}/complete`
- **Method:** `POST`
- **Auth:** Required
- **Body:** Empty
- **Response:** `200 OK`

### 6. Batch Complete Routines
Logs completions for multiple routines at once. Useful for "Mark All as Done" features.

- **URL:** `/routines/complete-batch`
- **Method:** `POST`
- **Auth:** Required
- **Body:**
  ```json
  {
    "routine_ids": ["uuid-1", "uuid-2", "uuid-3"]
  }
  ```
- **Response:** `200 OK`

### 7. Undo Completion
Removes the **most recent** completion for this routine. Useful for accidental clicks.

- **URL:** `/routines/{id}/complete`
- **Method:** `DELETE`
- **Auth:** Required
- **Response:** `200 OK`
