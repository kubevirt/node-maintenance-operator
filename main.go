/*


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
	"flag"
	"fmt"
	"io/ioutil"
	"kubevirt.io/node-maintenance-operator/api/v1beta1"
	"kubevirt.io/node-maintenance-operator/version"
	"os"
	"runtime"
	"strings"

	//"fmt"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	//"sigs.k8s.io/controller-runtime/pkg/client/config"
	nodemaintenancev1beta1 "kubevirt.io/node-maintenance-operator/api/v1beta1"
	"kubevirt.io/node-maintenance-operator/controllers"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	// +kubebuilder:scaffold:imports
)

const (
	// Must match port in deploy/webhooks/nodemaintenance.webhook.yaml
	WebhookPort = 8443
	// This is the cert location as configured by OLM
	WebhookCertDir  = "/apiserver.local.config/certificates"
	WebhookCertName = "apiserver.crt"
	WebhookKeyName  = "apiserver.key"

	// keep this synced to the name in deploy/webhooks/nodemaintenance.webhook.yaml
	WebhookConfigName = "nodemaintenance-validation.kubevirt.io"
)

var log = logf.Log.WithName("cmd")

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(nodemaintenancev1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	//log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	log.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	log.Info(fmt.Sprintf("Git Commit: %s", version.GitCommit))
	log.Info(fmt.Sprintf("Build Date: %s", version.BuildDate))
}

// getOperatorNamespace returns the namespace the operator should be running in.
func getOperatorNamespace() (string, error) {
	nsBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		if os.IsNotExist(err) {
			return "", err
		}
		return "", err
	}
	ns := strings.TrimSpace(string(nsBytes))

	return ns, nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	printVersion()

	namespace, err := getOperatorNamespace()
	if err != nil {
		log.Error(err, "Failed to get operator's namespace")
		os.Exit(1)
	}

	controllers.SetLeaseNamespace(namespace)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "node-maintenance-operator-lock",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.NodeMaintenanceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("NodeMaintenance"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodeMaintenance")
		os.Exit(1)
	}

	// Setup webhooks
	if err := setupWebhookServer(mgr); err != nil {
		log.Error(err, "Failed to setup webhook server")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupWebhookServer(mgr manager.Manager) error {

	// Make sure the certificates are mounted, this should be handled by the OLM
	certs := []string{filepath.Join(WebhookCertDir, WebhookCertName), filepath.Join(WebhookCertDir, WebhookKeyName)}
	for _, fname := range certs {
		if _, err := os.Stat(fname); err != nil {
			log.Error(err, "Failed to prepare webhook server, certificates not found")
			return err
		}
	}

	server := mgr.GetWebhookServer()
	server.Port = WebhookPort
	server.CertDir = WebhookCertDir
	server.CertName = WebhookCertName
	server.KeyName = WebhookKeyName

	server.Register("/validate-nodemaintenance-kubevirt-io-v1beta1-nodemaintenances", admission.ValidatingWebhookFor(&v1beta1.NodeMaintenance{}))

	v1beta1.InitValidator(mgr.GetClient())

	return nil

}
