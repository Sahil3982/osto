# Osto Secure CLI

A containerized command-line login system built with Go, Bubble Tea, PostgreSQL, bcrypt, and TOTP MFA.

## Project Context

This project is part of my hands-on Go systems work. A related project is [Vigil](https://github.com/Sahil3982/vigil), a lightweight cross-platform terminal monitor for CPU, memory, disk, and command profiling.

The projects share a focus on small, practical command-line tools, but solve different problems: Vigil focuses on system observability, while Osto focuses on authentication, persistence, and secure session handling.

The Osto implementation uses a separate authentication service, PostgreSQL transactions, row locking for account lockout, masked terminal prompts, Docker persistence, and a simple Bubble Tea interface.

## Quick Start with Docker

Use two terminals or run these commands in the same PowerShell window:

```powershell
cd C:\Users\Dell\Desktop\osto
docker compose up -d db
docker compose run --rm --build app
```

The first command starts PostgreSQL in the background. The second command rebuilds the app image and starts the interactive Bubble Tea CLI, leaving the terminal attached so you can type commands. Wait until the database is healthy before running the app.

When the CLI displays `>`, try it with:

```text
help
register
login
```

Passwords and MFA codes are entered through masked prompts. Press `Ctrl+C` to exit the CLI.

The PostgreSQL data is stored in the `postgres_data` named volume, so it survives container restarts. To stop the database when finished:

```powershell
docker compose down
```

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

