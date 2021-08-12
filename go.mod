module github.com/kubevirt/node-maintenance-operator

go 1.16

require (
	github.com/evanphx/json-patch v4.11.0+incompatible // indirect
	github.com/go-logr/zapr v0.4.0 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.14.0
	github.com/operator-framework/api v0.5.1 // indirect
	github.com/operator-framework/operator-lib v0.3.0
	github.com/prometheus/client_golang v1.11.0 // indirect
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.1.3 // indirect
	go.uber.org/zap v1.18.1 // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/tools v0.1.2
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	k8s.io/api v0.21.3
	k8s.io/apiextensions-apiserver v0.21.3 // indirect
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.20.2
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	mvdan.cc/sh/v3 v3.3.0
	sigs.k8s.io/controller-runtime v0.7.0
//github.com/kubernetes-sigs/controller-runtime v0.9.5
)

replace (
	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2
)
