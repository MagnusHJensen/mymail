package main

import (
	"encoding/json"
	"log/slog"
	"os"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username       string `json:"username"` // TODO: Define a shared format
	HashedPassword string `json:"hashed_password"`
}

func (u *User) IsCorrectPassword(password string) bool {
	if err := bcrypt.CompareHashAndPassword([]byte(u.HashedPassword), []byte(password)); err != nil {
		return false
	}

	return true
}

//TODO: This is a naive user impl. should update to use database.

type authService struct {
	logger *slog.Logger
	users  map[string]User
}

func NewAuthService(logger *slog.Logger, usersFile string) *authService {
	svc := &authService{
		logger: logger,
	}

	svc.users = loadUsers(usersFile)
	return svc
}

func (svc *authService) AuthUser(username, password string) *User {
	user, ok := svc.users[username]
	if !ok {
		// TODO: Timing attack
		return nil
	}

	if user.IsCorrectPassword(password) {
		return &user
	}

	// TODO: Timing attack
	return nil
}

// Returns map of username to user object
func loadUsers(usersFile string) map[string]User {
	fileData, err := os.ReadFile(usersFile)
	if err != nil {
		panic(err)
	}

	var users []User
	if err := json.Unmarshal(fileData, &users); err != nil {
		panic(err)
	}

	var userMap = make(map[string]User)
	for _, user := range users {
		userMap[user.Username] = user
	}

	return userMap
}
