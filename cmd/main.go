// =============================================================================
// Bu dosya ne ise yarar?
//   Safepoint operator'unun giris noktasi (main).
//   Manager'i baslatir: BackupSchedule + BackupRestore controller'lari,
//   istege bagli admission webhook, /metrics ve health endpoint'leri.
//
// Akis ozeti:
//   1) Flag'leri oku (metrics, webhook, leader-elect)
//   2) controller-runtime Manager olustur
//   3) Engine registry + JobRunner bagla
//   4) Controller'lari kaydet
//   5) mgr.Start ile sonsuz dongude kal
// =============================================================================

package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
	"github.com/diclebolek/Safepoint/internal/backup"
	"github.com/diclebolek/Safepoint/internal/controller"
	_ "github.com/diclebolek/Safepoint/internal/metrics"
	"github.com/diclebolek/Safepoint/internal/runner"
	backupwebhook "github.com/diclebolek/Safepoint/internal/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(backupv1.AddToScheme(scheme))
}

func main() {
	// CLI flags: metrics :8080, health :8081, webhook certs, optional webhooks.
	var (
		metricsAddr          string
		probeAddr            string
		webhookCertDir       string
		enableLeaderElection bool
		enableWebhooks       bool
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "metrics endpoint")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "health probe endpoint")
	flag.StringVar(&webhookCertDir, "webhook-cert-dir", "/certs", "directory with tls.crt and tls.key")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "enable leader election")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false, "enable validating admission webhook")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Manager = shared clients, cache, metrics, webhook server, leader election.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "backup-operator.goproject.io",
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    9443,
			CertDir: webhookCertDir,
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Engines (postgres/mysql/redis/mongodb) + Job runner used by the schedule controller.
	registry := backup.NewRegistry()
	jobRunner := &runner.JobRunner{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Registry: registry,
	}

	// Watch BackupSchedule -> create dump Jobs on cron.
	if err := (&controller.BackupScheduleReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Jobs:     jobRunner,
		StoreFor: controller.DefaultStoreFor(mgr.GetClient()),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BackupSchedule")
		os.Exit(1)
	}

	// Watch BackupRestore -> one-shot restore Jobs.
	if err := (&controller.BackupRestoreReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BackupRestore")
		os.Exit(1)
	}

	// Optional: reject Pods/Deployments that require backup but have no active schedule.
	if enableWebhooks {
		decoder := admission.NewDecoder(mgr.GetScheme())
		mgr.GetWebhookServer().Register("/validate-v1-pod", &webhook.Admission{
			Handler: &backupwebhook.BackupRequiredValidator{
				Client:  mgr.GetClient(),
				Decoder: decoder,
			},
		})
		mgr.GetWebhookServer().Register("/validate-v1-deployment", &webhook.Admission{
			Handler: &backupwebhook.DeploymentBackupValidator{
				Client:  mgr.GetClient(),
				Decoder: decoder,
			},
		})
		setupLog.Info("admission webhooks registered",
			"paths", []string{"/validate-v1-pod", "/validate-v1-deployment"})
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Block forever serving reconcile loops until signal.
	setupLog.Info("starting manager", "engines", "postgres,mysql,redis,mongodb", "webhooks", enableWebhooks)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
