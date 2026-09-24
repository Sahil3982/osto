# Osto Secure CLI: System Guide

This guide explains the project in plain language so you can discuss it confidently in an interview.

## 1. What does the system do?

Osto is an interactive command-line login application. A user can register, log in, enable or disable TOTP-based two-factor authentication, inspect their account, and log out. PostgreSQL stores the account data, Docker runs both the database and the CLI, and Bubble Tea handles the terminal interface.

The application starts in an unauthenticated command mode. After a successful login it switches to an authenticated command mode. A session has an expiration time; after that time the user is logged out automatically.

## 2. Main components

```text
cmd/osto/main.go
  Loads environment configuration, connects to PostgreSQL, runs the migration, and starts Bubble Tea.

internal/db
  Embeds and applies the SQL schema at startup.

internal/auth
  Owns registration, password hashing, login, lockout, TOTP, and user data.

internal/cli
  Owns terminal prompts, screen states, command history, completion, session display, and user feedback.

migrations
  The schema is kept with the database package because Go's embed directive cannot safely reach outside its package.
```

The code is intentionally split so authentication can be tested independently from terminal rendering. The CLI calls the service; it does not contain SQL or password hashing logic.

## 3. Registration flow

1. The user enters `register`, optionally followed by a username.
2. Bubble Tea asks for the username if it was not supplied.
3. Bubble Tea switches the text input to password mode, so the password is not displayed.
4. The auth service validates the username and password length.
5. The password is hashed with bcrypt. The plaintext password is never written to PostgreSQL.
6. PostgreSQL inserts the username, hash, registration timestamp, and empty MFA state.

Usernames are matched case-insensitively through a unique index on `LOWER(username)`.

## 4. Login flow

1. Bubble Tea collects the username and masked password.
2. The auth service starts a PostgreSQL transaction and locks the user row with `FOR UPDATE`.
3. The password is checked with bcrypt.
4. If MFA is enabled and no code was supplied, the service returns `ErrMFARequired`; the CLI then displays a masked TOTP prompt.
5. The TOTP code is checked with the `pquerna/otp` package.
6. A successful login resets failed attempts, clears any old lock, stores `last_login_at`, and returns user details.
7. The CLI creates a session expiration time using the configured timeout.

Unknown usernames and wrong passwords return the same public error so the application does not reveal whether an account exists.

## 5. Lockout design

Password failures and invalid TOTP codes use the same counter. The failed-attempt update happens inside the same transaction as the row lock, which prevents concurrent login attempts from overwriting each other's counters.

When the threshold is reached:

- `locked_until` is set to the current time plus the configured lockout duration.
- The user receives a generic locked response.
- Further attempts fail until the lock expires.

The threshold and duration are configurable with `LOCKOUT_THRESHOLD` and `LOCKOUT_DURATION`.

## 6. MFA design

`enable-2fa` generates a secret and an `otpauth://` URI compatible with Google Authenticator and similar applications. The URI is shown once so the user can add it to an authenticator. The secret is stored in PostgreSQL and is never included in `whoami` output.

`disable-2fa` clears the stored secret. In production, PostgreSQL should use encryption at rest and the connection should use TLS. A production version should also require a current TOTP code before disabling MFA.

## 7. Session and CLI design

Bubble Tea sends keyboard events to one model. The model has explicit states for command mode, username prompts, masked password prompts, TOTP prompts, and authenticated mode.

The CLI supports:

- `help` for the available commands
- Up/down arrows for in-memory history
- Tab completion for commands
- `whoami` for username, registration time, MFA state, session expiration, and last login
- `logout` and `exit`

History is capped at 100 entries. Passwords and TOTP codes are entered in separate prompt states and are not added to history. Database operations have a 10-second context timeout so a stalled database cannot hold a terminal operation forever.

## 8. PostgreSQL and Docker

The Compose file defines two services:

- `db`: PostgreSQL 17 with a health check and a named `postgres_data` volume
- `app`: the compiled Go CLI, started only after PostgreSQL is healthy

The named volume keeps data after containers restart. The database port is published for local development. The default credentials are for local demonstration only and must be replaced for a real deployment.

The Dockerfile uses a multi-stage build. The first stage compiles a static binary; the final Alpine image contains only the binary and a non-root user.

## 9. How to explain scalability

For one CLI process, the design is already separated into a UI, service, and database layer. PostgreSQL connection pooling is handled by `pgxpool`, so multiple operations can reuse database connections.

To scale this into a larger product, I would:

1. Move the auth service behind an HTTP or gRPC API while keeping the same service methods.
2. Add a real versioned migration runner instead of applying one embedded schema on startup.
3. Encrypt TOTP secrets or store them in a secrets manager.
4. Add structured logging, metrics, audit events, and tracing.
5. Add integration tests against PostgreSQL, including concurrent lockout tests.
6. Add rate limiting at the API boundary and use TLS everywhere.
7. Replace the in-memory CLI session with a server-side session store if multiple clients need shared sessions.

## 10. Interview questions and short answers

### Why bcrypt?
Bcrypt is deliberately expensive and includes a salt, which makes offline password cracking harder than storing plaintext, unsalted hashes, or fast hashes such as SHA-256. The application verifies passwords with bcrypt rather than decrypting them.

### Why PostgreSQL instead of a file database?
PostgreSQL provides transactions, row locking, durable persistence, concurrent access, and a natural path to production deployment. The Docker volume preserves its data across restarts.

### Why use a transaction during login?
The lockout counter is shared mutable state. The transaction plus `FOR UPDATE` makes reading the counter, deciding whether to lock, and writing the new value one serialized operation.

### Does enabling MFA replace the password?
No. MFA is an additional factor. Login still requires the password and then a current six-digit TOTP code.

### What happens if the database is unavailable?
Startup fails with a clear database error instead of starting an unusable CLI. Individual operations have bounded contexts and return an error if PostgreSQL stops responding.

### Why is the password not part of the command syntax?
Command-line arguments can be visible in terminal history, process inspection, logs, or shell tooling. The application asks for the password in a masked Bubble Tea input and excludes it from command history.

### What are the current security limitations?
The demo credentials are intentionally simple, TOTP secrets are stored directly in PostgreSQL, MFA enrollment is not confirmed with a code, and the CLI is a single interactive process. Production deployment should address those points with TLS, secret management, MFA confirmation, and an API/session architecture.

### How would you test the lockout behavior?
Use an isolated PostgreSQL test database, register a user, perform the configured number of wrong password and TOTP attempts, assert that the account becomes locked, then advance time or use a short lockout duration and assert that a valid login succeeds after expiry. Add a concurrent test to verify that multiple failures cannot overwrite the counter.
