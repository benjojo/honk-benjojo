
create table honks (honkid integer primary key, userid integer, what text, honker text, xid text, rid text, dt text, url text, audience text, noise text, convoy text, whofore integer, format text, precis text, oonker text);
create table donks (honkid integer, fileid integer);
create table files(fileid integer primary key, xid text, name text, url text, media text, local integer, content blob);
create table honkers (honkerid integer primary key, userid integer, name text, xid text, flavor text, combos text);
create table xonkers (xonkerid integer primary key, name text, info text, flavor text);
create table zonkers (zonkerid integer primary key, userid integer, name text, wherefore text);
create table doovers(dooverid integer primary key, dt text, tries integer, username text, rcpt text, msg blob);

create index idx_honksxid on honks(xid);
create index idx_honksconvoy on honks(convoy);
create index idx_honkshonker on honks(honker);
create index idx_honksoonker on honks(oonker);
create index idx_honkerxid on honkers(xid);
create index idx_xonkername on xonkers(name);
create index idx_zonkersname on zonkers(name);
create index idx_filesxid on files(xid);
create index idx_filesurl on files(url);

create table config (key text, value text);

create table users (userid integer primary key, username text, hash text, displayname text, about text, pubkey text, seckey text, options text);
create table auth (authid integer primary key, userid integer, hash text);
CREATE index idxusers_username on users(username);
CREATE index idxauth_userid on auth(userid);
CREATE index idxauth_hash on auth(hash);

