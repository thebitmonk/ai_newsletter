alter table users drop column if exists email_verified;
alter table users drop column if exists firebase_uid;

alter table users add column password_hash text;
-- Existing rows will have NULL password_hash; the down migration is best-effort.
update users set password_hash = '!invalid' where password_hash is null;
alter table users alter column password_hash set not null;

alter table users alter column email set not null;
alter table users add constraint users_email_key unique (email);

create table sessions (
    token_hash bytea primary key,
    user_id uuid not null references users(id) on delete cascade,
    account_id uuid not null references accounts(id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);
create index sessions_user_id_idx on sessions(user_id);
create index sessions_expires_at_idx on sessions(expires_at);
