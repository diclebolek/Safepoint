package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
	"github.com/diclebolek/Safepoint/internal/runner"
	"github.com/diclebolek/Safepoint/internal/storage"
)

const finalizerName = "backup.goproject.io/finalizer"

// JobManager creates and observes backup Jobs.
type JobManager interface {
	CreateJob(ctx context.Context, schedule *backupv1.BackupSchedule) (runner.Result, error)
	GetJob(ctx context.Context, namespace, name string) (runner.Result, error)
}

// BackupScheduleReconciler reconciles BackupSchedule objects.
type BackupScheduleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Clock    func() time.Time
	Jobs     JobManager
	StoreFor func(ctx context.Context, schedule *backupv1.BackupSchedule) (storage.ObjectStore, error)
}

// Reconcile is the core control loop for BackupSchedule.
func (r *BackupScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var schedule backupv1.BackupSchedule
	if err := r.Get(ctx, req.NamespacedName, &schedule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get BackupSchedule: %w", err)
	}

	if !schedule.DeletionTimestamp.IsZero() {
		return r.handleDelete(ctx, &schedule)
	}

	if !controllerutil.ContainsFinalizer(&schedule, finalizerName) {
		controllerutil.AddFinalizer(&schedule, finalizerName)
		if err := r.Update(ctx, &schedule); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if schedule.Spec.Suspend {
		return r.patchStatus(ctx, &schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhasePending
			s.Status.Message = "suspended"
			s.Status.ObservedGeneration = s.Generation
		})
	}

	if err := backup.ValidateCron(schedule.Spec.Schedule); err != nil {
		_, statusErr := r.patchStatus(ctx, &schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhaseFailed
			s.Status.Message = err.Error()
			s.Status.ObservedGeneration = s.Generation
		})
		return ctrl.Result{}, statusErr
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	cronSched, err := parser.Parse(schedule.Spec.Schedule)
	if err != nil {
		_, statusErr := r.patchStatus(ctx, &schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhaseFailed
			s.Status.Message = fmt.Sprintf("invalid cron: %v", err)
			s.Status.ObservedGeneration = s.Generation
		})
		return ctrl.Result{}, statusErr
	}

	now := r.now()

	if schedule.Status.Phase == backupv1.BackupPhaseRunning && schedule.Status.LastJobName != "" {
		return r.observeRunningJob(ctx, &schedule, cronSched, now)
	}

	due := schedule.Status.NextBackupTime == nil || !schedule.Status.NextBackupTime.After(now)
	if !due {
		requeueAfter := schedule.Status.NextBackupTime.Sub(now)
		if requeueAfter < time.Second {
			requeueAfter = time.Second
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if r.Jobs == nil {
		return ctrl.Result{}, fmt.Errorf("job runner is not configured")
	}

	logger.Info("backup due", "schedule", schedule.Name, "engine", schedule.EffectiveEngine())

	result, err := r.Jobs.CreateJob(ctx, &schedule)
	if err != nil {
		logger.Error(err, "create backup job failed")
		_, statusErr := r.patchStatus(ctx, &schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhaseFailed
			s.Status.Message = err.Error()
			s.Status.LastBackupTime = &metav1.Time{Time: now}
			s.Status.NextBackupTime = &metav1.Time{Time: cronSched.Next(now)}
			s.Status.ObservedGeneration = s.Generation
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// CreateJob may return an already-existing Job's terminal state.
	if result.Outcome == runner.JobSucceeded || result.Outcome == runner.JobFailed {
		_, _ = r.patchStatus(ctx, &schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhaseRunning
			s.Status.LastJobName = result.JobName
			s.Status.LastObjectKey = result.ObjectKey
			s.Status.ObservedGeneration = s.Generation
		})
		if err := r.Get(ctx, req.NamespacedName, &schedule); err != nil {
			return ctrl.Result{}, err
		}
		return r.observeRunningJob(ctx, &schedule, cronSched, now)
	}

	_, statusErr := r.patchStatus(ctx, &schedule, func(s *backupv1.BackupSchedule) {
		s.Status.Phase = backupv1.BackupPhaseRunning
		s.Status.Message = result.Message
		s.Status.LastJobName = result.JobName
		s.Status.LastObjectKey = result.ObjectKey
		s.Status.ObservedGeneration = s.Generation
	})
	if statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *BackupScheduleReconciler) observeRunningJob(
	ctx context.Context,
	schedule *backupv1.BackupSchedule,
	cronSched cron.Schedule,
	now time.Time,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	result, err := r.Jobs.GetJob(ctx, schedule.Namespace, schedule.Status.LastJobName)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch result.Outcome {
	case runner.JobNone, runner.JobRunning:
		if result.Outcome == runner.JobNone {
			_, statusErr := r.patchStatus(ctx, schedule, func(s *backupv1.BackupSchedule) {
				s.Status.Phase = backupv1.BackupPhaseFailed
				s.Status.Message = "backup job disappeared before completion"
				s.Status.LastBackupTime = &metav1.Time{Time: now}
				s.Status.NextBackupTime = &metav1.Time{Time: cronSched.Next(now)}
				s.Status.ObservedGeneration = s.Generation
			})
			return ctrl.Result{RequeueAfter: time.Minute}, statusErr
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	case runner.JobSucceeded:
		logger.Info("backup succeeded", "job", result.JobName, "objectKey", result.ObjectKey)
		if err := r.applyRetention(ctx, schedule); err != nil {
			logger.Error(err, "retention cleanup failed")
		}
		next := cronSched.Next(now)
		_, statusErr := r.patchStatus(ctx, schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhaseSuccess
			s.Status.Message = result.Message
			s.Status.LastBackupTime = &metav1.Time{Time: now}
			s.Status.NextBackupTime = &metav1.Time{Time: next}
			s.Status.LastObjectKey = result.ObjectKey
			s.Status.LastJobName = result.JobName
			s.Status.ObservedGeneration = s.Generation
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		requeueAfter := next.Sub(now)
		if requeueAfter < time.Second {
			requeueAfter = time.Second
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil

	case runner.JobFailed:
		logger.Info("backup failed", "job", result.JobName)
		next := cronSched.Next(now)
		_, statusErr := r.patchStatus(ctx, schedule, func(s *backupv1.BackupSchedule) {
			s.Status.Phase = backupv1.BackupPhaseFailed
			s.Status.Message = result.Message
			s.Status.LastBackupTime = &metav1.Time{Time: now}
			s.Status.NextBackupTime = &metav1.Time{Time: next}
			s.Status.LastJobName = result.JobName
			s.Status.ObservedGeneration = s.Generation
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *BackupScheduleReconciler) handleDelete(ctx context.Context, schedule *backupv1.BackupSchedule) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(schedule, finalizerName) {
		controllerutil.RemoveFinalizer(schedule, finalizerName)
		if err := r.Update(ctx, schedule); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *BackupScheduleReconciler) applyRetention(ctx context.Context, schedule *backupv1.BackupSchedule) error {
	if r.StoreFor == nil || schedule.Spec.RetentionDays <= 0 {
		return nil
	}
	store, err := r.StoreFor(ctx, schedule)
	if err != nil {
		return err
	}
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}
	for _, key := range backup.ApplyRetention(objects, schedule.Spec.RetentionDays, r.now()) {
		if err := store.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (r *BackupScheduleReconciler) patchStatus(
	ctx context.Context,
	schedule *backupv1.BackupSchedule,
	mutate func(*backupv1.BackupSchedule),
) (ctrl.Result, error) {
	base := schedule.DeepCopy()
	mutate(schedule)
	if err := r.Status().Patch(ctx, schedule, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *BackupScheduleReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now().UTC()
}

// SetupWithManager registers the controller with the manager.
func (r *BackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.BackupSchedule{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// DefaultStoreFor builds a MinIO store from the schedule destination + secret.
func DefaultStoreFor(c client.Client) func(context.Context, *backupv1.BackupSchedule) (storage.ObjectStore, error) {
	return func(ctx context.Context, schedule *backupv1.BackupSchedule) (storage.ObjectStore, error) {
		data, err := ReadSecretData(ctx, c, schedule.Namespace, schedule.Spec.Destination.CredentialsSecretRef)
		if err != nil {
			return nil, err
		}
		accessKey, secretKey, err := backup.ParseStorageSecret(data)
		if err != nil {
			return nil, err
		}
		region := schedule.Spec.Destination.Region
		if region == "" {
			region = "us-east-1"
		}
		return storage.NewMinIOStore(storage.Config{
			Endpoint: schedule.Spec.Destination.Endpoint,
			Bucket:   schedule.Spec.Destination.Bucket,
			Prefix:   schedule.Spec.Destination.Prefix,
			Region:   region,
			UseSSL:   schedule.Spec.Destination.UseSSL,
			Creds: storage.Credentials{
				AccessKey: accessKey,
				SecretKey: secretKey,
			},
		})
	}
}

// ReadSecretData loads a Secret's data map.
func ReadSecretData(ctx context.Context, c client.Client, namespace, name string) (map[string][]byte, error) {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	return secret.Data, nil
}
