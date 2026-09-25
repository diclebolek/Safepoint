package backup

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

// PostgresEngine dumps PostgreSQL with pg_dump, compresses with gzip, uploads to MinIO.
type PostgresEngine struct{}

func (PostgresEngine) Name() backupv1.DatabaseEngine { return backupv1.EnginePostgres }
func (PostgresEngine) DefaultImage() string          { return "postgres:16-alpine" }
func (PostgresEngine) FileExtension() string         { return "dump.gz" }

func (PostgresEngine) Validate(t Target) error {
	if t.Host == "" || t.Username == "" || t.Database == "" {
		return fmt.Errorf("postgres requires host, username, and database in secret")
	}
	return nil
}

func (e PostgresEngine) BuildJob(req JobRequest) (*batchv1.Job, error) {
	if req.Target.Port == "" {
		req.Target.Port = "5432"
	}
	script := `
apk add --no-cache curl gzip >/dev/null
export PGPASSWORD="$DB_PASSWORD"
FILE_PATH=/tmp/backup.dump.gz
pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -F c | gzip -c > "$FILE_PATH"
` + uploadScript

	env := append(targetEnv(req.Target), storageEnv(req)...)
	return baseJob(req, e, script, env)
}

// MySQLEngine dumps MySQL/MariaDB with mysqldump.
type MySQLEngine struct{}

func (MySQLEngine) Name() backupv1.DatabaseEngine { return backupv1.EngineMySQL }
func (MySQLEngine) DefaultImage() string          { return "mysql:8.4" }
func (MySQLEngine) FileExtension() string         { return "sql.gz" }

func (MySQLEngine) Validate(t Target) error {
	if t.Host == "" || t.Username == "" || t.Database == "" {
		return fmt.Errorf("mysql requires host, username, and database in secret")
	}
	return nil
}

func (e MySQLEngine) BuildJob(req JobRequest) (*batchv1.Job, error) {
	if req.Target.Port == "" {
		req.Target.Port = "3306"
	}
	script := `
export DEBIAN_FRONTEND=noninteractive
apt-get update >/dev/null && apt-get install -y --no-install-recommends curl gzip ca-certificates >/dev/null
FILE_PATH=/tmp/backup.sql.gz
mysqldump -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASSWORD" --single-transaction --routines --triggers "$DB_NAME" | gzip -c > "$FILE_PATH"
` + uploadScript

	env := append(targetEnv(req.Target), storageEnv(req)...)
	return baseJob(req, e, script, env)
}

// RedisEngine captures an RDB snapshot via redis-cli --rdb.
type RedisEngine struct{}

func (RedisEngine) Name() backupv1.DatabaseEngine { return backupv1.EngineRedis }
func (RedisEngine) DefaultImage() string          { return "redis:7-alpine" }
func (RedisEngine) FileExtension() string         { return "rdb.gz" }

func (RedisEngine) Validate(t Target) error {
	if t.Host == "" {
		return fmt.Errorf("redis requires host in secret")
	}
	return nil
}

func (e RedisEngine) BuildJob(req JobRequest) (*batchv1.Job, error) {
	if req.Target.Port == "" {
		req.Target.Port = "6379"
	}
	script := `
apk add --no-cache curl gzip >/dev/null
FILE_PATH=/tmp/dump.rdb.gz
if [ -n "$DB_PASSWORD" ]; then
  redis-cli -h "$DB_HOST" -p "$DB_PORT" -a "$DB_PASSWORD" --rdb /tmp/dump.rdb
else
  redis-cli -h "$DB_HOST" -p "$DB_PORT" --rdb /tmp/dump.rdb
fi
gzip -c /tmp/dump.rdb > "$FILE_PATH"
rm -f /tmp/dump.rdb
` + uploadScript

	env := append(targetEnv(req.Target), storageEnv(req)...)
	return baseJob(req, e, script, env)
}

// MongoEngine dumps MongoDB with mongodump --archive --gzip.
type MongoEngine struct{}

func (MongoEngine) Name() backupv1.DatabaseEngine { return backupv1.EngineMongoDB }
func (MongoEngine) DefaultImage() string          { return "mongo:7" }
func (MongoEngine) FileExtension() string         { return "archive.gz" }

func (MongoEngine) Validate(t Target) error {
	if t.Host == "" {
		return fmt.Errorf("mongodb requires host in secret")
	}
	return nil
}

func (e MongoEngine) BuildJob(req JobRequest) (*batchv1.Job, error) {
	if req.Target.Port == "" {
		req.Target.Port = "27017"
	}
	script := `
export DEBIAN_FRONTEND=noninteractive
apt-get update >/dev/null && apt-get install -y --no-install-recommends curl ca-certificates >/dev/null
FILE_PATH=/tmp/backup.archive.gz
if [ -n "$DB_USER" ]; then
  URI="mongodb://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}"
else
  URI="mongodb://${DB_HOST}:${DB_PORT}"
fi
if [ -n "$DB_NAME" ]; then
  mongodump --uri="$URI" --db="$DB_NAME" --archive="$FILE_PATH" --gzip
else
  mongodump --uri="$URI" --archive="$FILE_PATH" --gzip
fi
` + uploadScript

	env := append(targetEnv(req.Target), storageEnv(req)...)
	return baseJob(req, e, script, env)
}

var (
	_ Engine = PostgresEngine{}
	_ Engine = MySQLEngine{}
	_ Engine = RedisEngine{}
	_ Engine = MongoEngine{}
)
