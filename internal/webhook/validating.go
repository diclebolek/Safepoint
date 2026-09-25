// Package webhook provides admission validation for backup-related resources.
package webhook

import (
	"context"
	"fmt"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

const (
	// LabelRequireBackup marks a workload that must have a BackupSchedule.
	LabelRequireBackup = "backup.goproject.io/require-backup"
	// AnnotationDatabaseName links a workload to BackupSchedule.spec.databaseRef.
	AnnotationDatabaseName = "backup.goproject.io/database-name"
)

// BackupRequiredValidator rejects Pods that demand a BackupSchedule but lack one.
type BackupRequiredValidator struct {
	Client  client.Client
	Decoder admission.Decoder
}

// Handle implements admission.Handler for Pod CREATE/UPDATE.
func (v *BackupRequiredValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	if err := v.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	return v.admit(ctx, logger, req.Namespace, pod.Name, pod.Labels, pod.Annotations)
}

// DeploymentBackupValidator rejects Deployments whose pod template requires backup
// but no active BackupSchedule exists (fail fast before replicas are created).
type DeploymentBackupValidator struct {
	Client  client.Client
	Decoder admission.Decoder
}

// Handle implements admission.Handler for Deployment CREATE/UPDATE.
func (v *DeploymentBackupValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx)

	dep := &appsv1.Deployment{}
	if err := v.Decoder.Decode(req, dep); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	meta := dep.Spec.Template.ObjectMeta
	return (&BackupRequiredValidator{Client: v.Client}).admit(
		ctx, logger, req.Namespace, dep.Name, meta.Labels, meta.Annotations,
	)
}

func (v *BackupRequiredValidator) admit(
	ctx context.Context,
	logger interface {
		Error(err error, msg string, keysAndValues ...interface{})
	},
	namespace, name string,
	labels, annotations map[string]string,
) admission.Response {
	if labels[LabelRequireBackup] != "true" {
		return admission.Allowed("")
	}

	dbName := annotations[AnnotationDatabaseName]
	if dbName == "" {
		return admission.Denied(fmt.Sprintf(
			"%q has %s=true but missing annotation %s",
			name, LabelRequireBackup, AnnotationDatabaseName,
		))
	}

	ok, err := HasActiveBackup(ctx, v.Client, namespace, dbName)
	if err != nil {
		logger.Error(err, "list BackupSchedule")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if ok {
		return admission.Allowed("")
	}

	return admission.Denied(fmt.Sprintf(
		"database %q requires an active BackupSchedule in namespace %q before %q can be admitted",
		dbName, namespace, name,
	))
}

// HasActiveBackup reports whether an unsuspended BackupSchedule targets databaseRef.
func HasActiveBackup(ctx context.Context, c client.Client, namespace, databaseRef string) (bool, error) {
	var list backupv1.BackupScheduleList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for i := range list.Items {
		item := list.Items[i]
		if item.Spec.DatabaseRef == databaseRef && !item.Spec.Suspend {
			return true, nil
		}
	}
	return false, nil
}
