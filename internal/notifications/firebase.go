package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// FirebaseService handles Firebase Cloud Messaging
type FirebaseService struct {
	client *messaging.Client
	mu     sync.RWMutex
}

var (
	firebaseInstance *FirebaseService
	firebaseOnce     sync.Once
)

// GetFirebaseService returns the singleton Firebase service
func GetFirebaseService() *FirebaseService {
	firebaseOnce.Do(func() {
		firebaseInstance = &FirebaseService{}
		if err := firebaseInstance.initialize(); err != nil {
			log.Printf("⚠️ Firebase initialization failed: %v", err)
			// Don't panic - notifications will be disabled but app will work
		}
	})
	return firebaseInstance
}

// initialize sets up the Firebase client
func (s *FirebaseService) initialize() error {
	ctx := context.Background()

	var opt option.ClientOption

	// Try to get credentials from environment variable (for production - Render)
	if credJSON := os.Getenv("FIREBASE_SERVICE_ACCOUNT"); credJSON != "" {
		opt = option.WithCredentialsJSON([]byte(credJSON))
		log.Println("🔥 Firebase: Using credentials from FIREBASE_SERVICE_ACCOUNT env var")
	} else if credPath := os.Getenv("FIREBASE_SERVICE_ACCOUNT_PATH"); credPath != "" {
		// Try to load from file path (for local development)
		opt = option.WithCredentialsFile(credPath)
		log.Printf("🔥 Firebase: Using credentials from file: %s", credPath)
	} else {
		return fmt.Errorf("no Firebase credentials found (set FIREBASE_SERVICE_ACCOUNT or FIREBASE_SERVICE_ACCOUNT_PATH)")
	}

	// Initialize Firebase app
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	// Get messaging client
	client, err := app.Messaging(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Firebase messaging client: %w", err)
	}

	s.mu.Lock()
	s.client = client
	s.mu.Unlock()

	log.Println("✅ Firebase Cloud Messaging initialized successfully")
	return nil
}

// IsAvailable returns true if Firebase is properly configured
func (s *FirebaseService) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil
}

