package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("UserRegistration", UserRegistration)
}

type User struct {
	ID       string `json:"id,omitempty"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
	Name     string `json:"name"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
)

func (u *User) Validate() error {
	if !emailRegex.MatchString(u.Email) {
		return errors.New("invalid email format")
	}

	if len(u.Password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	if !containsUppercase(u.Password) || !containsLowercase(u.Password) || !containsNumber(u.Password) {
		return errors.New("password must contain at least one uppercase letter, one lowercase letter, and one number")
	}

	if strings.TrimSpace(u.Name) == "" {
		return errors.New("name cannot be empty")
	}

	return nil
}

func containsUppercase(s string) bool {
	return strings.ToLower(s) != s
}

func containsLowercase(s string) bool {
	return strings.ToUpper(s) != s
}

func containsNumber(s string) bool {
	for _, char := range s {
		if char >= '0' && char <= '9' {
			return true
		}
	}
	return false
}

func RegisterUser(ctx context.Context, user *User) (string, error) {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return "", err
	}

	client, err := app.Auth(ctx)
	if err != nil {
		return "", err
	}

	params := (&auth.UserToCreate{}).
		Email(user.Email).
		Password(user.Password).
		DisplayName(user.Name)

	firebaseUser, err := client.CreateUser(ctx, params)
	if err != nil {
		return "", err
	}

	return firebaseUser.UID, nil
}

func UserRegistration(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := user.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID, err := RegisterUser(r.Context(), &user)
	if err != nil {
		http.Error(w, "Failed to register user", http.StatusInternalServerError)
		return
	}

	response := UserResponse{
		ID:    userID,
		Email: user.Email,
		Name:  user.Name,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
