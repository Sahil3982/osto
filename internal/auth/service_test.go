package auth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHashIsNotPlaintext(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if string(hash) == password {
		t.Fatal("password was stored as plaintext")
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		t.Fatal("hash did not verify")
	}
}

func TestConfigDurations(t *testing.T) {
	config := Config{SessionTimeout: 30 * time.Minute, LockoutDuration: 15 * time.Minute}
	if config.SessionTimeout <= 0 || config.LockoutDuration <= 0 {
		t.Fatal("durations must be positive")
	}
}
