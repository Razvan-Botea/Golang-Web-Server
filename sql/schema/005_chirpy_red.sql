-- +goose Up
alter table users
add column is_chirpy_red boolean default false not null;

-- +goose Down
alter table users
drop colimn is_chirpy_red;
