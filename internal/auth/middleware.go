package auth

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "userID"

// AuthMiddleware creates a handler that verifies the Bearer token using the JWT Secret
func AuthMiddleware(jwtSecret string) (func(http.Handler) http.Handler, error) {
	if jwtSecret == "" {
		return nil, fmt.Errorf("jwtSecret cannot be empty")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			// Extract the token from "Bearer <token>"
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				http.Error(w, "Invalid token format", http.StatusUnauthorized)
				return
			}

			// Parse and verify the token using the HMAC Secret
			token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
				// Validate the algorithm is what we expect (HMAC)
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				// Return the byte slice of the secret
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				log.Printf("Token validation failed: %v", err)
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			// Extract user ID (Subject)
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			userID, err := claims.GetSubject()
			if err != nil {
				http.Error(w, "Invalid token subject", http.StatusUnauthorized)
				return
			}

			// Add userID to context so handlers can use it
			ctx := context.WithValue(r.Context(), UserContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}