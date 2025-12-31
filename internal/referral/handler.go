package referral

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/telegram"
)

// Handler handles referral-related HTTP requests
type Handler struct {
	repo *Repository
}

// NewHandler creates a new referral handler
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// ===== Response Types =====

type StatsResponse struct {
	Code            string  `json:"code"`
	ShareLink       string  `json:"share_link"`
	TotalReferrals  int     `json:"total_referrals"`
	ActiveReferrals int     `json:"active_referrals"`
	TotalEarned     float64 `json:"total_earned"`
	CurrentBalance  float64 `json:"current_balance"`
	CommissionRate  float64 `json:"commission_rate"` // 0.20 = 20%
}

type ReferralResponse struct {
	ID            string  `json:"id"`
	ReferredName  string  `json:"referred_name"`
	ReferredAvatar string `json:"referred_avatar"`
	Status        string  `json:"status"`
	ReferredAt    string  `json:"referred_at"`
	ActivatedAt   *string `json:"activated_at,omitempty"`
}

type EarningResponse struct {
	Month            string  `json:"month"`
	ReferredName     string  `json:"referred_name"`
	SubscriptionAmount float64 `json:"subscription_amount"`
	CommissionAmount float64 `json:"commission_amount"`
	Status           string  `json:"status"`
}

type ApplyCodeRequest struct {
	Code string `json:"code"`
}

type ApplyCodeResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ReferrerName string `json:"referrer_name,omitempty"`
}

// ===== Handlers =====

