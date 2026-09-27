// =============================================================================
// Bu dosya ne ise yarar? DestinationProfile / backup mode cozumleme testleri.
// =============================================================================

package backup_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
)

func TestResolveDestination(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = backupv1.AddToScheme(scheme)

	profile := &backupv1.DestinationProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-s3", Namespace: "demo"},
		Spec: backupv1.DestinationProfileSpec{
			ObjectStorageSpec: backupv1.ObjectStorageSpec{
				Endpoint:             "s3.example:9000",
				Bucket:               "backups",
				CredentialsSecretRef: "s3-creds",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile).Build()

	got, err := backup.ResolveDestination(context.Background(), c, "demo", nil, "prod-s3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoint != "s3.example:9000" {
		t.Fatalf("%+v", got)
	}

	inline := &backupv1.ObjectStorageSpec{Endpoint: "inline:9000", Bucket: "b", CredentialsSecretRef: "c"}
	got, err = backup.ResolveDestination(context.Background(), c, "demo", inline, "")
	if err != nil || got.Endpoint != "inline:9000" {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := backup.ResolveDestination(context.Background(), c, "demo", inline, "prod-s3"); err == nil {
		t.Fatal("expected xor error")
	}
}

func TestResolveBackupMode(t *testing.T) {
	s := &backupv1.BackupSchedule{Spec: backupv1.BackupScheduleSpec{Mode: backupv1.BackupModeIncremental}}
	mode, parent := backup.ResolveBackupMode(s)
	if mode != backupv1.BackupModeFull || parent != "" {
		t.Fatalf("promote expected, got %s %q", mode, parent)
	}
	s.Status.LastFullObjectKey = "demo/x/full.dump.gz"
	mode, parent = backup.ResolveBackupMode(s)
	if mode != backupv1.BackupModeIncremental || parent != "demo/x/full.dump.gz" {
		t.Fatalf("incr expected, got %s %q", mode, parent)
	}
}
