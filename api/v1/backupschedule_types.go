package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupPhase represents the high-level state of the last backup attempt.
type BackupPhase string

const (
	BackupPhasePending BackupPhase = "Pending"
	BackupPhaseRunning BackupPhase = "Running"
	BackupPhaseSuccess BackupPhase = "Succeeded"
	BackupPhaseFailed  BackupPhase = "Failed"
)

// DatabaseEngine identifies which dump strategy the operator uses.
// +kubebuilder:validation:Enum=postgres;mysql;redis;mongodb
type DatabaseEngine string

const (
	EnginePostgres DatabaseEngine = "postgres"
	EngineMySQL    DatabaseEngine = "mysql"
	EngineRedis    DatabaseEngine = "redis"
	EngineMongoDB  DatabaseEngine = "mongodb"
)

// BackupScheduleSpec defines the desired state of BackupSchedule.
type BackupScheduleSpec struct {
	// Engine selects the backup strategy (postgres, mysql, redis, mongodb).
	// +kubebuilder:default=postgres
	// +kubebuilder:validation:Enum=postgres;mysql;redis;mongodb
	Engine DatabaseEngine `json:"engine,omitempty"`

	// DatabaseRef is a logical name for the target (also used in admission checks).
	// +kubebuilder:validation:MinLength=1
	DatabaseRef string `json:"databaseRef"`

	// SecretRef holds connection credentials.
	// Common keys: host, port, username, password, database.
	// Redis may only need host, port, password.
	SecretRef string `json:"secretRef"`

	// Schedule is a standard cron expression (UTC).
	// Example: "0 */6 * * *" for every 6 hours.
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// RetentionDays is how long backups are kept in object storage.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=7
	RetentionDays int32 `json:"retentionDays,omitempty"`

	// Destination describes the S3-compatible target (e.g. MinIO).
	Destination ObjectStorageSpec `json:"destination"`

	// BackupImage overrides the default container image for the backup Job.
	// +optional
	BackupImage string `json:"backupImage,omitempty"`

	// ActiveDeadlineSeconds limits how long a backup Job may run.
	// +optional
	// +kubebuilder:default=1800
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

	// Suspend stops scheduling new backups when true.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// ObjectStorageSpec configures an S3-compatible destination.
type ObjectStorageSpec struct {
	Endpoint string `json:"endpoint"`
	Bucket   string `json:"bucket"`
	// +optional
	Prefix string `json:"prefix,omitempty"`
	// +optional
	// +kubebuilder:default=us-east-1
	Region               string `json:"region,omitempty"`
	CredentialsSecretRef string `json:"credentialsSecretRef"`
	// +optional
	UseSSL bool `json:"useSSL,omitempty"`
}

// BackupScheduleStatus defines the observed state of BackupSchedule.
type BackupScheduleStatus struct {
	// +optional
	Phase BackupPhase `json:"phase,omitempty"`
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	// +optional
	NextBackupTime *metav1.Time `json:"nextBackupTime,omitempty"`
	// +optional
	LastObjectKey string `json:"lastObjectKey,omitempty"`
	// +optional
	LastJobName string `json:"lastJobName,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bks
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.databaseRef`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Last Backup",type=date,JSONPath=`.status.lastBackupTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackupSchedule is the Schema for the backupschedules API.
type BackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupScheduleSpec   `json:"spec,omitempty"`
	Status BackupScheduleStatus `json:"status,omitempty"`
}

// EffectiveEngine returns the engine, defaulting to postgres.
func (s *BackupSchedule) EffectiveEngine() DatabaseEngine {
	if s.Spec.Engine == "" {
		return EnginePostgres
	}
	return s.Spec.Engine
}

// +kubebuilder:object:root=true

// BackupScheduleList contains a list of BackupSchedule.
type BackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSchedule{}, &BackupScheduleList{})
}
