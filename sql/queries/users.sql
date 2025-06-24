-- name: CreateUser :one
INSERT INTO users (email, hashed_password)
VALUES (
    $1, $2
)
RETURNING *;

-- name: DeleteAllUsers :exec
DELETE FROM users;

-- name: FindUserByEmail :one
SELECT * FROM users
WHERE LOWER(email) = LOWER($1);
