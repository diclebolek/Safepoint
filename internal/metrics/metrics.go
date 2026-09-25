package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	BackupSuccess = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safepoint_backup_success_total",
		Help: "Successful backup jobs",
	}, []string{"namespace", "schedule", "engine"})

	BackupFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safepoint_backup_failure_total",
		Help: "Failed backup jobs",
	}, []string{"namespace", "schedule", "engine"})

	BackupDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "safepoint_backup_duration_seconds",
		Help:    "Observed backup job duration from Running to terminal state",
		Buckets: prometheus.DefBuckets,
	}, []string{"namespace", "schedule", "engine"})

	RestoreSuccess = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safepoint_restore_success_total",
		Help: "Successful restore jobs",
	}, []string{"namespace", "name", "engine"})

	RestoreFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safepoint_restore_failure_total",
		Help: "Failed restore jobs",
	}, []string{"namespace", "name", "engine"})
)

func init() {
	metrics.Registry.MustRegister(BackupSuccess, BackupFailure, BackupDuration, RestoreSuccess, RestoreFailure)
}
