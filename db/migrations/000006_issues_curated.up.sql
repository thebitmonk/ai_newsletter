alter table issues
    add column subject text,
    add column title text,
    add column cover_url text,
    add column body_doc jsonb,
    add column body_doc_version int not null default 1,
    add column failed_reason text;
