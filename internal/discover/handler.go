package discover

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// DiscoverUser is the response DTO for nearby users
type DiscoverUser struct {
	ID               string  `json:"id"`
	Pseudo           *string `json:"pseudo"`
	FirstName        *string `json:"first_name"`
	AvatarURL        *string `json:"avatar_url"`
	LifeGoal         *string `json:"life_goal"`
	Hobbies          *string `json:"hobbies"`
	ProductivityPeak *string `json:"productivity_peak"`
	CurrentStreak    *int    `json:"current_streak"`
	City             *string `json:"city"`
	Country          *string `json:"country"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
}

// ListNearbyUsers returns users within a radius (km) of the given coordinates.
// GET /discover/users?lat=48.85&lon=2.35&radius=50
func (h *Handler) ListNearbyUsers(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	radiusStr := r.URL.Query().Get("radius")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || math.Abs(lat) > 90 {
		http.Error(w, `{"error":"invalid lat"}`, http.StatusBadRequest)
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || math.Abs(lon) > 180 {
		http.Error(w, `{"error":"invalid lon"}`, http.StatusBadRequest)
		return
	}

	radius := 50.0 // default 50km
	if radiusStr != "" {
		if r, err := strconv.ParseFloat(radiusStr, 64); err == nil && r > 0 && r <= 500 {
			radius = r
		}
	}

	query := `
		SELECT id, pseudo, first_name, avatar_url, life_goal, hobbies,
		       productivity_peak, current_streak, city, country, latitude, longitude
		FROM public.users
		WHERE id != $1
		  AND latitude IS NOT NULL
		  AND longitude IS NOT NULL
		  AND (discover_visible IS NULL OR discover_visible = true)
		  AND (
		    6371 * acos(
		      LEAST(1.0, GREATEST(-1.0,
		        cos(radians($2)) * cos(radians(latitude))
		        * cos(radians(longitude) - radians($3))
		        + sin(radians($2)) * sin(radians(latitude))
		      ))
		    )
		  ) <= $4
		ORDER BY location_updated_at DESC NULLS LAST
		LIMIT 100
	`

	rows, err := h.db.Query(r.Context(), query, userID, lat, lon, radius)
	if err != nil {
		http.Error(w, `{"error":"query failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := []DiscoverUser{}
	for rows.Next() {
		var u DiscoverUser
		if err := rows.Scan(
			&u.ID, &u.Pseudo, &u.FirstName, &u.AvatarURL, &u.LifeGoal, &u.Hobbies,
			&u.ProductivityPeak, &u.CurrentStreak, &u.City, &u.Country, &u.Latitude, &u.Longitude,
		); err != nil {
			continue
		}
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}