// NotificationPayload represents the data for a push notification
type NotificationPayload struct {
	Title        string            `json:"title"`
	Body         string            `json:"body"`
	ImageURL     string            `json:"image_url,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
	Sound        string            `json:"sound,omitempty"`
	Badge        int               `json:"badge,omitempty"`
	ClickAction  string            `json:"click_action,omitempty"`
	DeepLink     string            `json:"deep_link,omitempty"`
	NotificationID string          `json:"notification_id,omitempty"`
	Type         string            `json:"type,omitempty"` // morning, task_reminder, task_missed, etc.
}

// SendToDevice sends a notification to a specific device using FCM token
func (s *FirebaseService) SendToDevice(ctx context.Context, fcmToken string, payload NotificationPayload) (string, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("firebase client not initialized")
	}

	// Build data map
	data := payload.Data
	if data == nil {
		data = make(map[string]string)
	}

	// Add standard fields to data
	if payload.DeepLink != "" {
		data["deepLink"] = payload.DeepLink
	}
	if payload.NotificationID != "" {
		data["notification_id"] = payload.NotificationID
	}
	if payload.Type != "" {
		data["type"] = payload.Type
	}

	// Build the message
	message := &messaging.Message{
		Token: fcmToken,
		Notification: &messaging.Notification{
			Title:    payload.Title,
			Body:     payload.Body,
			ImageURL: payload.ImageURL,
		},
		Data: data,
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound:            payload.Sound,
					Badge:            &payload.Badge,
					MutableContent:   true,
					ContentAvailable: true,
				},
			},
		},
	}

	// Set default sound if not specified
	if payload.Sound == "" {
		message.APNS.Payload.Aps.Sound = "default"
	}

	// Send the message
	response, err := client.Send(ctx, message)
	if err != nil {
		return "", fmt.Errorf("failed to send notification: %w", err)
	}

	log.Printf("✅ Notification sent successfully: %s", response)
	return response, nil
}

// SendToMultipleDevices sends a notification to multiple devices
func (s *FirebaseService) SendToMultipleDevices(ctx context.Context, fcmTokens []string, payload NotificationPayload) (*messaging.BatchResponse, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("firebase client not initialized")
	}

	if len(fcmTokens) == 0 {
		return nil, fmt.Errorf("no FCM tokens provided")
	}

	// Build data map
	data := payload.Data
	if data == nil {
		data = make(map[string]string)
	}

	if payload.DeepLink != "" {
		data["deepLink"] = payload.DeepLink
	}
	if payload.NotificationID != "" {
		data["notification_id"] = payload.NotificationID
	}
	if payload.Type != "" {
		data["type"] = payload.Type
	}

	// Build the multicast message
	message := &messaging.MulticastMessage{
		Tokens: fcmTokens,
		Notification: &messaging.Notification{
			Title:    payload.Title,
			Body:     payload.Body,
			ImageURL: payload.ImageURL,
		},
		Data: data,
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound:            "default",
					MutableContent:   true,
					ContentAvailable: true,
				},
			},
		},
	}

	// Send to all devices
	response, err := client.SendEachForMulticast(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send multicast notification: %w", err)
	}

	log.Printf("✅ Multicast notification sent: %d success, %d failures", response.SuccessCount, response.FailureCount)
	return response, nil
}

// SendToTopic sends a notification to all users subscribed to a topic
func (s *FirebaseService) SendToTopic(ctx context.Context, topic string, payload NotificationPayload) (string, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("firebase client not initialized")
	}

	// Build data map
	data := payload.Data
	if data == nil {
		data = make(map[string]string)
	}

	if payload.DeepLink != "" {
		data["deepLink"] = payload.DeepLink
	}
	if payload.Type != "" {
		data["type"] = payload.Type
	}

	message := &messaging.Message{
		Topic: topic,
		Notification: &messaging.Notification{
			Title:    payload.Title,
			Body:     payload.Body,
			ImageURL: payload.ImageURL,
		},
		Data: data,
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound:            "default",
					MutableContent:   true,
					ContentAvailable: true,
				},
			},
		},
	}

	response, err := client.Send(ctx, message)
	if err != nil {
		return "", fmt.Errorf("failed to send topic notification: %w", err)
	}

	log.Printf("✅ Topic notification sent to '%s': %s", topic, response)
	return response, nil
}

// SubscribeToTopic subscribes tokens to a topic
func (s *FirebaseService) SubscribeToTopic(ctx context.Context, tokens []string, topic string) error {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("firebase client not initialized")
	}

	response, err := client.SubscribeToTopic(ctx, tokens, topic)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	log.Printf("✅ Subscribed %d tokens to topic '%s' (%d failures)", len(tokens)-response.FailureCount, topic, response.FailureCount)
	return nil
}

// UnsubscribeFromTopic unsubscribes tokens from a topic
func (s *FirebaseService) UnsubscribeFromTopic(ctx context.Context, tokens []string, topic string) error {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("firebase client not initialized")
	}

	response, err := client.UnsubscribeFromTopic(ctx, tokens, topic)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe from topic: %w", err)
	}

	log.Printf("✅ Unsubscribed %d tokens from topic '%s' (%d failures)", len(tokens)-response.FailureCount, topic, response.FailureCount)
	return nil
}

// ValidateToken checks if a FCM token is valid
func (s *FirebaseService) ValidateToken(ctx context.Context, token string) bool {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil {
		return false
	}

	// Send a dry run message to validate the token
	message := &messaging.Message{
		Token: token,
		Data:  map[string]string{"validate": "true"},
	}

	_, err := client.SendDryRun(ctx, message)
	return err == nil
}

// Helper function to convert struct to map[string]string for FCM data
func StructToStringMap(v interface{}) (map[string]string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for k, v := range m {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result, nil
}
