package userprofilemanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("UserProfileManagement", UserProfileManagement)
}

type UserProfile struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type UserProfileUpdate struct {
	Name string `json:"name"`
}

func UserProfileManagement(w http.ResponseWriter, r *http.Request) {
	// Initialize Firebase app
	ctx := context.Background()
	config := &firebase.Config{
		ProjectID: "synapscribe",
	}
	app, err := firebase.NewApp(ctx, config)
	if err != nil {
		http.Error(w, "Error initializing app: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get Firebase Auth client
	authClient, err := app.Auth(ctx)
	if err != nil {
		http.Error(w, "Error getting Auth client: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify Firebase ID token
	idToken := extractToken(r)
	fmt.Println("Received token:", idToken)
	if idToken == "" {
		http.Error(w, "No token provided", http.StatusUnauthorized)
		return
	}

	token, err := authClient.VerifyIDTokenAndCheckRevoked(ctx, idToken)
	if err != nil {
		http.Error(w, "Invalid token: " + err.Error(), http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleGetProfile(w, r, authClient, token.UID)
	case http.MethodPut:
		handleUpdateProfile(w, r, authClient, token.UID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetProfile(w http.ResponseWriter, r *http.Request, authClient *auth.Client, uid string) {
	user, err := authClient.GetUser(r.Context(), uid)
	if err != nil {
		http.Error(w, "Error getting user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	profile := UserProfile{
		ID:        user.UID,
		Email:     user.Email,
		Name:      user.DisplayName,
		CreatedAt: time.Unix(user.UserMetadata.CreationTimestamp/1000, 0).Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func handleUpdateProfile(w http.ResponseWriter, r *http.Request, authClient *auth.Client, uid string) {
	var update UserProfileUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	params := (&auth.UserToUpdate{}).DisplayName(update.Name)
	user, err := authClient.UpdateUser(r.Context(), uid, params)
	if err != nil {
		http.Error(w, "Error updating user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	updatedProfile := UserProfile{
		ID:        user.UID,
		Email:     user.Email,
		Name:      user.DisplayName,
		CreatedAt: time.Unix(user.UserMetadata.CreationTimestamp/1000, 0).Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedProfile)
}

func extractToken(r *http.Request) string {
	// Extract token from Authorization header
	bearerToken := r.Header.Get("X-Forwarded-Authorization")
	if bearerToken != "" && strings.HasPrefix(bearerToken, "Bearer ") {
		return strings.TrimPrefix(bearerToken, "Bearer ")
	}
	return ""
}
