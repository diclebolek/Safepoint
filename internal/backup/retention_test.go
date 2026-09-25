package backup_test

import (
	"testing"
	"time"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
	"github.com/diclebolek/Safepoint/internal/storage"
)

func TestObjectKey(t *testing.T) {
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	got := backup.ObjectKey("default", "shop-db-backup", "postgres", "dump.gz", at)
	want := "default/shop-db-backup/postgres-20260925T120000Z.dump.gz"
	if got != want {
		t.Fatalf("ObjectKey = %q, want %q", got, want)
	}
}

func TestApplyRetention(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	objects := []storage.ObjectInfo{
		{Key: "old.dump", LastModified: now.Add(-10 * 24 * time.Hour)},
		{Key: "new.dump", LastModified: now.Add(-1 * 24 * time.Hour)},
	}

	got := backup.ApplyRetention(objects, 7, now)
	if len(got) != 1 || got[0] != "old.dump" {
		t.Fatalf("ApplyRetention = %#v, want [old.dump]", got)
	}
}

func TestValidateCron(t *testing.T) {
	if err := backup.ValidateCron(""); err == nil {
		t.Fatal("expected error for empty cron")
	}
	if err := backup.ValidateCron("0 * * * *"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistryEngines(t *testing.T) {
	reg := backup.NewRegistry()
	for _, name := range []backupv1.DatabaseEngine{
		backupv1.EnginePostgres,
		backupv1.EngineMySQL,
		backupv1.EngineRedis,
		backupv1.EngineMongoDB,
	} {
		if _, err := reg.Get(name); err != nil {
			t.Fatalf("engine %s missing: %v", name, err)
		}
	}
}

func TestPostgresValidateAndBuildJob(t *testing.T) {
	eng := backup.PostgresEngine{}
	if err := eng.Validate(backup.Target{Host: "db", Username: "u", Database: "d"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Validate(backup.Target{}); err == nil {
		t.Fatal("expected validation error")
	}

	schedule := &backupv1.BackupSchedule{}
	schedule.Name = "shop-db-backup"
	schedule.Namespace = "default"
	schedule.Spec.Engine = backupv1.EnginePostgres

	job, err := eng.BuildJob(backup.JobRequest{
		Schedule:  schedule,
		Target:    backup.Target{Host: "db", Port: "5432", Username: "u", Password: "p", Database: "d"},
		ObjectKey: "default/shop/postgres-test.dump.gz",
		Storage: backup.StorageEnv{
			Endpoint:  "minio:9000",
			Bucket:    "db-backups",
			AccessKey: "ak",
			SecretKey: "sk",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Spec.Template.Spec.Containers[0].Image != "postgres:16-alpine" {
		t.Fatalf("unexpected image %s", job.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestRedisValidate(t *testing.T) {
	eng := backup.RedisEngine{}
	if err := eng.Validate(backup.Target{Host: "redis"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Validate(backup.Target{}); err == nil {
		t.Fatal("expected validation error")
	}
}
