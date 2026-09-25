//go:build envtest

package controller_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/controller"
	"github.com/diclebolek/Safepoint/internal/runner"
)

// TestEnvtestBackupScheduleSmoke boots an API server via envtest (CI / Linux).
func TestEnvtestBackupScheduleSmoke(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Skipf("envtest unavailable: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = backupv1.AddToScheme(s)

	k8s, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: s})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&controller.BackupScheduleReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Jobs: &stubJobs{
			createFn: func(context.Context, *backupv1.BackupSchedule) (runner.Result, error) {
				return runner.Result{Outcome: runner.JobRunning, JobName: "j1", ObjectKey: "k", Message: "created"}, nil
			},
		},
		Clock: func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
	}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mgr.Start(ctx) }()

	ns := "default"
	obj := &backupv1.BackupSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "envtest-bks", Namespace: ns},
		Spec: backupv1.BackupScheduleSpec{
			Engine:      backupv1.EnginePostgres,
			DatabaseRef: "db",
			SecretRef:   "sec",
			Schedule:    "0 * * * *",
			Destination: backupv1.ObjectStorageSpec{
				Endpoint: "minio:9000", Bucket: "b", CredentialsSecretRef: "s3",
			},
		},
	}
	if err := k8s.Create(ctx, obj); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var got backupv1.BackupSchedule
		if err := k8s.Get(ctx, types.NamespacedName{Name: obj.Name, Namespace: ns}, &got); err == nil {
			if len(got.Finalizers) > 0 || got.Status.Phase != "" {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("timed out waiting for reconcile side effects")
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
