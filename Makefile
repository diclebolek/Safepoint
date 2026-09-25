.PHONY: tidy build test vet run docker-build docker-build-all demo webhook-certs deploy gencerts ci dashboard

tidy:
	go mod tidy

build:
	go build -o bin/manager.exe ./cmd
	go build -o bin/dashboard.exe ./cmd/dashboard

vet:
	go vet ./...

test:
	go test ./... -count=1

ci: vet test build gencerts

run:
	go run ./cmd --enable-webhooks=false

docker-build:
	docker build -t backup-operator:dev .

docker-build-all: docker-build
	docker build -t safepoint-backup:dev -f Dockerfile.backup .
	docker build -t safepoint-dashboard:dev -f Dockerfile.dashboard .

install-crd:
	kubectl apply -f config/crd/bases/

install-rbac:
	kubectl apply -f config/rbac/role.yaml

gencerts:
	go run ./scripts/gencerts -out config/webhook/certs

webhook-certs: gencerts
	powershell -ExecutionPolicy Bypass -File .\scripts\gen-webhook-certs.ps1

demo:
	kubectl apply -f config/demo/minio.yaml
	kubectl apply -f config/demo/backupschedules.yaml
	kubectl apply -f config/demo/postgres.yaml
	kubectl apply -f config/demo/redis.yaml

deploy: install-crd install-rbac webhook-certs docker-build-all
	kubectl apply -f config/webhook/service.yaml
	kubectl apply -f config/manager/deployment.yaml
	kubectl apply -f config/dashboard/deployment.yaml

dashboard:
	kubectl -n backup-system port-forward svc/safepoint-dashboard 8088:8088

sample:
	kubectl apply -f config/samples/secrets.yaml
	kubectl apply -f config/samples/backupschedule.yaml
