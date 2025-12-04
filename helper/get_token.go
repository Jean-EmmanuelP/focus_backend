package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("../.env")

	projectURL := os.Getenv("SUPABASE_URL")
	apiKey := os.Getenv("SUPABASE_KEY")

	if projectURL == "" || apiKey == "" {
		log.Fatal("SUPABASE_URL or SUPABASE_KEY is missing in .env")
	}

	email := "alexis.chatelain123@gmail.com"
	password := "securepassword123" // Min 6 chars

	fmt.Printf("Checking for user %s...\n", email)

	// 1. Try to Login first
	token, err := login(projectURL, apiKey, email, password)
	if err == nil {
		printToken(token)
		return
	}

	// 2. If login fails, try to Sign Up
	fmt.Println("Login failed (user might not exist). Attempting to Sign Up...")
	token, err = signup(projectURL, apiKey, email, password)
	if err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	printToken(token)
}

// ---------------------------------------------------------
// Helper Functions (Standard HTTP Requests)
// ---------------------------------------------------------

func login(url, key, email, password string) (string, error) {
	apiURL := fmt.Sprintf("%s/auth/v1/token?grant_type=password", url)
	payload := map[string]string{"email": email, "password": password}

	return makeRequest(apiURL, key, payload)
}

func signup(url, key, email, password string) (string, error) {
	apiURL := fmt.Sprintf("%s/auth/v1/signup", url)
	payload := map[string]string{"email": email, "password": password}

	return makeRequest(apiURL, key, payload)
}

func makeRequest(url, key string, payload interface{}) (string, error) {
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON to get access_token
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	token, ok := result["access_token"].(string)
	if !ok {
		return "", fmt.Errorf("no access_token in response. (Did you disable 'Confirm Email' in Supabase?)")
	}

	return token, nil
}

func printToken(token string) {
	fmt.Println("\n✅ Success! Here is your Bearer Token:")
	fmt.Println("---------------------------------------------------")
	fmt.Println(token)
	fmt.Println("---------------------------------------------------")
}
