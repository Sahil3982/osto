package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAccountLocked      = errors.New("account is temporarily locked")
	ErrUserExists         = errors.New("username is already registered")
	ErrInvalidTOTP        = errors.New("invalid verification code")
	ErrMFARequired        = errors.New("MFA verification required")
)

type Config struct {
	SessionTimeout   time.Duration
	LockoutThreshold int
	LockoutDuration  time.Duration
}

type User struct {
	ID           int64
	Username     string
	RegisteredAt time.Time
	LastLoginAt  *time.Time
	MFAEnabled   bool
}

type Service struct {
	pool   *pgxpool.Pool
	config Config
}

func NewService(pool *pgxpool.Pool, config Config) *Service {
	if config.SessionTimeout <= 0 {
		config.SessionTimeout = 30 * time.Minute
	}
	if config.LockoutThreshold <= 0 {
		config.LockoutThreshold = 5
	}
	if config.LockoutDuration <= 0 {
		config.LockoutDuration = 15 * time.Minute
	}
	return &Service{pool: pool, config: config}
}

func (s *Service) Register(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 32 || strings.ContainsAny(username, " \t\n") {
		return errors.New("username must be 3-32 characters without spaces")
	}
	if len(password) < 10 {
		return errors.New("password must be at least 10 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO users (username, password_hash) VALUES ($1, $2)`, username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "users_username_idx") || strings.Contains(err.Error(), "duplicate key") {
			return ErrUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *Service) Login(ctx context.Context, username, password, code string) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("start login transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var user User
	var hash string
	var secret *string
	var failed int
	var lockedUntil *time.Time
	err = tx.QueryRow(ctx, `SELECT id, username, password_hash, totp_secret, failed_attempts, locked_until, registered_at, last_login_at FROM users WHERE LOWER(username) = LOWER($1) FOR UPDATE`, strings.TrimSpace(username)).Scan(&user.ID, &user.Username, &hash, &secret, &failed, &lockedUntil, &user.RegisteredAt, &user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	now := time.Now()
	if lockedUntil != nil && lockedUntil.After(now) {
		return User{}, ErrAccountLocked
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		locked, updateErr := s.recordFailure(ctx, tx, user.ID, failed, now)
		if updateErr != nil {
			return User{}, updateErr
		}
		if locked {
			return User{}, ErrAccountLocked
		}
		return User{}, ErrInvalidCredentials
	}
	if secret != nil {
		if code == "" {
			return User{}, ErrMFARequired
		}
		if !totp.Validate(code, *secret) {
			locked, updateErr := s.recordFailure(ctx, tx, user.ID, failed, now)
			if updateErr != nil {
				return User{}, updateErr
			}
			if locked {
				return User{}, ErrAccountLocked
			}
			return User{}, ErrInvalidTOTP
		}
	}
	_, err = tx.Exec(ctx, `UPDATE users SET failed_attempts = 0, locked_until = NULL, last_login_at = NOW() WHERE id = $1`, user.ID)
	if err != nil {
		return User{}, fmt.Errorf("record login: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit login: %w", err)
	}
	user.LastLoginAt = &now
	user.MFAEnabled = secret != nil
	return user, nil
}

func (s *Service) recordFailure(ctx context.Context, tx pgx.Tx, userID int64, failed int, now time.Time) (bool, error) {
	newFailed := failed + 1
	var newLock *time.Time
	if newFailed >= s.config.LockoutThreshold {
		t := now.Add(s.config.LockoutDuration)
		newLock = &t
		newFailed = 0
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET failed_attempts = $1, locked_until = $2 WHERE id = $3`, newFailed, newLock, userID); err != nil {
		return false, fmt.Errorf("record failed login: %w", err)
	}
	return newLock != nil, nil
}

func (s *Service) EnableMFA(ctx context.Context, userID int64, username string) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Osto CLI", AccountName: username})
	if err != nil {
		return "", fmt.Errorf("generate MFA secret: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET totp_secret = $1 WHERE id = $2`, key.Secret(), userID); err != nil {
		return "", fmt.Errorf("enable MFA: %w", err)
	}
	return key.URL(), nil
}

func (s *Service) DisableMFA(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET totp_secret = NULL WHERE id = $1`, userID)
	return err
}
