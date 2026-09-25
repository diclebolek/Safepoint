package controller

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
	"github.com/diclebolek/Safepoint/internal/metrics"
)

// BackupRestoreReconciler reconciles BackupRestore objects.
type BackupRestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock  func() time.Time
}

func (r *BackupRestoreReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now().UTC()
}

// Reconcile runs a one-shot restore Job.
func (r *BackupRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var restore backupv1.BackupRestore
	if err := r.Get(ctx, req.NamespacedName, &restore); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if restore.Status.Phase == backupv1.RestorePhaseSuccess || restore.Status.Phase == backupv1.RestorePhaseFailed {
		return ctrl.Result{}, nil
	}

	backup.SetScheme(r.Scheme)

	if restore.Status.JobName != "" {
		var job batchv1.Job
		err := r.Get(ctx, types.NamespacedName{Namespace: restore.Namespace, Name: restore.Status.JobName}, &job)
		if apierrors.IsNotFound(err) {
			return r.fail(ctx, &restore, "restore job disappeared")
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		if job.Status.Succeeded > 0 {
			metrics.RestoreSuccess.WithLabelValues(restore.Namespace, restore.Name, string(restore.EffectiveEngine())).Inc()
			return r.patch(ctx, &restore, func(o *backupv1.BackupRestore) {
				o.Status.Phase = backupv1.RestorePhaseSuccess
				o.Status.Message = "restore job succeeded"
				o.Status.CompletionTime = &metav1.Time{Time: r.now()}
				o.Status.ObservedGeneration = o.Generation
			})
		}
		if job.Status.Failed > 0 {
			metrics.RestoreFailure.WithLabelValues(restore.Namespace, restore.Name, string(restore.EffectiveEngine())).Inc()
			return r.fail(ctx, &restore, "restore job failed")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	dbSecret, err := ReadSecretData(ctx, r.Client, restore.Namespace, restore.Spec.SecretRef)
	if err != nil {
		return r.fail(ctx, &restore, err.Error())
	}
	storeSecret, err := ReadSecretData(ctx, r.Client, restore.Namespace, restore.Spec.Destination.CredentialsSecretRef)
	if err != nil {
		return r.fail(ctx, &restore, err.Error())
	}
	accessKey, secretKey, err := backup.ParseStorageSecret(storeSecret)
	if err != nil {
		return r.fail(ctx, &restore, err.Error())
	}
	region := restore.Spec.Destination.Region
	if region == "" {
		region = "us-east-1"
	}

	job, err := backup.BuildRestoreJob(backup.RestoreRequest{
		Restore:   &restore,
		Target:    backup.ParseTargetSecret(dbSecret),
		ObjectKey: restore.Spec.ObjectKey,
		Storage: backup.StorageEnv{
			Endpoint:  restore.Spec.Destination.Endpoint,
			Bucket:    restore.Spec.Destination.Bucket,
			Prefix:    restore.Spec.Destination.Prefix,
			Region:    region,
			AccessKey: accessKey,
			SecretKey: secretKey,
			UseSSL:    restore.Spec.Destination.UseSSL,
		},
	})
	if err != nil {
		return r.fail(ctx, &restore, err.Error())
	}
	if err := r.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, _ = r.patch(ctx, &restore, func(o *backupv1.BackupRestore) {
				o.Status.Phase = backupv1.RestorePhaseRunning
				o.Status.JobName = job.Name
				o.Status.Message = "restore job exists"
				o.Status.ObservedGeneration = o.Generation
			})
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return r.fail(ctx, &restore, err.Error())
	}

	logger.Info("created restore job", "job", job.Name)
	_, err = r.patch(ctx, &restore, func(o *backupv1.BackupRestore) {
		o.Status.Phase = backupv1.RestorePhaseRunning
		o.Status.JobName = job.Name
		o.Status.Message = "restore job created"
		o.Status.ObservedGeneration = o.Generation
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *BackupRestoreReconciler) fail(ctx context.Context, restore *backupv1.BackupRestore, msg string) (ctrl.Result, error) {
	_, err := r.patch(ctx, restore, func(o *backupv1.BackupRestore) {
		o.Status.Phase = backupv1.RestorePhaseFailed
		o.Status.Message = msg
		o.Status.CompletionTime = &metav1.Time{Time: r.now()}
		o.Status.ObservedGeneration = o.Generation
	})
	return ctrl.Result{}, err
}

func (r *BackupRestoreReconciler) patch(ctx context.Context, restore *backupv1.BackupRestore, mutate func(*backupv1.BackupRestore)) (ctrl.Result, error) {
	base := restore.DeepCopy()
	mutate(restore)
	if err := r.Status().Patch(ctx, restore, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch restore status: %w", err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the restore controller.
func (r *BackupRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.BackupRestore{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
