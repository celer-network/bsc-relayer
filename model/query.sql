-- name: InitBBCStatus :exec
insert into bbc_status (
    network_id,height,bbc_vals_hash,bsc_vals_hash
) values (
    $1,$2,$3,$4
) on conflict do nothing;

-- name: GetBBCStatus :one
select * from bbc_status
where network_id = $1;

-- name: UpdateHeight :exec
update bbc_status
    set height = $2
where network_id = $1;

-- name: UpdateBBCValsHash :exec
update bbc_status
    set bbc_vals_hash = $2
where network_id = $1;

-- name: UpdateBSCValsHash :exec
update bbc_status
    set bsc_vals_hash = $2,
    stake_mod_seq = $3
where network_id = $1;

-- name: UpdateAfterSync :exec
update bbc_status
    set synced_at = $2
where network_id = $1;