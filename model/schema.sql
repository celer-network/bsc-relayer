CREATE TABLE if not exists bbc_status (
    network_id int primary key not null,
    height int not null default 0,
    bbc_vals_hash text not null default '',
    bsc_vals_hash text not null default '',
    stake_mod_seq int not null default 0,
    synced_at int not null default 0
)