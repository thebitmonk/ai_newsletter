create table issues (
    id uuid primary key default gen_random_uuid(),
    publication_id uuid not null references publications(id) on delete cascade,
    state text not null check (state in (
        'planned', 'curating', 'drafted', 'approved',
        'sending', 'sent', 'failed', 'skipped'
    )),
    scheduled_at timestamptz not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index issues_publication_state_scheduled_idx
    on issues(publication_id, state, scheduled_at);

create unique index issues_publication_scheduled_uniq
    on issues(publication_id, scheduled_at);
