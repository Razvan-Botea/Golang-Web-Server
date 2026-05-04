-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(), NOW(), NOW(), $1, $2
)
RETURNING *;

-- name: DeleteUsers :exec
DELETE FROM USERS;

-- name: GetUserByEmail :one
select * 
from users
where email = $1;