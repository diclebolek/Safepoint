package webhook_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/webhook"
)

func TestHasActiveBackup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = backupv1.AddToScheme(scheme)

	schedule := &backupv1.BackupSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-db-backup", Namespace: "default"},
		Spec: backupv1.BackupScheduleSpec{
			DatabaseRef: "shop-postgres",
			Suspend:     false,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(schedule).Build()

	ok, err := webhook.HasActiveBackup(context.Background(), c, "default", "shop-postgres")
	if err != nil || !ok {
		t.Fatalf("expected active backup, ok=%v err=%v", ok, err)
	}

	ok, err = webhook.HasActiveBackup(context.Background(), c, "default", "other")
	if err != nil || ok {
		t.Fatalf("expected no backup for other, ok=%v err=%v", ok, err)
	}
}

func TestHasActiveBackupSuspended(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = backupv1.AddToScheme(scheme)

	schedule := &backupv1.BackupSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-db-backup", Namespace: "default"},
		Spec: backupv1.BackupScheduleSpec{
			DatabaseRef: "shop-postgres",
			Suspend:     true,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(schedule).Build()

	ok, err := webhook.HasActiveBackup(context.Background(), c, "default", "shop-postgres")
	if err != nil || ok {
		t.Fatalf("suspended schedule must not count, ok=%v err=%v", ok, err)
	}
}
