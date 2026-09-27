package backup

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

// ResolveDestination returns inline destination or loads DestinationProfile by ref.
func ResolveDestination(ctx context.Context, c client.Client, namespace string, inline *backupv1.ObjectStorageSpec, ref string) (backupv1.ObjectStorageSpec, error) {
	hasInline := inline != nil && inline.Endpoint != "" && inline.Bucket != "" && inline.CredentialsSecretRef != ""
	hasRef := ref != ""
	switch {
	case hasInline && hasRef:
		return backupv1.ObjectStorageSpec{}, fmt.Errorf("set only one of destination or destinationRef")
	case hasRef:
		var profile backupv1.DestinationProfile
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref}, &profile); err != nil {
			return backupv1.ObjectStorageSpec{}, fmt.Errorf("get DestinationProfile %s/%s: %w", namespace, ref, err)
		}
		return profile.Spec.ObjectStorageSpec, nil
	case hasInline:
		return *inline, nil
	default:
		return backupv1.ObjectStorageSpec{}, fmt.Errorf("destination or destinationRef is required")
	}
}

// ResolveBackupMode picks full vs incremental for this run.
// Incremental promotes to full when no prior successful full exists.
func ResolveBackupMode(schedule *backupv1.BackupSchedule) (mode backupv1.BackupMode, parent string) {
	mode = schedule.EffectiveMode()
	if mode == backupv1.BackupModeIncremental {
		if schedule.Status.LastFullObjectKey == "" {
			return backupv1.BackupModeFull, ""
		}
		return backupv1.BackupModeIncremental, schedule.Status.LastFullObjectKey
	}
	return backupv1.BackupModeFull, ""
}
