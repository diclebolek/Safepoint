package backup

import (
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

const (
	LabelScheduleName   = "backup.goproject.io/schedule"
	LabelEngine         = "backup.goproject.io/engine"
	AnnotationObjectKey = "backup.goproject.io/object-key"
)

// Target describes where and how to connect for a backup.
type Target struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
}

// JobRequest is the input for building a backup Job.
type JobRequest struct {
	Schedule  *backupv1.BackupSchedule
	Target    Target
	ObjectKey string
	Storage   StorageEnv
}

// StorageEnv is injected into the Job for MinIO/S3 upload (never log these).
type StorageEnv struct {
	Endpoint  string
	Bucket    string
	Prefix    string
	Region    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// Engine builds a Kubernetes Job that dumps a specific database engine.
type Engine interface {
	Name() backupv1.DatabaseEngine
	DefaultImage() string
	FileExtension() string
	Validate(target Target) error
	BuildJob(req JobRequest) (*batchv1.Job, error)
}

// Registry maps engine names to implementations.
type Registry struct {
	engines map[backupv1.DatabaseEngine]Engine
}

// NewRegistry returns the built-in engines (postgres, mysql, redis, mongodb).
func NewRegistry() *Registry {
	engines := []Engine{
		PostgresEngine{},
		MySQLEngine{},
		RedisEngine{},
		MongoEngine{},
	}
	m := make(map[backupv1.DatabaseEngine]Engine, len(engines))
	for _, e := range engines {
		m[e.Name()] = e
	}
	return &Registry{engines: m}
}

// Get returns an engine by name.
func (r *Registry) Get(name backupv1.DatabaseEngine) (Engine, error) {
	e, ok := r.engines[name]
	if !ok {
		return nil, fmt.Errorf("unsupported engine %q", name)
	}
	return e, nil
}

// ObjectKey builds a deterministic object name including engine and compression suffix.
func ObjectKey(namespace, scheduleName, engine, ext string, at time.Time) string {
	ext = strings.TrimPrefix(ext, ".")
	return fmt.Sprintf("%s/%s/%s-%s.%s",
		namespace,
		scheduleName,
		engine,
		at.UTC().Format("20060102T150405Z"),
		ext,
	)
}

// ParseTargetSecret maps Secret data into a Target.
func ParseTargetSecret(data map[string][]byte) Target {
	return Target{
		Host:     string(data["host"]),
		Port:     string(data["port"]),
		Username: string(data["username"]),
		Password: string(data["password"]),
		Database: string(data["database"]),
	}
}

// ParseStorageSecret reads accessKey/secretKey from a Secret.
func ParseStorageSecret(data map[string][]byte) (accessKey, secretKey string, err error) {
	accessKey = string(data["accessKey"])
	secretKey = string(data["secretKey"])
	if accessKey == "" || secretKey == "" {
		return "", "", fmt.Errorf("storage secret must contain accessKey and secretKey")
	}
	return accessKey, secretKey, nil
}

func imageFor(schedule *backupv1.BackupSchedule, engine Engine) string {
	if schedule.Spec.BackupImage != "" {
		return schedule.Spec.BackupImage
	}
	return engine.DefaultImage()
}

func jobMeta(req JobRequest, engine Engine) metav1.ObjectMeta {
	ts := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s-%s", req.Schedule.Name, engine.Name(), ts)
	// Job names must be <= 63 chars.
	if len(name) > 63 {
		name = name[:63]
		name = strings.TrimRight(name, "-")
	}
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: req.Schedule.Namespace,
		Labels: map[string]string{
			LabelScheduleName:              req.Schedule.Name,
			LabelEngine:                    string(engine.Name()),
			"app.kubernetes.io/managed-by": "backup-operator",
		},
		Annotations: map[string]string{
			AnnotationObjectKey: req.ObjectKey,
		},
	}
}

func baseJob(req JobRequest, engine Engine, script string, env []corev1.EnvVar) (*batchv1.Job, error) {
	deadline := int64(1800)
	if req.Schedule.Spec.ActiveDeadlineSeconds != nil {
		deadline = *req.Schedule.Spec.ActiveDeadlineSeconds
	}

	job := &batchv1.Job{
		ObjectMeta: jobMeta(req, engine),
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To[int32](1),
			TTLSecondsAfterFinished: ptr.To[int32](3600),
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelScheduleName: req.Schedule.Name,
						LabelEngine:       string(engine.Name()),
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "backup",
							Image:   imageFor(req.Schedule, engine),
							Command: []string{"/bin/sh", "-ec"},
							Args:    []string{script},
							Env:     env,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
							},
						},
					},
				},
			},
		},
	}

	if jobScheme != nil {
		if err := controllerutil.SetControllerReference(req.Schedule, job, jobScheme); err != nil {
			return nil, fmt.Errorf("set owner reference: %w", err)
		}
	}
	return job, nil
}

// jobScheme is set by the runner so Jobs get owner references.
var jobScheme *runtime.Scheme

// SetScheme configures the scheme used for owner references on Jobs.
func SetScheme(s *runtime.Scheme) {
	jobScheme = s
}

func storageEnv(req JobRequest) []corev1.EnvVar {
	scheme := "http"
	if req.Storage.UseSSL {
		scheme = "https"
	}
	return []corev1.EnvVar{
		{Name: "S3_ENDPOINT", Value: req.Storage.Endpoint},
		{Name: "S3_BUCKET", Value: req.Storage.Bucket},
		{Name: "S3_PREFIX", Value: req.Storage.Prefix},
		{Name: "S3_REGION", Value: req.Storage.Region},
		{Name: "S3_ACCESS_KEY", Value: req.Storage.AccessKey},
		{Name: "S3_SECRET_KEY", Value: req.Storage.SecretKey},
		{Name: "S3_SCHEME", Value: scheme},
		{Name: "OBJECT_KEY", Value: req.ObjectKey},
	}
}

func targetEnv(t Target) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "DB_HOST", Value: t.Host},
		{Name: "DB_PORT", Value: t.Port},
		{Name: "DB_USER", Value: t.Username},
		{Name: "DB_PASSWORD", Value: t.Password},
		{Name: "DB_NAME", Value: t.Database},
	}
}

// uploadScript uploads FILE_PATH to OBJECT_KEY.
// Prefer curl PUT (works with SeaweedFS / many S3 gateways); fall back to mc if available.
const uploadScript = `
upload_with_curl() {
  curl -fsS -X PUT \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"${FILE_PATH}" \
    "${S3_SCHEME}://${S3_ENDPOINT}/${S3_BUCKET}/${OBJECT_KEY}"
}
upload_with_mc() {
  if ! command -v mc >/dev/null 2>&1; then
    return 1
  fi
  mc alias set backup "${S3_SCHEME}://${S3_ENDPOINT}" "${S3_ACCESS_KEY}" "${S3_SECRET_KEY}" --api S3v4
  mc cp "${FILE_PATH}" "backup/${S3_BUCKET}/${OBJECT_KEY}"
}
if upload_with_curl; then
  echo "uploaded ${OBJECT_KEY} via curl"
elif upload_with_mc; then
  echo "uploaded ${OBJECT_KEY} via mc"
else
  echo "upload failed: curl PUT and mc both unavailable/failed" >&2
  exit 1
fi
`
