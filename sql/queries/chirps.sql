-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid(), NOW(), NOW(), $1, $2
)
RETURNING *;

-- name: GetChirps :many
Select * 
from chirps
order by created_at;

-- name: GetChirpsByAuthor :many
Select *
from chirps
where user_id = $1
order by created_at;

-- name: GetSingleChirp :one
select *
from chirps
where id = $1;

-- name: DeleteChirps :exec
DELETE FROM chirps;

-- name: DeleteChirp :exec
delete from chirps
where id = $1 and user_id = $2;