// kubectl-safepoint is a kubectl plugin: place the binary on PATH as kubectl-safepoint
// then run: kubectl safepoint status
package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	ns := namespaceFromFlags(os.Args[2:])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dc, err := dynamicClient()
	if err != nil {
		fatal(err)
	}

	switch cmd {
	case "status":
		if err := printSchedules(ctx, dc, ns); err != nil {
			fatal(err)
		}
		fmt.Println()
		if err := printRestores(ctx, dc, ns); err != nil {
			fatal(err)
		}
	case "schedules":
		if err := printSchedules(ctx, dc, ns); err != nil {
			fatal(err)
		}
	case "restores":
		if err := printRestores(ctx, dc, ns); err != nil {
			fatal(err)
		}
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `kubectl safepoint — Safepoint status plugin

Usage:
  kubectl safepoint status [-n namespace]
  kubectl safepoint schedules [-n namespace]
  kubectl safepoint restores [-n namespace]

Build:
  go build -o kubectl-safepoint.exe ./cmd/kubectl-safepoint
  # put binary on PATH (named kubectl-safepoint)
`)
}

func namespaceFromFlags(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-n" || args[i] == "--namespace" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if len(args[i]) > 2 && args[i][:2] == "-n" && args[i] != "-n" {
			return args[i][2:]
		}
	}
	return metav1.NamespaceAll
}

func dynamicClient() (dynamic.Interface, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

func gvrSchedule() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "backup.goproject.io", Version: "v1", Resource: "backupschedules"}
}

func gvrRestore() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "backup.goproject.io", Version: "v1", Resource: "backuprestores"}
}

func printSchedules(ctx context.Context, dc dynamic.Interface, ns string) error {
	list, err := dc.Resource(gvrSchedule()).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNAME\tENGINE\tPHASE\tLAST OBJECT KEY")
	for _, item := range list.Items {
		var s backupv1.BackupSchedule
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &s); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.Namespace, s.Name, s.EffectiveEngine(), s.Status.Phase, s.Status.LastObjectKey)
	}
	return w.Flush()
}

func printRestores(ctx context.Context, dc dynamic.Interface, ns string) error {
	list, err := dc.Resource(gvrRestore()).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNAME\tENGINE\tPHASE\tMESSAGE")
	for _, item := range list.Items {
		var r backupv1.BackupRestore
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &r); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.Namespace, r.Name, r.EffectiveEngine(), r.Status.Phase, r.Status.Message)
	}
	return w.Flush()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
