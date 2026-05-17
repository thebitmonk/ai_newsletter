create table publications (
    id uuid primary key default gen_random_uuid(),
    account_id uuid not null references accounts(id) on delete cascade,
    name text not null,
    brief text not null default '',
    timezone text not null,
    cadence_rule text,
    stories_per_issue_min int not null default 3 check (stories_per_issue_min >= 1),
    stories_per_issue_max int not null default 7 check (stories_per_issue_max >= stories_per_issue_min and stories_per_issue_max <= 20),
    intro_enabled boolean not null default true,
    curation_lead_time interval not null default '24 hours',
    approval_gate_enabled boolean not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index publications_account_id_idx on publications(account_id);
create index publications_account_created_idx on publications(account_id, created_at desc, id desc);
