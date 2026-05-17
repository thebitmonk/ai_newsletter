alter table issues
    drop column if exists failed_reason,
    drop column if exists body_doc_version,
    drop column if exists body_doc,
    drop column if exists cover_url,
    drop column if exists title,
    drop column if exists subject;
