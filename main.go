/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	routev1 "github.com/openshift/api/route/v1"
	security "github.com/openshift/api/security/v1"
	"github.com/openshift/oadp-operator/controllers"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	//+kubebuilder:scaffold:imports
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(oadpv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	watchNamespace, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace, "+
			"the manager will watch and manage resources in all namespaces")
	}

	// setting privileged pod security labels to operator ns
	err = addPodSecurityPrivilegedLabels(watchNamespace)
	if err != nil {
		setupLog.Error(err, "error setting privileged pod security labels to operator namespace")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "8b4defce.openshift.io",
		Namespace:              watchNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup scheme for OCP resources
	if err := monitor.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift monitoring APIs to scheme")
		os.Exit(1)
	}

	if err := security.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift security APIs to scheme")
		os.Exit(1)
	}

	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add OpenShift route API to scheme")
		os.Exit(1)
	}

	if err := velerov1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Velero APIs to scheme")
		os.Exit(1)
	}

	if err := appsv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Kubernetes APIs to scheme")
		os.Exit(1)
	}

	if err := v1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Kubernetes API extensions to scheme")
		os.Exit(1)
	}

	if err = (&controllers.DPAReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("DPA-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DataProtectionApplication")
		os.Exit(1)
	}

	if err = (&controllers.BucketReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("bucket-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Bucket")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}

// setting privileged pod security labels to OADP operator namespace
func addPodSecurityPrivilegedLabels(watchNamespaceName string) error {
	setupLog.Info("patching operator namespace with PSA labels")

	if len(watchNamespaceName) == 0 {
		return fmt.Errorf("cannot add privileged pod security labels, watchNamespaceName is empty")
	}

	kubeconf := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(kubeconf)
	if err != nil {
		setupLog.Error(err, "problem getting client")
		return err
	}

	operatorNamespace, err := clientset.CoreV1().Namespaces().Get(context.TODO(), watchNamespaceName, metav1.GetOptions{})
	if err != nil {
		setupLog.Error(err, "problem getting operator namespace")
		return err
	}

	privilegedLabels := map[string]string{
		"pod-security.kubernetes.io/enforce": "privileged",
		"pod-security.kubernetes.io/audit":   "privileged",
		"pod-security.kubernetes.io/warn":    "privileged",
	}

	operatorNamespace.SetLabels(privilegedLabels)

	_, err = clientset.CoreV1().Namespaces().Update(context.TODO(), operatorNamespace, metav1.UpdateOptions{})
	if err != nil {
		setupLog.Error(err, "problem patching operator namespace for privileged pod security labels")
		return err
	}
	return nil
}
