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

-- name: UpdateUser :one
UPDATE users
SET email = $1, hashed_password = $2
WHERE id = $3
RETURNING *;
