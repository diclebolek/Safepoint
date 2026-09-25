## Safepoint Helm Chart

```powershell
# Build images first
docker build -t backup-operator:dev .
docker build -t safepoint-backup:dev -f Dockerfile.backup .
docker build -t safepoint-dashboard:dev -f Dockerfile.dashboard .

kubectl create namespace backup-system --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/crd/bases/
.\scripts\gen-webhook-certs.ps1

helm upgrade --install safepoint charts/safepoint -n backup-system
```

Dashboard:

```powershell
kubectl -n backup-system port-forward svc/safepoint-dashboard 8088:8088
# open http://127.0.0.1:8088
```
