package referral

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles database operations for referrals
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new referral repository
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ===== Models =====

// ReferralCode represents a user's unique referral code
type ReferralCode struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
}

// Referral represents a referral relationship
type Referral struct {
	ID              string     `json:"id"`
	ReferrerID      string     `json:"referrer_id"`
	ReferredID      string     `json:"referred_id"`
	ReferralCodeID  string     `json:"referral_code_id"`
	Status          string     `json:"status"` // pending, active, churned
	ReferredAt      time.Time  `json:"referred_at"`
	ActivatedAt     *time.Time `json:"activated_at,omitempty"`
	ChurnedAt       *time.Time `json:"churned_at,omitempty"`
	ReferredName    string     `json:"referred_name,omitempty"`    // For display
	ReferredAvatar  string     `json:"referred_avatar,omitempty"` // For display
}

// ReferralStats represents aggregated stats for a user
type ReferralStats struct {
	Code            string  `json:"code"`
	TotalReferrals  int     `json:"total_referrals"`
	ActiveReferrals int     `json:"active_referrals"`
	TotalEarned     float64 `json:"total_earned"`
	CurrentBalance  float64 `json:"current_balance"`
	ShareLink       string  `json:"share_link"`
}

// ReferralEarning represents a monthly commission
type ReferralEarning struct {
	ID               string    `json:"id"`
	ReferrerID       string    `json:"referrer_id"`
	ReferralID       string    `json:"referral_id"`
	Month            string    `json:"month"` // YYYY-MM
	SubscriptionAmount float64 `json:"subscription_amount"`
	CommissionRate   float64   `json:"commission_rate"`
	CommissionAmount float64   `json:"commission_amount"`
	Status           string    `json:"status"`
	ReferredName     string    `json:"referred_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ===== Code Generation =====

// generateRandomCode generates a random alphanumeric code
func generateRandomCode(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return strings.ToUpper(hex.EncodeToString(bytes)[:length])
}

// GetOrCreateReferralCode gets existing or creates new referral code for user
func (r *Repository) GetOrCreateReferralCode(ctx context.Context, userID string) (*ReferralCode, error) {
	// Try to get existing
	var code ReferralCode
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, code, created_at
		FROM referral_codes
		WHERE user_id = $1
	`, userID).Scan(&code.ID, &code.UserID, &code.Code, &code.CreatedAt)

	if err == nil {
		return &code, nil
	}

	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to get referral code: %w", err)
	}

	// Generate new code
	// Get user's first name for personalized code
	var firstName *string
	r.pool.QueryRow(ctx, `
		SELECT COALESCE(first_name, pseudo)
		FROM users WHERE id = $1
	`, userID).Scan(&firstName)

	var prefix string
	if firstName != nil && len(*firstName) >= 3 {
		prefix = strings.ToUpper((*firstName)[:4])
	} else {
		prefix = "FOCUS"
	}

	// Try to create unique code
	var newCode string
	for i := 0; i < 10; i++ {
		newCode = prefix + "-" + generateRandomCode(5)

		_, err := r.pool.Exec(ctx, `
			INSERT INTO referral_codes (user_id, code)
			VALUES ($1, $2)
		`, userID, newCode)

		if err == nil {
			break
		}

		// Code collision, try again
		if i == 9 {
			return nil, fmt.Errorf("failed to generate unique code after 10 attempts")
		}
	}

	// Fetch the created code
	err = r.pool.QueryRow(ctx, `
		SELECT id, user_id, code, created_at
		FROM referral_codes
		WHERE user_id = $1
	`, userID).Scan(&code.ID, &code.UserID, &code.Code, &code.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch created code: %w", err)
	}

	return &code, nil
}

// GetReferralCodeByCode finds a referral code by its code string
func (r *Repository) GetReferralCodeByCode(ctx context.Context, code string) (*ReferralCode, error) {
	var rc ReferralCode
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, code, created_at
		FROM referral_codes
		WHERE code = $1
	`, strings.ToUpper(code)).Scan(&rc.ID, &rc.UserID, &rc.Code, &rc.CreatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get referral code: %w", err)
	}

	return &rc, nil
}

// ===== Referral Creation =====

// CreateReferral creates a referral when a new user signs up with a code
func (r *Repository) CreateReferral(ctx context.Context, referrerID, referredID, referralCodeID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO referrals (referrer_id, referred_id, referral_code_id, status, referred_at)
		VALUES ($1, $2, $3, 'pending', NOW())
		ON CONFLICT (referred_id) DO NOTHING
	`, referrerID, referredID, referralCodeID)

	if err != nil {
		return fmt.Errorf("failed to create referral: %w", err)
	}

	return nil
}

