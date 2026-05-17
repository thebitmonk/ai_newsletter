create table sources (
    id uuid primary key default gen_random_uuid(),
    publication_id uuid not null references publications(id) on delete cascade,
    type text not null check (type in ('rss', 'youtube_channel', 'x_handle', 'substack', 'web')),
    identifier text not null,
    poll_interval interval not null,
    enabled boolean not null default true,
    last_polled_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index sources_publication_id_idx on sources(publication_id);
create unique index sources_publication_identifier_uniq on sources(publication_id, type, identifier);
