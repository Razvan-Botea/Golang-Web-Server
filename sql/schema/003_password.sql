-- +goose Up

alter table users
add column hashed_password TEXT default 'unset' NOT NULL;

-- +goose Down
alter table users
drop column hashed_password;