// ActivateReferral marks a referral as active when the referred user subscribes
func (r *Repository) ActivateReferral(ctx context.Context, referredID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE referrals
		SET status = 'active', activated_at = NOW()
		WHERE referred_id = $1 AND status = 'pending'
	`, referredID)

	if err != nil {
		return fmt.Errorf("failed to activate referral: %w", err)
	}

	return nil
}

// ChurnReferral marks a referral as churned when the referred user cancels
func (r *Repository) ChurnReferral(ctx context.Context, referredID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE referrals
		SET status = 'churned', churned_at = NOW()
		WHERE referred_id = $1 AND status = 'active'
	`, referredID)

	if err != nil {
		return fmt.Errorf("failed to churn referral: %w", err)
	}

	return nil
}

// ===== Stats & Lists =====

// GetReferralStats returns aggregated stats for a user
func (r *Repository) GetReferralStats(ctx context.Context, userID string) (*ReferralStats, error) {
	// Get or create code first
	code, err := r.GetOrCreateReferralCode(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create referral code: %w", err)
	}

	var stats ReferralStats
	stats.Code = code.Code
	stats.ShareLink = "https://apps.apple.com/app/focus-fire-level/id6743387301"

	// Get counts - using separate queries for robustness
	// Count total referrals
	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM referrals r
		JOIN referral_codes rc ON r.referral_code_id = rc.id
		WHERE rc.user_id = $1 AND r.status IN ('pending', 'active')
	`, userID).Scan(&stats.TotalReferrals)
	if err != nil && err != pgx.ErrNoRows {
		// Table might not exist, return empty stats
		stats.TotalReferrals = 0
	}

	// Count active referrals
	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM referrals r
		JOIN referral_codes rc ON r.referral_code_id = rc.id
		WHERE rc.user_id = $1 AND r.status = 'active'
	`, userID).Scan(&stats.ActiveReferrals)
	if err != nil && err != pgx.ErrNoRows {
		stats.ActiveReferrals = 0
	}

	// Get total earned
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(commission_amount), 0)
		FROM referral_earnings
		WHERE referrer_id = $1 AND status = 'credited'
	`, userID).Scan(&stats.TotalEarned)
	if err != nil && err != pgx.ErrNoRows {
		stats.TotalEarned = 0
	}

	// Get current balance
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(current_balance, 0)
		FROM referral_credits
		WHERE user_id = $1
	`, userID).Scan(&stats.CurrentBalance)
	if err != nil && err != pgx.ErrNoRows {
		stats.CurrentBalance = 0
	}

	return &stats, nil
}

// GetReferrals returns list of referrals for a user
func (r *Repository) GetReferrals(ctx context.Context, userID string) ([]Referral, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			r.id, r.referrer_id, r.referred_id, r.referral_code_id,
			r.status, r.referred_at, r.activated_at, r.churned_at,
			COALESCE(u.first_name, u.pseudo, 'Utilisateur') as referred_name,
			COALESCE(u.avatar_url, '') as referred_avatar
		FROM referrals r
		JOIN users u ON u.id = r.referred_id
		WHERE r.referrer_id = $1
		ORDER BY r.referred_at DESC
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to get referrals: %w", err)
	}
	defer rows.Close()

	var referrals []Referral
	for rows.Next() {
		var ref Referral
		err := rows.Scan(
			&ref.ID, &ref.ReferrerID, &ref.ReferredID, &ref.ReferralCodeID,
			&ref.Status, &ref.ReferredAt, &ref.ActivatedAt, &ref.ChurnedAt,
			&ref.ReferredName, &ref.ReferredAvatar,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan referral: %w", err)
		}
		referrals = append(referrals, ref)
	}

	return referrals, nil
}

