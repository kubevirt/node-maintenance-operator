package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/pflag"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	"kubevirt.io/node-maintenance-operator/pkg/apis"
	"kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	"kubevirt.io/node-maintenance-operator/pkg/controller"
	"kubevirt.io/node-maintenance-operator/version"
)

// Change below variables to serve metrics on different host or port.
const (
	metricsHost       = "0.0.0.0"
	metricsPort int32 = 8383
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

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	log.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	log.Info(fmt.Sprintf("Git Commit: %s", version.GitCommit))
	log.Info(fmt.Sprintf("Build Date: %s", version.BuildDate))
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:     true,
		LeaderElectionID:   "node-maintenance-operator-lock",
	})
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Create Service object to expose the metrics port.
	servicePorts := []v1.ServicePort{
		{Port: metricsPort, Name: metrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort}},
	}
	_, err = metrics.CreateMetricsService(context.Background(), cfg, servicePorts)
	if err != nil {
		log.Info(err.Error())
	}

	// Setup webhooks
	if err := setupWebhookServer(mgr); err != nil {
		log.Error(err, "Failed to setup webhook server")
		os.Exit(1)
	}

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
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
