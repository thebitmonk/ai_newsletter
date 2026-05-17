create table candidates (
    id uuid primary key default gen_random_uuid(),
    publication_id uuid not null references publications(id) on delete cascade,
    source_id uuid not null references sources(id) on delete cascade,
    source_item_id text not null,
    url text not null,
    title text,
    raw_content jsonb not null,
    fetched_at timestamptz not null default now(),
    expires_at timestamptz not null,
    unique (publication_id, source_item_id)
);

create index candidates_publication_expires_idx
    on candidates(publication_id, expires_at);

create index candidates_source_id_idx on candidates(source_id);