// GetEarnings returns list of earnings for a user
func (r *Repository) GetEarnings(ctx context.Context, userID string, limit int) ([]ReferralEarning, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			e.id, e.referrer_id, e.referral_id,
			TO_CHAR(e.month, 'YYYY-MM') as month,
			e.subscription_amount, e.commission_rate, e.commission_amount,
			e.status, e.created_at,
			COALESCE(u.first_name, u.pseudo, 'Utilisateur') as referred_name
		FROM referral_earnings e
		JOIN referrals r ON r.id = e.referral_id
		JOIN users u ON u.id = r.referred_id
		WHERE e.referrer_id = $1
		ORDER BY e.month DESC, e.created_at DESC
		LIMIT $2
	`, userID, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get earnings: %w", err)
	}
	defer rows.Close()

	var earnings []ReferralEarning
	for rows.Next() {
		var e ReferralEarning
		err := rows.Scan(
			&e.ID, &e.ReferrerID, &e.ReferralID,
			&e.Month, &e.SubscriptionAmount, &e.CommissionRate, &e.CommissionAmount,
			&e.Status, &e.CreatedAt, &e.ReferredName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan earning: %w", err)
		}
		earnings = append(earnings, e)
	}

	return earnings, nil
}

// ===== Commission Processing (for cron job) =====

// RecordMonthlyEarning records a commission for a referral
func (r *Repository) RecordMonthlyEarning(ctx context.Context, referralID string, subscriptionAmount float64, commissionRate float64) error {
	commissionAmount := subscriptionAmount * commissionRate

	// Get first day of current month
	now := time.Now()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Get referrer ID from referral
	var referrerID string
	err := r.pool.QueryRow(ctx, `
		SELECT referrer_id FROM referrals WHERE id = $1
	`, referralID).Scan(&referrerID)

	if err != nil {
		return fmt.Errorf("failed to get referrer: %w", err)
	}

	// Insert earning (ignore if already exists for this month)
	_, err = r.pool.Exec(ctx, `
		INSERT INTO referral_earnings (referrer_id, referral_id, month, subscription_amount, commission_rate, commission_amount, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		ON CONFLICT (referral_id, month) DO NOTHING
	`, referrerID, referralID, month, subscriptionAmount, commissionRate, commissionAmount)

	if err != nil {
		return fmt.Errorf("failed to record earning: %w", err)
	}

	return nil
}

// CreditPendingEarnings credits all pending earnings to user balances
func (r *Repository) CreditPendingEarnings(ctx context.Context) error {
	// Start transaction
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get all pending earnings grouped by referrer
	rows, err := tx.Query(ctx, `
		SELECT referrer_id, SUM(commission_amount) as total
		FROM referral_earnings
		WHERE status = 'pending'
		GROUP BY referrer_id
	`)
	if err != nil {
		return fmt.Errorf("failed to get pending earnings: %w", err)
	}

	type credit struct {
		UserID string
		Amount float64
	}
	var credits []credit

	for rows.Next() {
		var c credit
		if err := rows.Scan(&c.UserID, &c.Amount); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan credit: %w", err)
		}
		credits = append(credits, c)
	}
	rows.Close()

	// Credit each user
	for _, c := range credits {
		// Upsert credit balance
		_, err := tx.Exec(ctx, `
			INSERT INTO referral_credits (user_id, total_earned, current_balance, updated_at)
			VALUES ($1, $2, $2, NOW())
			ON CONFLICT (user_id) DO UPDATE SET
				total_earned = referral_credits.total_earned + $2,
				current_balance = referral_credits.current_balance + $2,
				updated_at = NOW()
		`, c.UserID, c.Amount)

		if err != nil {
			return fmt.Errorf("failed to credit user %s: %w", c.UserID, err)
		}
	}

	// Mark all as credited
	_, err = tx.Exec(ctx, `
		UPDATE referral_earnings
		SET status = 'credited', credited_at = NOW()
		WHERE status = 'pending'
	`)
	if err != nil {
		return fmt.Errorf("failed to mark earnings as credited: %w", err)
	}

	return tx.Commit(ctx)
}

// GetReferrerForUser returns the referrer ID if user was referred
func (r *Repository) GetReferrerForUser(ctx context.Context, userID string) (string, error) {
	var referrerID string
	err := r.pool.QueryRow(ctx, `
		SELECT referrer_id FROM referrals WHERE referred_id = $1
	`, userID).Scan(&referrerID)

	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get referrer: %w", err)
	}

	return referrerID, nil
}

// ===== Admin Functions =====

// UserBalance represents a user's referral balance for admin view
type UserBalance struct {
	UserID         string  `json:"user_id"`
	Email          string  `json:"email"`
	FirstName      string  `json:"first_name"`
	Code           string  `json:"code"`
	ActiveReferrals int    `json:"active_referrals"`
	TotalEarned    float64 `json:"total_earned"`
	CurrentBalance float64 `json:"current_balance"`
}

// DeductBalance reduces a user's balance after payment
func (r *Repository) DeductBalance(ctx context.Context, userID string, amount float64) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE referral_credits
		SET current_balance = current_balance - $2,
		    updated_at = NOW()
		WHERE user_id = $1 AND current_balance >= $2
	`, userID, amount)

	if err != nil {
		return fmt.Errorf("failed to deduct balance: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("insufficient balance or user not found")
	}

	return nil
}

// GetAllPositiveBalances returns all users with balance > 0
func (r *Repository) GetAllPositiveBalances(ctx context.Context) ([]UserBalance, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			c.user_id,
			COALESCE(u.email, '') as email,
			COALESCE(u.first_name, u.pseudo, 'User') as first_name,
			COALESCE(rc.code, '') as code,
			(SELECT COUNT(*) FROM referrals ref WHERE ref.referrer_id = c.user_id AND ref.status = 'active') as active_referrals,
			c.total_earned,
			c.current_balance
		FROM referral_credits c
		JOIN users u ON u.id = c.user_id
		LEFT JOIN referral_codes rc ON rc.user_id = c.user_id
		WHERE c.current_balance > 0
		ORDER BY c.current_balance DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}
	defer rows.Close()

	var balances []UserBalance
	for rows.Next() {
		var b UserBalance
		err := rows.Scan(&b.UserID, &b.Email, &b.FirstName, &b.Code, &b.ActiveReferrals, &b.TotalEarned, &b.CurrentBalance)
		if err != nil {
			return nil, fmt.Errorf("failed to scan balance: %w", err)
		}
		balances = append(balances, b)
	}

	return balances, nil
}
