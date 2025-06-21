-- name: CreateChirp :one
INSERT INTO chirps (body, user_id)
VALUES ($1, $2)
RETURNING id, created_at, updated_at, body, user_id;


-- name: GetChirps :many
SELECT id, created_at, updated_at, body, user_id FROM CHIRPS
ORDER BY created_at asc;

-- name: GetChirpByID :one
SELECT id, created_at, updated_at, body, user_id FROM CHIRPS
WHERE id = $1;