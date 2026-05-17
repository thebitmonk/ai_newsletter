create extension if not exists citext;
create extension if not exists pgcrypto;

create table accounts (
    id uuid primary key default gen_random_uuid(),
    created_at timestamptz not null default now()
);

create table users (
    id uuid primary key default gen_random_uuid(),
    email citext not null unique,
    password_hash text not null,
    created_at timestamptz not null default now()
);

create table account_members (
    account_id uuid not null references accounts(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    role text not null default 'owner' check (role in ('owner')),
    created_at timestamptz not null default now(),
    primary key (account_id, user_id)
);
create index account_members_user_id_idx on account_members(user_id);

create table sessions (
    token_hash bytea primary key,
    user_id uuid not null references users(id) on delete cascade,
    account_id uuid not null references accounts(id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);
create index sessions_user_id_idx on sessions(user_id);
create index sessions_expires_at_idx on sessions(expires_at);
