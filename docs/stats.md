# Stats API Documentation

These endpoints provide aggregated analytics to power the dashboard, fire mode, and quests tab.

## Endpoints

### 1. Dashboard
High-performance endpoint for the home screen.
- **URL:** `/dashboard`
- **Method:** `GET`
- **Auth:** Required
- **Response:** `200 OK`
  ```json
  {
    "user": { "id": "...", "full_name": "..." },
    "areas": [ { "id": "...", "name": "..." } ],
    "active_quests": [ { "id": "...", "title": "..." } ],
    "todays_routines": [ 
        { "id": "...", "title": "...", "completed": true } 
    ],
    "stats": {
        "focused_today": 120,
        "streak_days": 5
    },
    "sessions_last_7": [
        { "date": "2023-10-01", "minutes": 45, "sessions": 2 }
    ]
  }
  ```

### 2. Fire Mode
Aggregated stats for the "Fire Mode" tracking screen.
- **URL:** `/firemode`
- **Method:** `GET`
- **Auth:** Required
- **Response:** `200 OK`
  ```json
  {
    "minutes_today": 120,
    "sessions_today": 4,
    "sessions_week": 15,
    "minutes_last_7": 840,
    "sessions_last_7": 28,
    "active_quests": [ ... ]
  }
  ```

### 3. Quests Tab
Loads all structural data (Areas, Quests, Routines) in one go for the management tab.
- **URL:** `/quests-tab`
- **Method:** `GET`
- **Auth:** Required
- **Response:** `200 OK`
  ```json
  {
    "areas": [ ... ],
    "quests": [ ... ],
    "routines": [ ... ]
  }
  ```