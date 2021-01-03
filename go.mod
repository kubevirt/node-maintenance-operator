module kubevirt.io/node-maintenance-operator

go 1.15

require (
	github.com/go-openapi/spec v0.19.4
	github.com/kr/pretty v0.2.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/onsi/ginkgo v1.12.2
	github.com/onsi/gomega v1.10.1
	github.com/operator-framework/api v0.3.8
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
	k8s.io/api v0.18.2
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200121204235-bf4fb3bd569c
	k8s.io/kubectl v0.18.2
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
	mvdan.cc/sh/v3 => mvdan.cc/sh/v3 v3.1.2 // v3.2.0 is broken: https://github.com/mvdan/sh/issues/625
)
