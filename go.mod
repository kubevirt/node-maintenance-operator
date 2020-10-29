module kubevirt.io/node-maintenance-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/spec v0.19.4 // indirect
	github.com/onsi/ginkgo v1.12.2
	github.com/onsi/gomega v1.10.1
	github.com/operator-framework/api v0.3.13
	github.com/operator-framework/operator-sdk v1.1.0 // indirect
	github.com/sirupsen/logrus v1.6.0
	k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.18.8
	k8s.io/utils v0.0.0-20200603063816-c1c6865ac451
	mvdan.cc/sh/v3 v3.1.2 // indirect
	sigs.k8s.io/controller-runtime v0.6.2
)

replace k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
