# Mini CyberArk (Go + MongoDB)

Secure-ish credential demo that mimics a tiny slice of a PAM flow: you can create credentials, retrieve the current password, and the system rotates it in the background 10 seconds after retrieval.

> Note: This is a learning/demo project. It does not provide production-grade security. See the Security notes and Roadmap below for what’s missing.

## Features

- REST API with two endpoints:
  - POST `/create` — store a username/password pair
  - GET `/retrieve/{username}` — return the current password immediately and schedule a rotation 10 seconds later
- MongoDB backend with a unique index on `username`
- Timestamps on rotation (`last_rotated` in Asia/Kolkata timezone)

## Tech Stack

- Language: Go
- DB: MongoDB (local `mongodb://localhost:27017` by default)
- Driver: `go.mongodb.org/mongo-driver`

## Project structure

```
miniCyberArk/
├─ main.go       # HTTP server, MongoDB init, handlers
├─ go.mod        # module + dependencies
└─ README.md     # this file
```

## Prerequisites

- Go 1.21+
- A running MongoDB (local or container)

On Windows you can use Docker to run MongoDB locally:

```powershell
docker run --name mongo -p 27017:27017 -d mongo:6
```

## Setup and Run

```powershell
# From the project directory
go mod tidy
go run .
# Server starts on http://localhost:8080
```

The code uses these defaults (change in `main.go` if you like):

- Mongo URI: `mongodb://localhost:27017`
- Database: `miniCyberarkVault`
- Collection: `credentials`

## API

### Create credential

- Method: POST
- URL: `/create`
- Body (application/json):

```json
{
  "username": "alice",
  "password": "Initial@123"
}
```

- Responses:
  - 201 Created

```json
{
  "username": "alice",
  "password": "Initial@123",
  "message": "Credential created successfully"
}
```

    - 409 Conflict (duplicate username)

```text
Username already exists
```

### Retrieve (and rotate after 10s)

- Method: GET
- URL: `/retrieve/{username}`

- Response (200 OK): returns the CURRENT password immediately, then rotates it in the background after 10 seconds.

```json
{
  "username": "alice",
  "password": "Initial@123",
  "retrieved_time": "2025-11-01 12:00:00 IST"
}
```

Rotation is logged by the server and updates the document fields:

```json
{
  "password": "<new-random-12-char>",
  "last_rotated": "2025-11-01T12:00:10+05:30"
}
```

If user not found:

```text
Credential not found
```

## Quick test with curl (Windows PowerShell)

```powershell
# Create
curl -X POST http://localhost:8080/create `
	-H "Content-Type: application/json" `
	-d '{"username":"alice","password":"Initial@123"}'

# Retrieve (returns current password now; rotates in 10s)
curl http://localhost:8080/retrieve/alice
```

## Security notes (important)

This is intentionally minimal and not production-ready:

- Passwords are stored in plaintext in MongoDB. Use encryption-at-rest (e.g., AES-GCM) or store only brokered/ephemeral secrets.
- No authentication/authorization around the API.
- No auditing of who retrieved what and why.
- No rate limiting, approvals, dual control, or session recording.
- Rotation policy is hardcoded (10 seconds) and in-process (lost if the process is stopped during the wait).

## Roadmap / ideas

- Add API auth (e.g., JWT or mTLS) and RBAC.
- Add audit logs collection (who/when/why) with immutable storage.
- Encrypt secrets at rest with a key from env/KMS; rotate data keys.
- Config via environment variables (Mongo URI, DB/collection, server port, rotation delay).
- Make rotation durable (persist a job with ETA and run a background scheduler).
- Add tests and CI.

## License

MIT (or your preferred license)
