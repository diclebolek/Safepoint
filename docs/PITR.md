# Point-in-Time Recovery (PITR) with Safepoint

## What Safepoint does today

1. **Logical dump** (`pg_dump` / `mysqldump` / …) → object storage  
2. **Manifest JSON** next to each object, including for Postgres:
   - `mode`, `objectKey`, `parentObjectKey`
   - `walLsn` = `pg_current_wal_lsn()` at dump start  
3. **Incremental mode** chains dumps via `parentObjectKey` / `lastFullObjectKey`  
4. **Continuous WAL shipping** (demo): `pg_receivewal` DaemonSet uploads segments to the same DestinationProfile bucket

That is enough to restore to a **backup timestamp** (the dump itself). With WAL segments under `wal/<database>/` you can also aim at a later LSN/time using Postgres recovery settings.

## Demo: WAL shipping → DestinationProfile

```powershell
kubectl apply -f config/demo/destinationprofile.yaml
kubectl apply -f config/demo/postgres.yaml          # wal_level=replica + replicator role
kubectl apply -f config/demo/wal-shipping.yaml      # DaemonSet: pg_receivewal + mc mirror

kubectl -n demo get ds safepoint-wal-shipping
kubectl -n demo logs -l app=safepoint-wal-shipping -c pg-receivewal --tail=50
kubectl -n demo logs -l app=safepoint-wal-shipping -c s3-uploader --tail=50
```

Wiring:

| Piece | Role |
|--------|------|
| `DestinationProfile/demo-minio` | Shared S3 endpoint + bucket (`db-backups`) |
| Annotation `backup.goproject.io/destination-profile` | Documents which profile the DaemonSet targets |
| ConfigMap `safepoint-wal-shipping` | Endpoint/bucket/prefix (`wal/shop-postgres`) |
| Secret `minio-credentials` | Same keys as the DestinationProfile |
| Secret `shop-postgres-replicator` | Replication login for `pg_receivewal` |

## Restore to a dump (logical)

```powershell
$key = kubectl -n demo get bks shop-db-backup -o jsonpath="{.status.lastObjectKey}"
(Get-Content config\demo\backuprestore.yaml) -replace 'REPLACE_WITH_LAST_OBJECT_KEY',$key | kubectl apply -f -
kubectl -n demo get bkr shop-db-restore -w
```

## True PITR (ops checklist)

To restore to an arbitrary second (e.g. 14:37):

1. Base backup or logical dump whose manifest has `walLsn`  
2. Continuous WAL under `s3://db-backups/wal/shop-postgres/` (this DaemonSet)  
3. Restore base, configure `restore_command` / `recovery_target_time` (or `recovery_target_lsn`) on a recovery Postgres  

Safepoint owns the **policy CRDs + shipping path**; the final `recovery.conf` / `postgresql.auto.conf` step remains a DBA/runbook action (see Postgres docs).

## Inspect LSN after a demo backup

```powershell
kubectl -n demo get bks shop-db-backup -o jsonpath="{.status.lastObjectKey}{'\n'}"
# manifest key is "<objectKey>.manifest.json" in the same bucket
```

## LinkedIn / CV wording

> Logical backups to S3 with schedule/restore CRDs; Postgres dumps capture WAL LSN in manifests; continuous `pg_receivewal` shipping lands WAL segments on the same DestinationProfile for PITR-ready ops.
