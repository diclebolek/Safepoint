package backup_test

import (
	"testing"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
)

func TestBuildRestoreJobPostgres(t *testing.T) {
	restore := &backupv1.BackupRestore{}
	restore.Name = "shop-db-restore"
	restore.Namespace = "demo"
	restore.Spec.Engine = backupv1.EnginePostgres
	restore.Spec.ObjectKey = "demo/x.dump.gz"

	job, err := backup.BuildRestoreJob(backup.RestoreRequest{
		Restore:   restore,
		ObjectKey: restore.Spec.ObjectKey,
		Target:    backup.Target{Host: "db", Username: "u", Database: "d", Port: "5432"},
		Storage: backup.StorageEnv{
			Endpoint: "minio:9000", Bucket: "db-backups", AccessKey: "a", SecretKey: "s",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Spec.Template.Spec.Containers[0].Image != backup.DefaultBackupImage {
		t.Fatalf("image=%s", job.Spec.Template.Spec.Containers[0].Image)
	}
}
