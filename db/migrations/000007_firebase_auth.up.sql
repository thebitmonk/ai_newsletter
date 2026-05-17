-- Swap the authentication identity backing for users from
-- (email + password_hash) to firebase_uid. The Account/User schema separation
-- from ADR-0013 is preserved; only the identity column on users changes per
-- ADR-0016.

-- Drop the unique constraint on email — Firebase may legitimately have
-- multiple identities sharing an email (e.g. password + Google login), and
-- the source of truth for uniqueness is now firebase_uid.
alter table users drop constraint users_email_key;

-- email becomes optional. Firebase guarantees a UID on every user but email
-- is only present when the user signs in with an email-based provider.
alter table users alter column email drop not null;

-- Sessions are owned by Firebase now; the backend never mints or stores them.
drop table sessions;

-- Drop the now-unused password column.
alter table users drop column password_hash;

-- New columns: firebase_uid (the identity column) and email_verified
-- (cached from claims on each authed request).
alter table users add column firebase_uid citext unique;
alter table users add column email_verified boolean not null default false;
