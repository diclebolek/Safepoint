# Demo visuals (LinkedIn / CV)

Capture these after the demo stack is running — attach to README or LinkedIn post.

## 1) Schedules succeeded

```powershell
kubectl -n demo get bks
kubectl -n demo get jobs --sort-by=.metadata.creationTimestamp | Select-Object -Last 8
```

Screenshot: terminal with `PHASE Succeeded` and `lastObjectKey`.

## 2) Grafana

```powershell
kubectl -n demo port-forward svc/grafana 3000:3000
```

Open http://127.0.0.1:3000/d/safepoint-backups — screenshot Postgres/Redis success counters.

## 3) Optional: metrics raw

```powershell
kubectl -n backup-system port-forward svc/backup-operator-metrics 8080:8080
# browser or curl http://127.0.0.1:8080/metrics | findstr safepoint_
```

## Suggested LinkedIn caption (TR)

Safepoint: Kubernetes uzerinde Go ile yazdigim backup operator.  
CRD ile yedek politikasi, admission webhook ile zorunlu kural, Postgres/Redis/MySQL/Mongo, S3'e dump, restore CRD, Prometheus/Grafana.  
Repo: https://github.com/diclebolek/Safepoint
