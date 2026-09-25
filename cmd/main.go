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

	registry := backup.NewRegistry()
	jobRunner := &runner.JobRunner{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Registry: registry,
	}

	if err := (&controller.BackupScheduleReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Jobs:     jobRunner,
		StoreFor: controller.DefaultStoreFor(mgr.GetClient()),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BackupSchedule")
		os.Exit(1)
	}

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

	setupLog.Info("starting manager", "engines", "postgres,mysql,redis,mongodb", "webhooks", enableWebhooks)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
