# Daily Reflections API Documentation

Daily Reflections allow users to journal about their day, capturing wins, challenges, and goals for tomorrow. Each user can have **one** entry per day.

## Data Model

```json
{
  "id": "uuid",
  "date": "string (YYYY-MM-DD)",
  "biggest_win": "string (optional)",
  "challenges": "string (optional)",
  "best_moment": "string (optional)",
  "goal_for_tomorrow": "string (optional)"
}
```

---

## Endpoints

### 1. Get Reflection by Date
Retrieves the reflection entry for a specific date.

- **URL:** `/reflections/{date}`
- **Method:** `GET`
- **Auth:** Required
- **Path Params:** `date` (YYYY-MM-DD)
- **Response:** `200 OK`
  ```json
  {
    "id": "r1...",
    "date": "2023-10-27",
    "biggest_win": "Finished the project",
    "challenges": "None",
    "best_moment": "Lunch with friends",
    "goal_for_tomorrow": "Start new task"
  }
  ```
  *Returns 404 if no entry exists for that date.*

### 2. Upsert Reflection (Create or Update)
Creates a new entry for the given date or updates the existing one if it already exists. This simplifies the frontend logic (save button always hits this).

- **URL:** `/reflections/{date}`
- **Method:** `PUT`
- **Auth:** Required
- **Path Params:** `date` (YYYY-MM-DD)
- **Body:**
  ```json
  {
    "biggest_win": "Cleaned the house",
    "challenges": "Tired",
    "best_moment": "Coffee",
    "goal_for_tomorrow": "Run 5k"
  }
  ```
- **Response:** `200 OK` (Returns the updated object)

### 3. List Reflections
Retrieves a history of past reflections.

- **URL:** `/reflections`
- **Method:** `GET`
- **Auth:** Required
- **Query Params:**
  - `from={date}` (Optional)
  - `to={date}` (Optional)
  - `limit={int}` (Default: 10)
- **Response:** `200 OK`
