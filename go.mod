module kubevirt.io/node-maintenance-operator

go 1.15

require (
	github.com/go-openapi/spec v0.19.4
	github.com/google/renameio v1.0.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/nxadm/tail v1.4.6 // indirect
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.1
	github.com/operator-framework/api v0.3.8
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/mod v0.4.0 // indirect
	golang.org/x/sys v0.0.0-20210112091331-59c308dcf3cc // indirect
	golang.org/x/term v0.0.0-20201210144234-2321bbc49cbf // indirect
	golang.org/x/tools v0.0.0-20210111221946-d33bae441459
	k8s.io/api v0.18.15
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.15
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	k8s.io/kubectl v0.18.15
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	mvdan.cc/sh/v3 v3.2.1
	sigs.k8s.io/controller-runtime v0.6.0
)

replace k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
