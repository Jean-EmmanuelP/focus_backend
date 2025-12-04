# Areas API Documentation

Areas represent the major "buckets" or categories of a user's life (e.g., "Health", "Career", "Social"). Each area acts as a container for Quests (Goals) and Routines (Habits).

## Data Model

```json
{
  "id": "uuid",
  "name": "string (required)",
  "slug": "string (optional, unique per user)",
  "icon": "string (optional, icon name)",
  "completeness": "integer (0-100, calculated progress)"
}
```

---

## Endpoints

### 1. List Areas
Retrieves all areas created by the authenticated user.

- **URL:** `/areas`
- **Method:** `GET`
- **Auth:** Required
- **Response:** `200 OK`
  ```json
  [
    {
      "id": "a1b2c3d4-...",
      "name": "Health",
      "slug": "health",
      "icon": "heart",
      "completeness": 0
    }
  ]
  ```

### 2. Create Area
Creates a new life area.

- **URL:** `/areas`
- **Method:** `POST`
- **Auth:** Required
- **Body:**
  ```json
  {
    "name": "Career",
    "slug": "career",
    "icon": "briefcase"
  }
  ```
- **Response:** `200 OK` (Returns the created object)

### 3. Update Area
Updates an existing area. Only fields provided in the body will be updated.

- **URL:** `/areas/{id}`
- **Method:** `PATCH`
- **Auth:** Required
- **Body:**
  ```json
  {
    "name": "Professional Life"
  }
  ```
- **Response:** `200 OK` (Returns the updated object)

### 4. Delete Area
Deletes an area and **cascades** to delete all associated Quests and Routines.

- **URL:** `/areas/{id}`
- **Method:** `DELETE`
- **Auth:** Required
- **Response:** `200 OK`
