package runner

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
)

// JobOutcome is the observed state of the active backup Job.
type JobOutcome string

const (
	JobNone      JobOutcome = "None"
	JobRunning   JobOutcome = "Running"
	JobSucceeded JobOutcome = "Succeeded"
	JobFailed    JobOutcome = "Failed"
)

// Result is returned by CreateJob / GetJob.
type Result struct {
	Outcome   JobOutcome
	JobName   string
	ObjectKey string
	Message   string
}

// JobRunner creates and observes per-schedule backup Jobs.
type JobRunner struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Registry *backup.Registry
	Clock    func() time.Time
}

func (r *JobRunner) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now().UTC()
}

// GetJob reports the status of an existing backup Job.
func (r *JobRunner) GetJob(ctx context.Context, namespace, name string) (Result, error) {
	var job batchv1.Job
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			return Result{Outcome: JobNone, Message: "job not found"}, nil
		}
		return Result{}, fmt.Errorf("get job %s: %w", name, err)
	}
	return inspectJob(&job), nil
}

// CreateJob builds and creates a new backup Job for the schedule.
func (r *JobRunner) CreateJob(ctx context.Context, schedule *backupv1.BackupSchedule) (Result, error) {
	backup.SetScheme(r.Scheme)

	engineName := schedule.EffectiveEngine()
	engine, err := r.Registry.Get(engineName)
	if err != nil {
		return Result{}, err
	}

	dbSecret, err := readSecret(ctx, r.Client, schedule.Namespace, schedule.Spec.SecretRef)
	if err != nil {
		return Result{}, err
	}
	target := backup.ParseTargetSecret(dbSecret)
	if err := engine.Validate(target); err != nil {
		return Result{}, err
	}

	storeSecret, err := readSecret(ctx, r.Client, schedule.Namespace, schedule.Spec.Destination.CredentialsSecretRef)
	if err != nil {
		return Result{}, err
	}
	accessKey, secretKey, err := backup.ParseStorageSecret(storeSecret)
	if err != nil {
		return Result{}, err
	}

	region := schedule.Spec.Destination.Region
	if region == "" {
		region = "us-east-1"
	}

	objectKey := backup.ObjectKey(
		schedule.Namespace,
		schedule.Name,
		string(engineName),
		engine.FileExtension(),
		r.now(),
	)

	job, err := engine.BuildJob(backup.JobRequest{
		Schedule:  schedule,
		Target:    target,
		ObjectKey: objectKey,
		Storage: backup.StorageEnv{
			Endpoint:  schedule.Spec.Destination.Endpoint,
			Bucket:    schedule.Spec.Destination.Bucket,
			Prefix:    schedule.Spec.Destination.Prefix,
			Region:    region,
			AccessKey: accessKey,
			SecretKey: secretKey,
			UseSSL:    schedule.Spec.Destination.UseSSL,
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("build job: %w", err)
	}

	if err := r.Client.Create(ctx, job); err != nil {
		return Result{}, fmt.Errorf("create job: %w", err)
	}

	return Result{
		Outcome:   JobRunning,
		JobName:   job.Name,
		ObjectKey: objectKey,
		Message:   "backup job created",
	}, nil
}

func inspectJob(job *batchv1.Job) Result {
	objectKey := job.Annotations[backup.AnnotationObjectKey]
	if job.Status.Succeeded > 0 {
		return Result{
			Outcome:   JobSucceeded,
			JobName:   job.Name,
			ObjectKey: objectKey,
			Message:   "backup job succeeded",
		}
	}
	if job.Status.Failed > 0 {
		return Result{
			Outcome:   JobFailed,
			JobName:   job.Name,
			ObjectKey: objectKey,
			Message:   "backup job failed",
		}
	}
	return Result{
		Outcome:   JobRunning,
		JobName:   job.Name,
		ObjectKey: objectKey,
		Message:   "backup job running",
	}
}

func readSecret(ctx context.Context, c client.Client, namespace, name string) (map[string][]byte, error) {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	return secret.Data, nil
}
