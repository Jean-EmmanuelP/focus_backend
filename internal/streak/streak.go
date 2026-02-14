package streak

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateUserStreak updates the user's streak based on daily engagement.
// Should be called when: user sends a chat message, completes a task,
// completes a routine, or finishes a focus session.
func UpdateUserStreak(ctx context.Context, db *pgxpool.Pool, userID string) {
	_, err := db.Exec(ctx, `
		UPDATE users SET
			current_streak = CASE
				WHEN last_active_date = CURRENT_DATE THEN current_streak
				WHEN last_active_date = CURRENT_DATE - 1 THEN COALESCE(current_streak, 0) + 1
				ELSE 1
			END,
			longest_streak = GREATEST(
				COALESCE(longest_streak, 0),
				CASE
					WHEN last_active_date = CURRENT_DATE THEN COALESCE(current_streak, 0)
					WHEN last_active_date = CURRENT_DATE - 1 THEN COALESCE(current_streak, 0) + 1
					ELSE 1
				END
			),
			last_active_date = CURRENT_DATE
		WHERE id = $1
	`, userID)
	if err != nil {
		fmt.Printf("Failed to update streak for user %s: %v\n", userID, err)
	}
}