// GetStats returns the user's referral stats and code
// GET /referral/stats
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	stats, err := h.repo.GetReferralStats(r.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to get referral stats: %v", err)
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	response := StatsResponse{
		Code:            stats.Code,
		ShareLink:       stats.ShareLink,
		TotalReferrals:  stats.TotalReferrals,
		ActiveReferrals: stats.ActiveReferrals,
		TotalEarned:     stats.TotalEarned,
		CurrentBalance:  stats.CurrentBalance,
		CommissionRate:  0.20, // 20%
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetReferrals returns the list of people the user has referred
// GET /referral/list
func (h *Handler) GetReferrals(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	referrals, err := h.repo.GetReferrals(r.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to get referrals: %v", err)
		http.Error(w, "Failed to get referrals", http.StatusInternalServerError)
		return
	}

	// Convert to response format - initialize to empty array (not nil)
	response := []ReferralResponse{}
	for _, ref := range referrals {
		item := ReferralResponse{
			ID:            ref.ID,
			ReferredName:  ref.ReferredName,
			ReferredAvatar: ref.ReferredAvatar,
			Status:        ref.Status,
			ReferredAt:    ref.ReferredAt.Format("2006-01-02"),
		}
		if ref.ActivatedAt != nil {
			activatedStr := ref.ActivatedAt.Format("2006-01-02")
			item.ActivatedAt = &activatedStr
		}
		response = append(response, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetEarnings returns the user's commission history
// GET /referral/earnings
func (h *Handler) GetEarnings(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	earnings, err := h.repo.GetEarnings(r.Context(), userID, 50)
	if err != nil {
		log.Printf("❌ Failed to get earnings: %v", err)
		http.Error(w, "Failed to get earnings", http.StatusInternalServerError)
		return
	}

	// Convert to response format - initialize to empty array (not nil)
	response := []EarningResponse{}
	for _, e := range earnings {
		response = append(response, EarningResponse{
			Month:              e.Month,
			ReferredName:       e.ReferredName,
			SubscriptionAmount: e.SubscriptionAmount,
			CommissionAmount:   e.CommissionAmount,
			Status:             e.Status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ApplyCode applies a referral code for a new user
// POST /referral/apply
func (h *Handler) ApplyCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ApplyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}

	// Check if user already has a referrer
	existingReferrer, err := h.repo.GetReferrerForUser(r.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to check existing referrer: %v", err)
		http.Error(w, "Failed to apply code", http.StatusInternalServerError)
		return
	}

	if existingReferrer != "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApplyCodeResponse{
			Success: false,
			Message: "Tu as deja ete parraine",
		})
		return
	}

	// Find the referral code
	referralCode, err := h.repo.GetReferralCodeByCode(r.Context(), req.Code)
	if err != nil {
		log.Printf("❌ Failed to find referral code: %v", err)
		http.Error(w, "Failed to apply code", http.StatusInternalServerError)
		return
	}

	if referralCode == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApplyCodeResponse{
			Success: false,
			Message: "Code invalide",
		})
		return
	}

	// Can't refer yourself
	if referralCode.UserID == userID {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApplyCodeResponse{
			Success: false,
			Message: "Tu ne peux pas utiliser ton propre code",
		})
		return
	}

	// Create the referral
	err = h.repo.CreateReferral(r.Context(), referralCode.UserID, userID, referralCode.ID)
	if err != nil {
		log.Printf("❌ Failed to create referral: %v", err)
		http.Error(w, "Failed to apply code", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Referral created: %s referred by %s", userID, referralCode.UserID)

	// Send Telegram notification
	telegram.Get().Send(telegram.Event{
		Type:     telegram.EventReferralApplied,
		UserID:   userID,
		UserName: "Nouveau user",
		Data: map[string]interface{}{
			"referrer_name": referralCode.Code,
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ApplyCodeResponse{
		Success: true,
		Message: "Code applique avec succes !",
	})
}

// ValidateCode checks if a referral code is valid (public endpoint)
// GET /referral/validate/{code}
func (h *Handler) ValidateCode(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}

	referralCode, err := h.repo.GetReferralCodeByCode(r.Context(), code)
	if err != nil {
		log.Printf("❌ Failed to validate code: %v", err)
		http.Error(w, "Failed to validate code", http.StatusInternalServerError)
		return
	}

	if referralCode == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid": true,
		"code":  referralCode.Code,
	})
}

// ===== Webhook Handlers (called by subscription events) =====

// ActivateUserReferral should be called when a referred user subscribes
// This activates the referral and starts earning commissions
func (h *Handler) ActivateUserReferral(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Activate referral
	err := h.repo.ActivateReferral(r.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to activate referral: %v", err)
		http.Error(w, "Failed to activate referral", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Referral activated for user %s", userID)

	// Send Telegram notification for referral activation (subscription!)
	telegram.Get().Send(telegram.Event{
		Type:     telegram.EventReferralActivated,
		UserID:   userID,
		UserName: "User",
		Data:     map[string]interface{}{},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ProcessMonthlyCommissions processes all pending commissions (cron job)
// POST /jobs/referral/process-commissions
func (h *Handler) ProcessMonthlyCommissions(w http.ResponseWriter, r *http.Request) {
	err := h.repo.CreditPendingEarnings(r.Context())
	if err != nil {
		log.Printf("❌ Failed to process commissions: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Println("✅ Monthly commissions processed")
	w.Write([]byte("OK"))
}

// MarkUserPaid marks a user's balance as paid (admin endpoint)
// POST /admin/referral/mark-paid
// Body: { "user_id": "uuid", "amount": 18.00 }
func (h *Handler) MarkUserPaid(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string  `json:"user_id"`
		Amount float64 `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.Amount <= 0 {
		http.Error(w, "user_id and amount are required", http.StatusBadRequest)
		return
	}

	err := h.repo.DeductBalance(r.Context(), req.UserID, req.Amount)
	if err != nil {
		log.Printf("❌ Failed to mark paid: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Marked %.2f€ as paid for user %s", req.Amount, req.UserID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("%.2f€ deducted from balance", req.Amount),
	})
}

// GetAllBalances returns all users with positive balances (admin endpoint)
// GET /admin/referral/balances
func (h *Handler) GetAllBalances(w http.ResponseWriter, r *http.Request) {
	balances, err := h.repo.GetAllPositiveBalances(r.Context())
	if err != nil {
		log.Printf("❌ Failed to get balances: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balances)
}
