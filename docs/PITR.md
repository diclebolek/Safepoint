# Point-in-Time Recovery (PITR) with Safepoint

## What Safepoint does today

1. **Logical dump** (`pg_dump` / `mysqldump` / …) → object storage  
2. **Manifest JSON** next to each object, including for Postgres:
   - `mode`, `objectKey`, `parentObjectKey`
   - `walLsn` = `pg_current_wal_lsn()` at dump start  
3. **Incremental mode** chains dumps via `parentObjectKey` / `lastFullObjectKey`

That is enough to restore to a **backup timestamp** (the dump itself).

## What “true PITR” still needs (ops side)

To restore to an arbitrary second (e.g. 14:37), Postgres also needs:

1. `archive_mode = on` and `archive_command` shipping WAL to S3 (or `pg_receivewal`)  
2. A base backup (or dump) + continuous WAL segments  
3. `recovery_target_time` / `recovery_target_lsn` on restore  

Safepoint records **`walLsn` in the manifest** so you can align dump + WAL tooling later. Full continuous WAL shipping is cluster/DBA configuration (not a single CRD field).

## How to inspect the LSN after a demo backup

```powershell
# last object
kubectl -n demo get bks shop-db-backup -o jsonpath="{.status.lastObjectKey}{'\n'}"
# manifest key is "<objectKey>.manifest.json" in the same bucket
```

## LinkedIn / CV wording

> Logical backups to S3 with schedule/restore CRDs; Postgres dumps capture WAL LSN in sidecar manifests to support a future PITR pipeline.
