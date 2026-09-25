## Safepoint Helm Chart

```powershell
docker build -t backup-operator:dev .
docker build -t safepoint-backup:dev -f Dockerfile.backup .

kubectl create namespace backup-system --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/crd/bases/
.\scripts\gen-webhook-certs.ps1

helm upgrade --install safepoint charts/safepoint -n backup-system
```

Metrics: Service on port 8080 (`/metrics`). Grafana dashboard ConfigMap: `dashboards/safepoint.json`.
