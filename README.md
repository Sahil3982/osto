# Osto Secure CLI

A containerized command-line login system built with Go, Bubble Tea, PostgreSQL, bcrypt, and TOTP MFA.

## Run with Docker

```sh
docker compose up --build
```

The Postgres data is stored in the `postgres_data` named volume, so it survives container restarts. To run the CLI interactively:

```sh
docker compose run --rm app
```

For local development, start only PostgreSQL with `docker compose up -d db`, then run `go run ./cmd/osto` with `DATABASE_URL` set to the localhost URL in `.env.example`.

## Commands

Before login:

- `register [username]` creates an account. The password is entered in a masked prompt and must be at least 10 characters.
- `login [username]` authenticates a user. The password and, when enabled, the TOTP code are entered in masked prompts.
- `help` lists commands.
- `exit` quits.

After login:

- `whoami` displays username, registration date, MFA status, session expiration, and last login.
- `enable-2fa` generates a Google Authenticator-compatible `otpauth://` URI.
- `disable-2fa` disables MFA.
- `logout` ends the session.

Up/down arrows browse non-sensitive command history and Tab completes commands. Account lockout occurs after five failed password or MFA attempts for 15 minutes. Sessions expire after 30 minutes by default; `SESSION_TIMEOUT`, `LOCKOUT_THRESHOLD`, and `LOCKOUT_DURATION` can override these values.

## Security notes

Passwords are hashed with bcrypt. Passwords and TOTP codes are masked in the CLI and are not saved in command history. TOTP secrets are stored in PostgreSQL and should be encrypted at rest in production. Set a strong database password and use TLS for non-local deployments. The demo defaults are intended for local Docker use only.

## Tests

```sh
go test ./...
```
