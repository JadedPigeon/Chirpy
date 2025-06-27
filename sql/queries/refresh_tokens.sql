-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (refresh_token, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: FindRefreshToken :one
SELECT * FROM refresh_tokens
WHERE refresh_token = $1;

-- name: RevokeRefreshToken :one
UPDATE refresh_tokens
SET updated_at = NOW(), revoked_at = NOW()
WHERE refresh_token = $1
RETURNING *;
