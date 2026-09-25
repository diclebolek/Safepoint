.PHONY: tidy build test test-race vet run docker-build demo webhook-certs deploy gencerts ci

tidy:
	go mod tidy

build:
	go build -o bin/manager.exe ./cmd

vet:
	go vet ./...

test:
	go test ./... -count=1

ci: vet test build gencerts

# -race needs a working C toolchain (works in GitHub Actions / Linux).
test-race:
	go test ./... -count=1 -race

run:
	go run ./cmd --enable-webhooks=false

docker-build:
	docker build -t backup-operator:dev .

install-crd:
	kubectl apply -f config/crd/bases/backup.goproject.io_backupschedules.yaml

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

deploy: install-crd install-rbac webhook-certs docker-build
	kubectl apply -f config/webhook/service.yaml
	kubectl apply -f config/manager/deployment.yaml

sample:
	kubectl apply -f config/samples/secrets.yaml
	kubectl apply -f config/samples/backupschedule.yaml
