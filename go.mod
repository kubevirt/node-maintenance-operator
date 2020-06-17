module kubevirt.io/node-maintenance-operator

go 1.13

require (
	cloud.google.com/go v0.34.0 // indirect
	github.com/Azure/go-autorest/autorest v0.10.2 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.8.3 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/appscode/jsonpatch v0.0.0-20190108182946-7c0e3b262f30 // indirect
	github.com/coreos/prometheus-operator v0.26.0 // indirect
	github.com/emicklei/go-restful v2.9.0+incompatible // indirect
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/go-logr/logr v0.1.0 // indirect
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/go-openapi/jsonpointer v0.18.0 // indirect
	github.com/go-openapi/jsonreference v0.18.0 // indirect
	github.com/go-openapi/spec v0.19.0
	github.com/go-openapi/swag v0.18.0 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/mock v1.3.1-0.20190508161146-9fa652df1129
	github.com/google/btree v1.0.0 // indirect
	github.com/google/uuid v1.1.0 // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gophercloud/gophercloud v0.1.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20190212212710-3befbb6ad0cc // indirect
	github.com/hashicorp/golang-lru v0.0.0-00010101000000-000000000000 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mailru/easyjson v0.0.0-20190221075403-6243d8e04c3f // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/operator-framework/operator-sdk v0.8.0
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90 // indirect
	github.com/prometheus/common v0.2.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190225181712-6ed1f7e10411 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/sirupsen/logrus v1.3.0
	github.com/spf13/pflag v1.0.3
	go.uber.org/atomic v1.3.2 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.9.1 // indirect
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
	k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
	k8s.io/client-go v0.0.0-20181213151034-8d9ed539ba31
	k8s.io/klog v0.2.0
	k8s.io/kube-openapi v0.0.0-20180711000925-0cf8f7e6ed1d
	sigs.k8s.io/controller-runtime v0.1.10
	sigs.k8s.io/controller-tools v0.1.11-0.20190411181648-9d55346c2bde // indirect
	sigs.k8s.io/testing_frameworks v0.1.2 // indirect
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.36.0
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.1.0+incompatible
	github.com/BurntSushi/toml => github.com/BurntSushi/toml v0.3.1
	github.com/PuerkitoBio/purell => github.com/PuerkitoBio/purell v1.1.1
	github.com/PuerkitoBio/urlesc => github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578
	github.com/appscode/jsonpatch => github.com/appscode/jsonpatch v0.0.0-20190108182946-7c0e3b262f30
	github.com/beorn7/perks => github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973
	github.com/coreos/prometheus-operator => github.com/coreos/prometheus-operator v0.26.0
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v1.1.1
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v2.9.0+incompatible
	github.com/evanphx/json-patch => github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/fsnotify/fsnotify => github.com/fsnotify/fsnotify v1.4.7
	github.com/ghodss/yaml => github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr => github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.1.1
	github.com/go-openapi/jsonpointer => github.com/go-openapi/jsonpointer v0.18.0
	github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.18.0
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.19.0
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.18.0
	github.com/gobuffalo/envy => github.com/gobuffalo/envy v1.6.15
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.2.1
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache => github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef
	github.com/golang/mock => github.com/golang/mock v1.3.1-0.20190508161146-9fa652df1129
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.0
	github.com/google/btree => github.com/google/btree v1.0.0
	github.com/google/gofuzz => github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf
	github.com/google/uuid => github.com/google/uuid v1.1.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.2.0
	github.com/gophercloud/gophercloud => github.com/gophercloud/gophercloud v0.1.0
	github.com/gregjones/httpcache => github.com/gregjones/httpcache v0.0.0-20190212212710-3befbb6ad0cc
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.0
	github.com/hpcloud/tail => github.com/hpcloud/tail v1.0.0
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.7
	github.com/inconshreveable/mousetrap => github.com/inconshreveable/mousetrap v1.0.0
	github.com/joho/godotenv => github.com/joho/godotenv v1.3.0
	github.com/json-iterator/go => github.com/json-iterator/go v1.1.5
	github.com/konsorten/go-windows-terminal-sequences => github.com/konsorten/go-windows-terminal-sequences v1.0.2
	github.com/mailru/easyjson => github.com/mailru/easyjson v0.0.0-20190221075403-6243d8e04c3f
	github.com/markbates/inflect => github.com/markbates/inflect v1.0.4
	github.com/matttproud/golang_protobuf_extensions => github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/mitchellh/go-homedir => github.com/mitchellh/go-homedir v1.1.0
	github.com/modern-go/concurrent => github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 => github.com/modern-go/reflect2 v0.0.0-20180701023420-4b7aa43c6742
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega => github.com/onsi/gomega v1.5.0
	github.com/operator-framework/operator-sdk => github.com/operator-framework/operator-sdk v0.8.0
	github.com/pborman/uuid => github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709
	github.com/peterbourgon/diskv => github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pkg/errors => github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90
	github.com/prometheus/common => github.com/prometheus/common v0.2.0
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.0-20190225181712-6ed1f7e10411
	github.com/rogpeppe/go-internal => github.com/rogpeppe/go-internal v1.2.2
	github.com/sirupsen/logrus => github.com/sirupsen/logrus v1.3.0
	github.com/spf13/afero => github.com/spf13/afero v1.2.1
	github.com/spf13/cobra => github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag => github.com/spf13/pflag v1.0.3
	go.uber.org/atomic => go.uber.org/atomic v1.3.2
	go.uber.org/multierr => go.uber.org/multierr v1.1.0
	go.uber.org/zap => go.uber.org/zap v1.9.1
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20190225124518-7f87c0fbb88b
	golang.org/x/net => golang.org/x/net v0.0.0-20190227022144-312bce6e941f
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20190226205417-e64efc72b421
	golang.org/x/sync => golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190226215855-775f8194d0f9
	golang.org/x/text => golang.org/x/text v0.3.0
	golang.org/x/time => golang.org/x/time v0.0.0-20181108054448-85acf8d2951c
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190226205152-f727befe758c
	google.golang.org/appengine => google.golang.org/appengine v1.4.0
	gopkg.in/inf.v0 => gopkg.in/inf.v0 v0.9.1
	gopkg.in/tomb.v1 => gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.2
	k8s.io/api => k8s.io/api v0.0.0-20181213150558-05914d821849
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20181213153335-0fe22c71c476
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20181127025237-2b1284ed4c93
	k8s.io/client-go => k8s.io/client-go v0.0.0-20181213151034-8d9ed539ba31
	k8s.io/gengo => k8s.io/gengo v0.0.0-20190128074634-0689ccc1d7d6
	k8s.io/klog => k8s.io/klog v0.2.0
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20180711000925-0cf8f7e6ed1d
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.1.10
	sigs.k8s.io/controller-tools => sigs.k8s.io/controller-tools v0.1.11-0.20190411181648-9d55346c2bde
	sigs.k8s.io/yaml => sigs.k8s.io/yaml v1.1.0
)
