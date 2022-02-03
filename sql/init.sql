create table if not exists version (version integer primary key);

INSERT INTO version(version) SELECT 0 where 0 not in (select version from version where version=0);