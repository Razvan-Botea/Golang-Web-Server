-- name: CreateRefreshToken :one
insert into refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
values (
    $1, NOW(), NOW(), $2, $3, NULL
)
RETURNING *;

-- name: GetUserFromRefreshToken :one
select users.* from users
join refresh_tokens ON users.id = refresh_tokens.user_id
where refresh_tokens.token = $1
and revoked_at IS NULL
and expires_at > NOW();

-- name: RevokeRefreshToken :exec
update refresh_tokens
set revoked_at = NOW(),
    updated_at = NOW()
where token = $1;