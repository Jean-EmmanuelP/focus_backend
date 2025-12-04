# Quests API Documentation

Quests represent big, finite goals or projects (e.g., "Read 12 Books", "Run a Marathon"). They are linked to a specific Area and track progress towards a target value.

## Data Model

```json
{
  "id": "uuid",
  "area_id": "uuid (foreign key to areas)",
  "title": "string (required)",
  "status": "string (default: 'active')",
  "current_value": "integer (default: 0)",
  "target_value": "integer (default: 1)"
}
```

---

## Endpoints

### 1. List Quests
Retrieves all quests for the user. Optionally filter by Area.

- **URL:** `/quests`
- **Method:** `GET`
- **Query Params:** `?area_id={uuid}` (Optional)
- **Auth:** Required
- **Response:** `200 OK`
  ```json
  [
    {
      "id": "q1q2q3...",
      "area_id": "a1b2...",
      "title": "Read 12 Books",
      "status": "active",
      "current_value": 3,
      "target_value": 12
    }
  ]
  ```

### 2. Create Quest
Creates a new goal.

- **URL:** `/quests`
- **Method:** `POST`
- **Auth:** Required
- **Body:**
  ```json
  {
    "area_id": "a1b2...",
    "title": "Save $10,000",
    "target_value": 10000
  }
  ```
- **Response:** `200 OK` (Returns the created object)

### 3. Update Quest
Updates quest details or progress.

- **URL:** `/quests/{id}`
- **Method:** `PATCH`
- **Auth:** Required
- **Body:**
  ```json
  {
    "current_value": 2500,
    "status": "active"
  }
  ```
- **Response:** `200 OK` (Returns the updated object)

### 4. Delete Quest
Deletes a quest permanently.

- **URL:** `/quests/{id}`
- **Method:** `DELETE`
- **Auth:** Required
- **Response:** `200 OK`
