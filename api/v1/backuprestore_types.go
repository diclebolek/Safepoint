package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestorePhase is the phase of a BackupRestore.
type RestorePhase string

const (
	RestorePhasePending RestorePhase = "Pending"
	RestorePhaseRunning RestorePhase = "Running"
	RestorePhaseSuccess RestorePhase = "Succeeded"
	RestorePhaseFailed  RestorePhase = "Failed"
)

// BackupRestoreSpec defines a one-shot restore from object storage.
type BackupRestoreSpec struct {
	// Engine selects restore strategy.
	// +kubebuilder:default=postgres
	// +kubebuilder:validation:Enum=postgres;mysql;redis;mongodb
	Engine DatabaseEngine `json:"engine,omitempty"`

	// SecretRef holds DB connection credentials.
	SecretRef string `json:"secretRef"`

	// ObjectKey is the object storage key to restore (e.g. demo/shop/...dump.gz).
	ObjectKey string `json:"objectKey"`

	// Destination is the S3-compatible source of the backup object.
	Destination ObjectStorageSpec `json:"destination"`

	// BackupImage overrides the restore Job image.
	// +optional
	BackupImage string `json:"backupImage,omitempty"`

	// ActiveDeadlineSeconds limits Job runtime.
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
}

// BackupRestoreStatus is the observed state.
type BackupRestoreStatus struct {
	// +optional
	Phase RestorePhase `json:"phase,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	JobName string `json:"jobName,omitempty"`
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bkr
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackupRestore restores a backup object into a database.
type BackupRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupRestoreSpec   `json:"spec,omitempty"`
	Status BackupRestoreStatus `json:"status,omitempty"`
}

// EffectiveEngine returns engine or postgres.
func (r *BackupRestore) EffectiveEngine() DatabaseEngine {
	if r.Spec.Engine == "" {
		return EnginePostgres
	}
	return r.Spec.Engine
}

// +kubebuilder:object:root=true

// BackupRestoreList contains BackupRestore items.
type BackupRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupRestore{}, &BackupRestoreList{})
}
