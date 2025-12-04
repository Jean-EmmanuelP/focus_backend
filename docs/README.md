# Firelevel Backend Documentation

## Overview
This backend manages the core logic for the Firelevel application, utilizing Go (Chi router) and PostgreSQL (Supabase).

## Modules

### [Areas](./areas.md)
The high-level buckets of life (Health, Career, etc.).
- **Features:** User-defined categories with icons.

### [Quests](./quests.md)
Finite goals with measurable progress.
- **Features:** Target values, progress tracking (current/target).

### [Routines](./routines.md)
Recurring habits.
- **Features:** Check-in system (Completions), frequency settings.

### [Completions](./completions.md)
History logs for routine executions.
- **Features:** Historical data for analytics/calendars.

### [Reflections](./reflections.md)
Daily journaling system.
- **Features:** One entry per day, structured prompts (Wins, Challenges, etc.).

### [Focus Sessions](./focus.md)
Pomodoro / Deep Work timer tracking.
- **Features:** Track duration, link to Quests, history logging.

### [Stats & Dashboard](./stats.md)
Aggregated analytics and performance endpoints.
- **Features:** Focus charts, routine heatmaps, single-request dashboard.

## Authentication
All endpoints require a valid Supabase JWT in the Authorization header:
`Authorization: Bearer <token>`

## Errors
Standard HTTP status codes are used:
- `200 OK`: Success
- `400 Bad Request`: Invalid JSON or missing fields
- `401 Unauthorized`: Invalid or missing token
- `404 Not Found`: Resource ID not found
- `500 Internal Server Error`: Database or server failure
