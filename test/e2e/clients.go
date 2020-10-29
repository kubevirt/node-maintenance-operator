package e2e

import (
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	nodemaintenancev1beta1 "kubevirt.io/node-maintenance-operator/api/v1beta1"
)

var (
	// Client defines the API client to run CRUD operations, that will be used for testing
	Client client.Client
	// K8sClient defines k8s client to run subresource operations, for example you should use it to get pod logs
	KubeClient *kubernetes.Clientset
	// ClientsEnabled tells if the client from the package can be used
	ClientsEnabled bool
)

func init() {
	// Setup Scheme for all resources

	// in case we need to work with CRDs
	if err := apiextensionsv1beta1.AddToScheme(scheme.Scheme); err != nil {
		klog.Exit(err.Error())
	}

	// in case we need to work with OLM types
	if err := operatorsv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Exit(err.Error())
	}

	// for NMO types
	if err := nodemaintenancev1beta1.AddToScheme(scheme.Scheme); err != nil {
		klog.Exit(err.Error())
	}

	var err error
	Client, err = newClient()
	if err != nil {
		klog.Info("Failed to initialize client, check the KUBECONFIG env variable", err.Error())
		ClientsEnabled = false
		return
	}
	KubeClient, err = newK8s()
	if err != nil {
		klog.Info("Failed to initialize k8s client, check the KUBECONFIG env variable", err.Error())
		ClientsEnabled = false
		return
	}
	ClientsEnabled = true
}

// newClient returns a controller-runtime client.
func newClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	c, err := client.New(cfg, client.Options{})
	return c, err
}

// newK8s returns a kubernetes clientset
func newK8s() (*kubernetes.Clientset, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Exit(err.Error())
	}
	return clientset, nil
}
