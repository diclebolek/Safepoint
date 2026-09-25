# Safepoint

**Kubernetes-native database backup operator with policy enforcement.**

Safepoint schedules, runs, and verifies backups for stateful workloads on Kubernetes — then blocks deployments that would run unprotected.

[![CI](https://github.com/diclebolek/Safepoint/actions/workflows/ci.yml/badge.svg)](https://github.com/diclebolek/Safepoint/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.27+-326CE5?logo=kubernetes)](https://kubernetes.io/)

**Repository:** https://github.com/diclebolek/Safepoint

---

## Table of contents

1. [What problem does this solve?](#what-problem-does-this-solve)
2. [What is Safepoint?](#what-is-safepoint)
3. [What is MinIO?](#what-is-minio)
4. [How the pieces fit together](#how-the-pieces-fit-together)
5. [Supported engines](#supported-engines)
6. [Tech stack — what and why](#tech-stack--what-and-why)
7. [Prerequisites](#prerequisites)
8. [Install Safepoint (full guide)](#install-safepoint-full-guide)
9. [Use Safepoint day to day](#use-safepoint-day-to-day)
10. [Demo stack (Postgres + Redis + MinIO)](#demo-stack-postgres--redis--minio)
11. [Custom Resource reference](#custom-resource-reference)
12. [Admission webhook](#admission-webhook)
13. [Security model](#security-model)
14. [Project layout](#project-layout)
15. [Development & CI](#development--ci)
16. [Roadmap](#roadmap)
17. [CV / talking points](#cv--talking-points)
18. [License](#license)

---

## What problem does this solve?

In real clusters, databases run inside Pods. Backups are often forgotten scripts, copy-pasted CronJobs, or missing until the first outage.

Safepoint turns backup policy into **Kubernetes API objects**. You declare *what* to back up and *when*; the operator runs Jobs, uploads artifacts, updates status, and can **refuse** unprotected workloads via an admission webhook.

**One sentence:** *declare backup policy as YAML, enforce it like any other Kubernetes resource.*

---

## What is Safepoint?

| Concept | Plain meaning |
|--------|----------------|
| **Kubernetes** | Runs containers (Pods) across machines |
| **Operator** | A controller that watches API objects and keeps the cluster matching your desired state |
| **CRD / `BackupSchedule`** | Your backup policy as a first-class Kubernetes resource |
| **Job** | A one-shot Pod that runs a dump and exits |
| **MinIO** | Where backup files are stored (S3-compatible) |
| **Admission webhook** | Gatekeeper: “May this Pod/Deployment be created?” |

---

## What is MinIO?

**MinIO** is free, open-source **object storage** that speaks the same API as **Amazon S3**.

Think of it as a hard drive in the cloud (or in your cluster) where files are stored as **objects** inside **buckets** — not as traditional database rows.

| Term | Meaning |
|------|---------|
| **Bucket** | A named folder for objects (e.g. `db-backups`) |
| **Object / key** | A file path inside the bucket (e.g. `demo/shop-db-backup/postgres-....dump.gz`) |
| **accessKey / secretKey** | Username/password for the S3 API |

**Why Safepoint uses MinIO**

- Local/dev: no AWS bill
- Same code path as production S3 (`minio-go` / AWS SDK style)
- Demo manifests under `config/demo/minio.yaml` start MinIO for you

After a successful backup you can list objects:

```powershell
kubectl -n demo exec deploy/minio -- mc alias set local http://127.0.0.1:9000 minioadmin minioadmin
kubectl -n demo exec deploy/minio -- mc ls local/db-backups --recursive
```

Or open the MinIO console (port-forward):

```powershell
kubectl -n demo port-forward svc/minio 9001:9001
# browser → http://127.0.0.1:9001  (minioadmin / minioadmin in demo)
```

> For production you can point `destination.endpoint` at real AWS S3 / GCS / Cloudflare R2 — same fields, different endpoint and credentials.

---

## How the pieces fit together

```text
You apply BackupSchedule (YAML)
        │
        ▼
Safepoint Operator (reconcile loop)
        │  when cron is due
        ▼
Kubernetes Job  →  pg_dump / redis-cli / …  →  gzip  →  upload to MinIO
        │
        ▼
BackupSchedule.status  (Succeeded / Failed, lastObjectKey, nextBackupTime)

Optional:
Pod/Deployment with require-backup=true
        │
        ▼
Validating webhook  →  deny if no active BackupSchedule
```

---

## Supported engines

| Engine | Tool | Artifact |
|--------|------|----------|
| `postgres` (default) | `pg_dump` | `.dump.gz` |
| `mysql` | `mysqldump` | `.sql.gz` |
| `redis` | `redis-cli --rdb` | `.rdb.gz` |
| `mongodb` | `mongodump --archive --gzip` | `.archive.gz` |

---

## Tech stack — what and why

| Technology | Why |
|------------|-----|
| **Go** | Kubernetes operator standard; static binary |
| **controller-runtime** | Manager, clients, webhooks (Kubebuilder stack) |
| **CRD `BackupSchedule`** | GitOps-friendly API |
| **batch/v1 Job** | Async dumps with backoff / TTL |
| **robfig/cron** | Schedule parsing |
| **MinIO + minio-go** | Free S3-compatible storage |
| **gzip** | Smaller backups |
| **Validating webhook** | Fail-closed policy (`failurePolicy: Fail`) |
| **Docker / distroless** | Small non-root operator image |
| **GitHub Actions** | CI on every push |

---

## Prerequisites

1. **Go 1.22+** — https://go.dev/dl/
2. **Docker Desktop** — https://www.docker.com/products/docker-desktop/
3. **Enable Kubernetes in Docker Desktop**
   - Settings → **Kubernetes** → ☑ Enable Kubernetes → Apply & Restart
4. **kubectl** (ships with Docker Desktop)

Check:

```powershell
docker version
kubectl get nodes
```

If `kubectl get nodes` fails, Kubernetes is not enabled yet.

---

## Install Safepoint (full guide)

Do these steps **in order** from the repo root.

### 1) Clone

```powershell
git clone https://github.com/diclebolek/Safepoint.git
cd Safepoint
```

### 2) Build the operator image

```powershell
go test ./... -count=1
docker build -t backup-operator:dev .
```

Docker Desktop Kubernetes will use this local image tag (`imagePullPolicy: IfNotPresent`).

### 3) Install CRD + RBAC

```powershell
kubectl apply -f config/crd/bases/backup.goproject.io_backupschedules.yaml
kubectl create namespace backup-system --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/rbac/role.yaml
```

### 4) Generate webhook TLS and apply webhook config

```powershell
.\scripts\gen-webhook-certs.ps1
```

This creates `config/webhook/certs/`, a Secret `webhook-server-cert` in `backup-system`, and patches the `ValidatingWebhookConfiguration` `caBundle`.

### 5) Deploy the operator

```powershell
kubectl apply -f config/webhook/service.yaml
kubectl apply -f config/manager/deployment.yaml
kubectl -n backup-system get pods -w
```

Wait until the pod is `Running` / Ready.

### 6) Deploy object storage + sample schedules + databases

The demo Service is still named `minio`, but the image is **SeaweedFS** (S3-compatible). Use this when MinIO images cannot be pulled.

**Order matters** (webhook is fail-closed): schedules **before** protected DBs.

```powershell
kubectl apply -f config/demo/minio.yaml
kubectl apply -f config/demo/backupschedules.yaml
kubectl apply -f config/demo/postgres.yaml
kubectl apply -f config/demo/redis.yaml

# Create the S3 bucket once
kubectl -n demo exec deploy/minio -- sh -c "echo 's3.bucket.create -name db-backups' | weed shell -master=localhost:9333"
```

### 7) Verify

```powershell
kubectl -n backup-system get pods
kubectl -n demo get pods
kubectl -n demo get bks
kubectl -n demo get jobs
kubectl -n demo describe bks shop-db-backup
```

Demo schedules run every few minutes. When a Job succeeds, `status.phase` becomes `Succeeded` and `lastObjectKey` shows the uploaded object.

### One-liner via Makefile (after image build)

```powershell
make deploy    # CRD, RBAC, certs, operator
make demo      # storage + schedules + Postgres + Redis
```

### Local operator without webhook (dev only)

```powershell
go run ./cmd --enable-webhooks=false
```

Use this while iterating on reconcile logic. For admission tests, use the in-cluster Deployment.

---

## Use Safepoint day to day

### Create your own backup policy

1. Put DB credentials in a Secret (`host`, `port`, `username`, `password`, `database`).
2. Put MinIO/S3 credentials in a Secret (`accessKey`, `secretKey`).
3. Apply a `BackupSchedule`:

```yaml
apiVersion: backup.goproject.io/v1
kind: BackupSchedule
metadata:
  name: my-app-db
  namespace: default
spec:
  engine: postgres
  databaseRef: my-postgres
  secretRef: my-postgres-credentials
  schedule: "0 */6 * * *"      # every 6 hours UTC
  retentionDays: 7
  destination:
    endpoint: minio.demo.svc.cluster.local:9000
    bucket: db-backups
    prefix: postgres/my-app
    credentialsSecretRef: minio-credentials
    useSSL: false
```

```powershell
kubectl apply -f my-schedule.yaml
kubectl get bks -A
kubectl describe backupschedule my-app-db
```

### Pause backups

```yaml
spec:
  suspend: true
```

### Protect a workload with the webhook

```yaml
metadata:
  labels:
    backup.goproject.io/require-backup: "true"
  annotations:
    backup.goproject.io/database-name: my-postgres   # must match spec.databaseRef
```

If no **active** (non-suspended) `BackupSchedule` exists for that `databaseRef` in the same namespace, create/update is **denied**.

### Uninstall

```powershell
kubectl delete -f config/demo/ --ignore-not-found
kubectl delete -f config/manager/deployment.yaml --ignore-not-found
kubectl delete -f config/webhook/ --ignore-not-found
kubectl delete -f config/rbac/role.yaml --ignore-not-found
kubectl delete -f config/crd/bases/backup.goproject.io_backupschedules.yaml --ignore-not-found
kubectl delete ns demo backup-system --ignore-not-found
```

---

## Demo stack (Postgres + Redis + MinIO)

| Resource | Namespace | Purpose |
|----------|-----------|---------|
| MinIO Deployment/Service | `demo` | S3-compatible store + `db-backups` bucket Job |
| `shop-db-backup` | `demo` | Postgres schedule every 5 minutes |
| `shop-redis-backup` | `demo` | Redis schedule every 10 minutes |
| `shop-postgres` / `shop-redis` | `demo` | Sample DBs labeled for admission |

**Negative test:** remove schedules, re-apply `config/demo/postgres.yaml` → Deployment should be rejected.

---

## Custom Resource reference

```yaml
apiVersion: backup.goproject.io/v1
kind: BackupSchedule
metadata:
  name: shop-db-backup
  namespace: demo
spec:
  engine: postgres              # postgres | mysql | redis | mongodb
  databaseRef: shop-postgres
  secretRef: shop-postgres-credentials
  schedule: "0 */6 * * *"
  retentionDays: 7
  suspend: false
  destination:
    endpoint: minio.demo.svc.cluster.local:9000
    bucket: db-backups
    prefix: postgres/shop
    region: us-east-1
    credentialsSecretRef: minio-credentials
    useSSL: false
status:
  phase: Succeeded              # Pending | Running | Succeeded | Failed
  lastBackupTime: ...
  nextBackupTime: ...
  lastObjectKey: ...
  lastJobName: ...
  message: backup job succeeded
```

### Secret keys

| Secret | Keys |
|--------|------|
| Database | `host`, `port`, `username`, `password`, `database` (Redis: often `host`/`port`/`password`) |
| MinIO / S3 | `accessKey`, `secretKey` |

Never commit real production credentials.

---

## Admission webhook

- Paths: `/validate-v1-pod`, `/validate-v1-deployment`
- `failurePolicy: Fail` (fail-closed)
- TLS via `scripts/gencerts` + `scripts/gen-webhook-certs.ps1`

---

## Security model

- Credentials only in Secrets (not logged)
- Operator: non-root, read-only root FS, dropped capabilities
- Least-privilege RBAC
- Webhook TLS with Service SAN
- Distroless manager image

---

## Project layout

```text
Safepoint/
├── api/v1/                 # CRD types
├── cmd/main.go             # Manager entrypoint
├── internal/
│   ├── controller/         # Reconcile loop
│   ├── runner/             # Job create / observe
│   ├── backup/             # Engines (postgres, mysql, redis, mongodb)
│   ├── storage/            # MinIO client
│   └── webhook/            # Admission handlers
├── config/
│   ├── crd/ rbac/ manager/ webhook/
│   ├── demo/               # MinIO + DBs + schedules
│   └── samples/
├── scripts/gencerts/       # TLS generator
├── .github/workflows/ci.yml
├── Dockerfile
├── Makefile
└── README.md
```

---

## Restore

```powershell
# copy lastObjectKey from a successful BackupSchedule
kubectl -n demo get bks shop-db-backup -o jsonpath="{.status.lastObjectKey}{\"\n\"}"
# edit config/samples/backuprestore.yaml then:
kubectl apply -f config/crd/bases/backup.goproject.io_backuprestores.yaml
kubectl apply -f config/samples/backuprestore.yaml
kubectl -n demo get bkr -w
```

## Helm

See [`charts/safepoint/README.md`](charts/safepoint/README.md).

## Hardened backup image

```powershell
docker build -t safepoint-backup:dev -f Dockerfile.backup .
```

Jobs default to this image (`safepoint-backup:dev`) so dump tools are preinstalled.

## Metrics

Operator exposes Prometheus metrics on `:8080/metrics`:

- `safepoint_backup_success_total`
- `safepoint_backup_failure_total`
- `safepoint_backup_duration_seconds`
- `safepoint_restore_success_total`
- `safepoint_restore_failure_total`

Grafana dashboard JSON: `charts/safepoint/dashboards/safepoint.json`.

## Development & CI

```powershell
go mod tidy
go vet ./...
go test ./... -count=1
go build -o bin/manager.exe ./cmd
go run ./scripts/gencerts -out config/webhook/certs
```

GitHub Actions (`.github/workflows/ci.yml`): vet, test (`-race`), envtest, build, Helm lint, docker build.

> On some Windows setups `go test -race` needs a C toolchain; use `go test ./...` locally. CI (Ubuntu) runs `-race`.

---

## Roadmap

- [x] CRD + reconcile  
- [x] Job dump → gzip → MinIO  
- [x] Engines: Postgres, MySQL, Redis, MongoDB  
- [x] Validating webhook + demo stack  
- [x] Unit tests + GitHub Actions  
- [x] envtest suite (`-tags=envtest`, CI)  
- [x] Hardened backup image (`Dockerfile.backup`)  
- [x] Prometheus metrics + Grafana dashboard  
- [x] Helm chart (`charts/safepoint`)  
- [x] Restore CRD (`BackupRestore`)  

---

## CV / talking points

> *Safepoint — Go Kubernetes operator that schedules database backups (Postgres/Redis/…) to S3-compatible storage and enforces backup policy via validating admission webhooks.*

1. Operator reconcile loop, status, finalizers  
2. Multi-engine `Engine` interface  
3. Async Job pattern  
4. MinIO / S3 destination  
5. Fail-closed admission webhook  
6. RBAC, non-root image, CI  

---

## License

MIT — see [LICENSE](LICENSE).
