package userauthentication

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	firebase "firebase.google.com/go/v4"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("UserAuthentication", UserAuthentication)
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	IDToken string `json:"idToken"`
	User    User   `json:"user"`
}

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type FirebaseSignInResponse struct {
	IDToken      string `json:"idToken"`
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
	LocalID      string `json:"localId"`
	DisplayName  string `json:"displayName"`
}

func UserAuthentication(w http.ResponseWriter, r *http.Request) {
	var loginReq LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Step 1: Authenticate with Firebase
	firebaseResp, err := authenticateWithFirebase(loginReq.Email, loginReq.Password)
	if err != nil {
		http.Error(w, "Authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Step 2: Verify the ID token
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		http.Error(w, "Failed to initialize Firebase app", http.StatusInternalServerError)
		return
	}

	client, err := app.Auth(ctx)
	if err != nil {
		http.Error(w, "Failed to get Firebase Auth client", http.StatusInternalServerError)
		return
	}

	token, err := client.VerifyIDToken(ctx, firebaseResp.IDToken)
	if err != nil {
		http.Error(w, "Invalid ID token", http.StatusUnauthorized)
		return
	}

	response := LoginResponse{
		IDToken: firebaseResp.IDToken,
		User: User{
			ID:    token.UID,
			Email: firebaseResp.Email,
			Name:  firebaseResp.DisplayName,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func authenticateWithFirebase(email, password string) (*FirebaseSignInResponse, error) {
	apiKey := os.Getenv("FIREBASE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("FIREBASE_API_KEY environment variable is not set")
	}

	url := fmt.Sprintf("https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s", apiKey)

	requestBody, _ := json.Marshal(map[string]interface{}{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentication failed: %s", body)
	}

	var firebaseResp FirebaseSignInResponse
	if err := json.Unmarshal(body, &firebaseResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &firebaseResp, nil
}
