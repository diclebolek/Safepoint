package controller_test

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/controller"
	"github.com/diclebolek/Safepoint/internal/runner"
)

type stubJobs struct {
	createFn func(context.Context, *backupv1.BackupSchedule) (runner.Result, error)
	getFn    func(context.Context, string, string) (runner.Result, error)
}

func (s *stubJobs) CreateJob(ctx context.Context, schedule *backupv1.BackupSchedule) (runner.Result, error) {
	if s.createFn != nil {
		return s.createFn(ctx, schedule)
	}
	return runner.Result{Outcome: runner.JobRunning, JobName: "job-1", ObjectKey: "k", Message: "created"}, nil
}

func (s *stubJobs) GetJob(ctx context.Context, namespace, name string) (runner.Result, error) {
	if s.getFn != nil {
		return s.getFn(ctx, namespace, name)
	}
	return runner.Result{Outcome: runner.JobRunning, JobName: name, Message: "running"}, nil
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := backupv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func baseSchedule() *backupv1.BackupSchedule {
	return &backupv1.BackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "shop-db-backup",
			Namespace:  "demo",
			Generation: 1,
		},
		Spec: backupv1.BackupScheduleSpec{
			Engine:        backupv1.EnginePostgres,
			DatabaseRef:   "shop-postgres",
			SecretRef:     "db-secret",
			Schedule:      "0 * * * *",
			RetentionDays: 7,
			Destination: backupv1.ObjectStorageSpec{
				Endpoint:             "minio:9000",
				Bucket:               "db-backups",
				CredentialsSecretRef: "minio-credentials",
			},
		},
	}
}

func TestReconcileAddsFinalizer(t *testing.T) {
	scheme := newScheme(t)
	s := baseSchedule()
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(s).WithObjects(s).Build()

	r := &controller.BackupScheduleReconciler{
		Client: c,
		Scheme: scheme,
		Jobs:   &stubJobs{},
		Clock:  func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got backupv1.BackupSchedule
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(s), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range got.Finalizers {
		if f == "backup.goproject.io/finalizer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected finalizer, got %#v", got.Finalizers)
	}
}

func TestReconcileSuspend(t *testing.T) {
	scheme := newScheme(t)
	s := baseSchedule()
	s.Finalizers = []string{"backup.goproject.io/finalizer"}
	s.Spec.Suspend = true
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(s).WithObjects(s).Build()

	r := &controller.BackupScheduleReconciler{
		Client: c,
		Scheme: scheme,
		Jobs:   &stubJobs{},
		Clock:  func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got backupv1.BackupSchedule
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(s), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != backupv1.BackupPhasePending || got.Status.Message != "suspended" {
		t.Fatalf("unexpected status: phase=%s msg=%s", got.Status.Phase, got.Status.Message)
	}
}

func TestReconcileCreatesJobWhenDue(t *testing.T) {
	scheme := newScheme(t)
	s := baseSchedule()
	s.Finalizers = []string{"backup.goproject.io/finalizer"}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(s).WithObjects(s).Build()

	created := false
	r := &controller.BackupScheduleReconciler{
		Client: c,
		Scheme: scheme,
		Clock:  func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
		Jobs: &stubJobs{
			createFn: func(context.Context, *backupv1.BackupSchedule) (runner.Result, error) {
				created = true
				return runner.Result{
					Outcome:   runner.JobRunning,
					JobName:   "shop-db-backup-postgres-1",
					ObjectKey: "demo/shop-db-backup/postgres-x.dump.gz",
					Message:   "backup job created",
				}, nil
			},
		},
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected CreateJob to be called")
	}
	if res.RequeueAfter != 10*time.Second {
		t.Fatalf("expected requeue after 10s, got %v", res.RequeueAfter)
	}

	var got backupv1.BackupSchedule
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(s), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != backupv1.BackupPhaseRunning {
		t.Fatalf("phase=%s want Running", got.Status.Phase)
	}
	if got.Status.LastJobName != "shop-db-backup-postgres-1" {
		t.Fatalf("lastJob=%s", got.Status.LastJobName)
	}
}

func TestReconcileObservesSuccessfulJob(t *testing.T) {
	scheme := newScheme(t)
	s := baseSchedule()
	s.Finalizers = []string{"backup.goproject.io/finalizer"}
	s.Status.Phase = backupv1.BackupPhaseRunning
	s.Status.LastJobName = "job-ok"
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(s).WithObjects(s).Build()

	r := &controller.BackupScheduleReconciler{
		Client: c,
		Scheme: scheme,
		Clock:  func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
		Jobs: &stubJobs{
			getFn: func(context.Context, string, string) (runner.Result, error) {
				return runner.Result{
					Outcome:   runner.JobSucceeded,
					JobName:   "job-ok",
					ObjectKey: "obj",
					Message:   "backup job succeeded",
				}, nil
			},
		},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got backupv1.BackupSchedule
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(s), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != backupv1.BackupPhaseSuccess {
		t.Fatalf("phase=%s want Succeeded", got.Status.Phase)
	}
	if got.Status.NextBackupTime == nil {
		t.Fatal("expected NextBackupTime")
	}
}

func TestReconcileInvalidCron(t *testing.T) {
	scheme := newScheme(t)
	s := baseSchedule()
	s.Finalizers = []string{"backup.goproject.io/finalizer"}
	s.Spec.Schedule = "not-a-cron"
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(s).WithObjects(s).Build()

	r := &controller.BackupScheduleReconciler{
		Client: c,
		Scheme: scheme,
		Jobs:   &stubJobs{},
		Clock:  func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got backupv1.BackupSchedule
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(s), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != backupv1.BackupPhaseFailed {
		t.Fatalf("phase=%s want Failed", got.Status.Phase)
	}
}
