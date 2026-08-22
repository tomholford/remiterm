-- Baseline schema (version 1). Pragmas are applied via the DSN, not here.

create table db_version (
    version integer primary key
) strict, without rowid;
insert into db_version values(1);

create table messages (
    id text primary key,
    created_at integer not null,
    payload text not null
) strict, without rowid;
create index idx_messages_created_at on messages(created_at);

create table profiles (
    handle text not null collate nocase,
    fetched_at integer not null,
    payload text not null,
    primary key (handle)
) strict, without rowid;
