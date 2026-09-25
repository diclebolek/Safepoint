package backup

import (
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

const DefaultBackupImage = "safepoint-backup:dev"

// RestoreRequest builds a restore Job.
type RestoreRequest struct {
	Restore   *backupv1.BackupRestore
	Target    Target
	Storage   StorageEnv
	ObjectKey string
}

// BuildRestoreJob creates an engine-specific restore Job.
func BuildRestoreJob(req RestoreRequest) (*batchv1.Job, error) {
	engine := req.Restore.EffectiveEngine()
	script, err := restoreScript(engine)
	if err != nil {
		return nil, err
	}
	if err := validateRestoreTarget(engine, req.Target); err != nil {
		return nil, err
	}

	image := DefaultBackupImage
	if req.Restore.Spec.BackupImage != "" {
		image = req.Restore.Spec.BackupImage
	}

	deadline := int64(1800)
	if req.Restore.Spec.ActiveDeadlineSeconds != nil {
		deadline = *req.Restore.Spec.ActiveDeadlineSeconds
	}

	ts := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-restore-%s", req.Restore.Name, ts)
	if len(name) > 63 {
		name = name[:63]
		name = strings.TrimRight(name, "-")
	}

	scheme := "http"
	if req.Storage.UseSSL {
		scheme = "https"
	}
	env := append(targetEnv(req.Target), []corev1.EnvVar{
		{Name: "S3_ENDPOINT", Value: req.Storage.Endpoint},
		{Name: "S3_BUCKET", Value: req.Storage.Bucket},
		{Name: "S3_ACCESS_KEY", Value: req.Storage.AccessKey},
		{Name: "S3_SECRET_KEY", Value: req.Storage.SecretKey},
		{Name: "S3_SCHEME", Value: scheme},
		{Name: "OBJECT_KEY", Value: req.ObjectKey},
	}...)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: req.Restore.Namespace,
			Labels: map[string]string{
				LabelScheduleName:              req.Restore.Name,
				LabelEngine:                    string(engine),
				"app.kubernetes.io/managed-by": "backup-operator",
				"backup.goproject.io/kind":     "restore",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To[int32](1),
			TTLSecondsAfterFinished: ptr.To[int32](3600),
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "restore",
						Image:   image,
						Command: []string{"/bin/sh", "-ec"},
						Args:    []string{script},
						Env:     env,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
						},
					}},
				},
			},
		},
	}
	if jobScheme != nil {
		if err := controllerutil.SetControllerReference(req.Restore, job, jobScheme); err != nil {
			return nil, fmt.Errorf("set owner reference: %w", err)
		}
	}
	return job, nil
}

func validateRestoreTarget(engine backupv1.DatabaseEngine, t Target) error {
	switch engine {
	case backupv1.EnginePostgres, backupv1.EngineMySQL:
		if t.Host == "" || t.Username == "" || t.Database == "" {
			return fmt.Errorf("%s restore requires host, username, database", engine)
		}
	case backupv1.EngineRedis, backupv1.EngineMongoDB:
		if t.Host == "" {
			return fmt.Errorf("%s restore requires host", engine)
		}
	}
	return nil
}

func restoreScript(engine backupv1.DatabaseEngine) (string, error) {
	download := `
FILE_PATH=/tmp/restore.bin
curl -fsS -o "$FILE_PATH" "${S3_SCHEME}://${S3_ENDPOINT}/${S3_BUCKET}/${OBJECT_KEY}"
`
	switch engine {
	case backupv1.EnginePostgres:
		return download + `
export PGPASSWORD="$DB_PASSWORD"
# custom format dumps use pg_restore; gzip custom still works after gunzip if needed
if echo "$OBJECT_KEY" | grep -q '\.gz$'; then
  gunzip -c "$FILE_PATH" > /tmp/restore.dump || cp "$FILE_PATH" /tmp/restore.dump
else
  cp "$FILE_PATH" /tmp/restore.dump
fi
pg_restore -h "$DB_HOST" -p "${DB_PORT:-5432}" -U "$DB_USER" -d "$DB_NAME" --clean --if-exists /tmp/restore.dump || \
  psql -h "$DB_HOST" -p "${DB_PORT:-5432}" -U "$DB_USER" -d "$DB_NAME" -f /tmp/restore.dump
echo "postgres restore finished"
`, nil
	case backupv1.EngineMySQL:
		return download + `
gunzip -c "$FILE_PATH" 2>/dev/null | mysql -h "$DB_HOST" -P "${DB_PORT:-3306}" -u "$DB_USER" -p"$DB_PASSWORD" "$DB_NAME" \
  || mysql -h "$DB_HOST" -P "${DB_PORT:-3306}" -u "$DB_USER" -p"$DB_PASSWORD" "$DB_NAME" < "$FILE_PATH"
echo "mysql restore finished"
`, nil
	case backupv1.EngineRedis:
		return download + `
gunzip -c "$FILE_PATH" > /tmp/dump.rdb 2>/dev/null || cp "$FILE_PATH" /tmp/dump.rdb
# Best-effort: replace RDB via redis-cli --pipe is not applicable; document that Redis restore
# typically requires stopping the server and replacing dump.rdb. For demo we use DEBUG RELOAD after copy is not possible remotely.
# Instead push keys is out of scope; we validate file downloaded.
test -s /tmp/dump.rdb
echo "redis rdb downloaded (apply offline by replacing dump.rdb on the Redis volume)"
`, nil
	case backupv1.EngineMongoDB:
		return download + `
mongorestore --uri="mongodb://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT:-27017}" --archive="$FILE_PATH" --gzip --drop \
  || mongorestore --uri="mongodb://${DB_HOST}:${DB_PORT:-27017}" --archive="$FILE_PATH" --gzip --drop
echo "mongodb restore finished"
`, nil
	default:
		return "", fmt.Errorf("unsupported restore engine %q", engine)
	}
}
