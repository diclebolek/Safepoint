package runner_test

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
	"github.com/diclebolek/Safepoint/internal/runner"
)

func TestCreateJobBuildsPostgresJob(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = backupv1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	schedule := &backupv1.BackupSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-db-backup", Namespace: "demo"},
		Spec: backupv1.BackupScheduleSpec{
			Engine:      backupv1.EnginePostgres,
			DatabaseRef: "shop-postgres",
			SecretRef:   "db-secret",
			Schedule:    "0 * * * *",
			Destination: backupv1.ObjectStorageSpec{
				Endpoint:             "minio:9000",
				Bucket:               "db-backups",
				CredentialsSecretRef: "minio-secret",
			},
		},
	}

	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-secret", Namespace: "demo"},
		Data: map[string][]byte{
			"host":     []byte("shop-postgres"),
			"port":     []byte("5432"),
			"username": []byte("postgres"),
			"password": []byte("secret"),
			"database": []byte("shop"),
		},
	}
	minioSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "minio-secret", Namespace: "demo"},
		Data: map[string][]byte{
			"accessKey": []byte("minioadmin"),
			"secretKey": []byte("minioadmin"),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(schedule, dbSecret, minioSecret).Build()
	r := &runner.JobRunner{
		Client:   c,
		Scheme:   scheme,
		Registry: backup.NewRegistry(),
		Clock:    func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
	}

	res, err := r.CreateJob(context.Background(), schedule)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != runner.JobRunning {
		t.Fatalf("outcome=%s", res.Outcome)
	}
	if res.JobName == "" || res.ObjectKey == "" {
		t.Fatalf("empty job/object: %#v", res)
	}

	var job batchv1.Job
	key := types.NamespacedName{Namespace: schedule.Namespace, Name: res.JobName}
	if err := c.Get(context.Background(), key, &job); err != nil {
		t.Fatal(err)
	}
	if job.Spec.Template.Spec.Containers[0].Image != "postgres:16-alpine" {
		t.Fatalf("image=%s", job.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestGetJobSucceeded(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-1",
			Namespace: "demo",
			Annotations: map[string]string{
				backup.AnnotationObjectKey: "obj.dump.gz",
			},
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &runner.JobRunner{Client: c, Scheme: scheme, Registry: backup.NewRegistry()}

	res, err := r.GetJob(context.Background(), "demo", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != runner.JobSucceeded || res.ObjectKey != "obj.dump.gz" {
		t.Fatalf("unexpected %#v", res)
	}
}
