package webhooks

import (
	"context"
	"os"
	"time"
	"fmt"
	"strconv"
	"net"
	"net/http"
	"encoding/json"
	"io/ioutil"
	"crypto/tls"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/api/errors"
	//certutil "k8s.io/client-go/util/cert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	cryptorand "crypto/rand"
	"encoding/pem"
	"strings"
	"path"
)

const CertificateBlockType = "CERTIFICATE"
const RSAPrivateKeyBlockType = "RSA PRIVATE KEY"

const (
	//DefaultListenPort =  443
	DefaultListenPort =  8454
	DefaultListenAddress = "0.0.0.0" //"10.99.25.77"
	ShutdownTimeoutInSeconds = 30 * time.Second
	defaultKeepAlivePeriod = 5 * time.Minute
	WebhookServiceName = "webhook"
)


var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)



type WebhookCallback interface {
	validateCRDAdmission(runtime.Object) error
}

type WebHooks struct {
	ListenIp	string
	ListenIpAndPort string
	ListenPort int
	Client     kubernetes.Interface
	ErrChan chan error

	GroupVersion schema.GroupVersion
	Namespace string
	UrlPath string
	WebhookCallback WebhookCallback
}

func getServiceIP(KubeClient kubernetes.Interface, serviceName string, namespaceName string) (string, error) {

	pods, err := KubeClient.CoreV1().Pods("node-maintenance-operator").List(metav1.ListOptions{LabelSelector: "name=node-maintenance-operator"})
	if err != nil {
		return "", err
	}

	if pods.Size() == 0 {
		return "", fmt.Errorf("There are no pods deployed in cluster to run the operator")
	}

	return pods.Items[0].Status.PodIP, nil
}

/*
func getServiceIP(KubeClient kubernetes.Interface, serviceName string, namespaceName string) (string, error) {

	svc, err := KubeClient.CoreV1().Services(namespaceName).Get(serviceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return svc.Spec.ClusterIP, nil
}
*/

func NewWebHooks(config *rest.Config, groupVersion schema.GroupVersion, namespace string, callback WebhookCallback) (*WebHooks, error) {

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &WebHooks{
			Client: cs,
			GroupVersion: groupVersion,
			Namespace: namespace,
			UrlPath: "/creation-hook",
			WebhookCallback: callback,
		}, nil
}

func (obj *WebHooks)  readIpAndPort() error {

	listenPort := os.Getenv("LISTEN_PORT")
	if listenPort == "" {
		listenPort = strconv.Itoa(DefaultListenPort)
	}
	numListenPort, err := strconv.Atoi(listenPort)
	if err != nil {
		return fmt.Errorf("invalid listen port %s : %v", listenPort, err)
	}

	listenIp := os.Getenv("LISTEN_ADDRESS")
	if listenIp == "" {
		var err error

		listenIp, err = getServiceIP(obj.Client,WebhookServiceName,obj.Namespace)
		if err != nil {
			return fmt.Errorf("failed to service address: %v", err)
		}
		//listenIp = DefaultListenAddress;
	}

	ipAndPort := fmt.Sprintf("%s:%s", listenIp, listenPort)

	obj.ListenIpAndPort = ipAndPort
	obj.ListenPort = numListenPort
	obj.ListenIp = listenIp

	log.Infof("webhook listening address: %s", ipAndPort)

	return nil
}



func (obj *WebHooks) initService() (*corev1.Service) {
	return  &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WebhookServiceName,
			Namespace: obj.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:     int32(obj.ListenPort),
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(obj.ListenPort),
					},
				},
			},
			Selector: map[string]string {
				"webhook": "true",
			},
			//Type: corev1.ServiceTypeNodePort, //ServiceTypeClusterIP,
		},
	}

}

func (obj *WebHooks) registerService() error {
	svc := obj.initService()

	err := obj.Client.CoreV1().Services(obj.Namespace).Delete(WebhookServiceName,&metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("can't delete service: %v",err)
	}

	_, err = obj.Client.CoreV1().Services(obj.Namespace).Create(svc)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	log.Info("Service object created")
	return nil

}


func GenerateSelfSignedCertKeyWithFixtures(host string, alternateIPs []net.IP, alternateDNS []string, fixtureDirectory string) ([]byte, []byte, error) {
	validFrom := time.Now().Add(-time.Hour) // valid an hour earlier to avoid flakes due to clock skew
	maxAge := time.Hour * 24 * 365          // one year self-signed certs

	baseName := fmt.Sprintf("%s_%s_%s", host, strings.Join(ipsToStrings(alternateIPs), "-"), strings.Join(alternateDNS, "-"))
	certFixturePath := path.Join(fixtureDirectory, baseName+".crt")
	keyFixturePath := path.Join(fixtureDirectory, baseName+".key")
	if len(fixtureDirectory) > 0 {
		cert, err := ioutil.ReadFile(certFixturePath)
		if err == nil {
			key, err := ioutil.ReadFile(keyFixturePath)
			if err == nil {
				return cert, key, nil
			}
			return nil, nil, fmt.Errorf("cert %s can be read, but key %s cannot: %v", certFixturePath, keyFixturePath, err)
		}
		maxAge = 100 * time.Hour * 24 * 365 // 100 years fixtures
	}

	caKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	/*
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s-ca", host),
		},
		NotBefore: validFrom,
		NotAfter:  validFrom.Add(maxAge),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	caCertificate, err := x509.ParseCertificate(caDERBytes)
	if err != nil {
		return nil, nil, err
	}
	*/

	priv, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Issuer: pkix.Name{
			CommonName: fmt.Sprintf("%s", host),
		},
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s", host),
		},
		NotBefore: validFrom,
		NotAfter:  validFrom.Add(maxAge),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, host)
	}

	template.IPAddresses = append(template.IPAddresses, alternateIPs...)
	template.DNSNames = append(template.DNSNames, alternateDNS...)

	derBytes, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, &priv.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	// Generate cert, followed by ca
	certBuffer := bytes.Buffer{}
	if err := pem.Encode(&certBuffer, &pem.Block{Type: CertificateBlockType, Bytes: derBytes}); err != nil {
		return nil, nil, err
	}
	/*
	if err := pem.Encode(&certBuffer, &pem.Block{Type: CertificateBlockType, Bytes: caDERBytes}); err != nil {
		return nil, nil, err
	}
	*/

	// Generate key
	keyBuffer := bytes.Buffer{}
	if err := pem.Encode(&keyBuffer, &pem.Block{Type: RSAPrivateKeyBlockType, Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return nil, nil, err
	}

	if len(fixtureDirectory) > 0 {
		if err := ioutil.WriteFile(certFixturePath, certBuffer.Bytes(), 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write cert fixture to %s: %v", certFixturePath, err)
		}
		if err := ioutil.WriteFile(keyFixturePath, keyBuffer.Bytes(), 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write key fixture to %s: %v", certFixturePath, err)
		}
	}

	return certBuffer.Bytes(), keyBuffer.Bytes(), nil
}

func ipsToStrings(ips []net.IP) []string {
	ss := make([]string, 0, len(ips))
	for _, ip := range ips {
		ss = append(ss, ip.String())
	}
	return ss
}
func (obj *WebHooks) CreateSelfSignedCert() ([]byte,[]byte,error) {

	var alternateDNS []string
	var  alternateIPs []net.IP

	// add either the bind address or localhost to the valid alternates
	if obj.ListenIp == "0.0.0.0" {
		alternateDNS = append(alternateDNS, "localhost")
	} else {
		netIp := net.ParseIP(obj.ListenIp)
		if netIp == nil {
			return nil, nil, fmt.Errorf("Can't parse %s to ip", netIp)
		}
		alternateIPs = append(alternateIPs, netIp)
	}

	hostname := WebhookServiceName + "." + obj.Namespace + ".svc"
	cert, key, err := GenerateSelfSignedCertKeyWithFixtures(hostname, alternateIPs, alternateDNS, "")

	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate self signed cert: %v", err)
	}

	log.Infof("Key: %s",key)
	log.Infof("Cert: %s",cert)

	return cert, key, nil
}

func (obj *WebHooks) createValidatingHook(certPem []byte) ([]admissionregistrationv1beta1.ValidatingWebhook) {

	groupVersion := fmt.Sprintf("%s.%s",obj.GroupVersion.Group, obj.GroupVersion.Version)

	failurePolicy := admissionregistrationv1beta1.Fail

	webHooks := []admissionregistrationv1beta1.ValidatingWebhook{
		{
			Name:          groupVersion,
			FailurePolicy: &failurePolicy,
			Rules: []admissionregistrationv1beta1.RuleWithOperations{{
				Operations: []admissionregistrationv1beta1.OperationType{
					admissionregistrationv1beta1.Create,
				},
				Rule: admissionregistrationv1beta1.Rule{
					APIGroups:   []string{obj.GroupVersion.Group},
					APIVersions: []string{obj.GroupVersion.Version},
					Resources:   []string{"nodemaintenances"},
				},
			}},
			ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
				Service: &admissionregistrationv1beta1.ServiceReference{
					Namespace: obj.Namespace,
					Name:      WebhookServiceName,
					Path:      &obj.UrlPath,
				},
				CABundle: certPem,
			},
		},
	}

	return webHooks
}

func (obj *WebHooks) registerAdmissionObj(certPem []byte, keyPem []byte) error {

	webhookObjName := fmt.Sprintf("%s.%s",obj.GroupVersion.Group, obj.GroupVersion.Version)

	webhookCfg, err := obj.Client.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(webhookObjName, metav1.GetOptions{} )

	webHooks := obj.createValidatingHook(certPem)

	if err != nil {
		if !errors.IsNotFound(err) {
			return err;
		}

		newWebhookCfg := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: webhookObjName,
			},
			Webhooks: webHooks,
		}
		_, err = obj.Client.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Create( newWebhookCfg )

	} else {

		// update registered webhook with our data
		webhookCfg.Webhooks = webHooks

		_, err = obj.Client.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Update( webhookCfg )
	}

	return err
}

func (obj *WebHooks) Start(stopChan <-chan struct{}) error {

	err := obj.readIpAndPort()
	if err != nil {
		return fmt.Errorf("webhooks: Can't get ip and port : %v", err)
	}

	err = obj.registerService()
	if err != nil {
		return fmt.Errorf("webhooks: can't create service object: %v", err)
	}

	certPem, keyPem, err := obj.CreateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("webhooks: can't create self signed certificate pair: %v", err)
	}


	err = obj.registerAdmissionObj(certPem, keyPem)
	if err != nil {
		return fmt.Errorf("webhooks: can't create or register admission object: %v", err)
	}

	mux := http.NewServeMux()
	return obj.RunHttpsServer(stopChan, mux, certPem, keyPem)

}

func makeAddmissionErrorResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func handleCRDAdmissionRequest(ar v1beta1.AdmissionReview, cb WebhookCallback ) (*v1beta1.AdmissionResponse) {

	crdResource := metav1.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1beta1", Resource: "customresourcedefinitions"}
	if ar.Request.Resource != crdResource {
		err := fmt.Errorf("expect resource to be %s actual %s", crdResource, ar.Request.Resource)
		log.Error(err)
		return makeAddmissionErrorResponse(err)
	}

	log.Info("deserializing admission request")

	raw := ar.Request.Object.Raw
	crd := apiextensionsv1beta1.CustomResourceDefinition{}
	deserializer := codecs.UniversalDeserializer()
	obj, _, err := deserializer.Decode(raw, nil, &crd)

	if err != nil {
		log.Errorf("Failed to deserialize CRD object: %v", err)
		return makeAddmissionErrorResponse(err)
	}

	err = cb.validateCRDAdmission(obj)
	if err != nil {
		log.Errorf("Error while handling callback: %v", err)
		return makeAddmissionErrorResponse(err)
	}

	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	log.Info("crd admission validated")

	return &reviewResponse
}

func (obj *WebHooks) RunHttpsServer(stopCh <-chan struct{}, handler *http.ServeMux, certPem []byte, keyPem []byte) error {

	keyPair, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		return fmt.Errorf("Can't create keypair for webhook: %v", err)
	}

	var netListener net.Listener

	netListener, err = net.Listen("tcp4", obj.ListenIpAndPort)
	if err != nil {
		return fmt.Errorf("failed to listen on %s : error %v",obj.ListenIpAndPort, err)
	}

	admissionHandler := func(w http.ResponseWriter, r *http.Request) {

		log.Info("service handler called")

		var body []byte
		if r.Body != nil {
			if data, err := ioutil.ReadAll(r.Body); err == nil {
				body = data
			} else {
				log.Infof("handler: can't read request body %v", err)
			}
		}

		// verify the content type is accurate
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			log.Errorf("wrong contentType expect application/json. contentType=%s ", contentType)
			return
		}

		log.Info(fmt.Sprintf("handling request body: %s", body))

		// The AdmissionReview that was sent to the webhook
		requestedAdmissionReview := v1beta1.AdmissionReview{}

		responseAdmissionReview := v1beta1.AdmissionReview{}

		deserializer := codecs.UniversalDeserializer()
		if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
			log.Error(err)
			responseAdmissionReview.Response = makeAddmissionErrorResponse(err)
		} else {
			// pass to admitFunc
			responseAdmissionReview.Response = handleCRDAdmissionRequest(requestedAdmissionReview, obj.WebhookCallback)
		}

		// Return the same UID
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

		log.Info(fmt.Sprintf("sending response: %v", responseAdmissionReview.Response))

		respBytes, err := json.Marshal(responseAdmissionReview)
		if err != nil {
			log.Error(err)
		}
		if _, err := w.Write(respBytes); err != nil {
			log.Error(err)
		}


	}

	handler.Handle(obj.UrlPath, http.HandlerFunc(admissionHandler))


	secureServer := &http.Server{
		Addr:           obj.ListenIpAndPort,
		Handler:        handler,
		MaxHeaderBytes: 1 << 20,
		TLSConfig: &tls.Config{
			// Can't use SSLv3 because of POODLE and BEAST
			// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
			// Can't use TLSv1.1 because of RC4 cipher usage
			MinVersion: tls.VersionTLS12,
		},
	}

	secureServer.TLSConfig.Certificates = []tls.Certificate{keyPair}

	shutDownTimeout := time.Duration(ShutdownTimeoutInSeconds)
	// Shutdown server gracefully.
	stoppedCh := make(chan struct{})
	go func() {
		defer close(stoppedCh)
		<-stopCh
		ctx, cancel := context.WithTimeout(context.Background(), shutDownTimeout)
		secureServer.Shutdown(ctx)
		cancel()
	}()

	go func() {
		//defer utilruntime.HandleCrash()
		listener := tcpKeepAliveListener{impl: netListener.(*net.TCPListener)}

		log.Infof("web hook server: start listening on %s", listener.Addr().String())
		err := secureServer.Serve(listener)
		msg := fmt.Sprintf("Stopped listening on %s", listener.Addr().String())
		select {
		case <-stopCh:
			log.Info(msg)
		default:
			log.Errorf("Server exit: %s due to error: %v", msg, err)
		}
	}()
	time.Sleep(time.Second)

	return nil
}

func StartAdmissionWebhook(config *rest.Config, stopChan <-chan struct{}, gvk schema.GroupVersion, namespace string, callback WebhookCallback) error {

	obj, err := NewWebHooks(config, gvk, namespace, callback)
	if err != nil {
		return err
	}

	err = obj.Start(stopChan)
	if err != nil {
		return err
	}
	return nil
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
//
// Copied from Go 1.7.2 net/http/server.go
type tcpKeepAliveListener struct {
	impl *net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.impl.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(defaultKeepAlivePeriod)

	if err != nil {
		log.Info("TCP listener accepted")
	} else {
		log.Infof("TCP listener accept error: %v", err)
	}

	return tc, nil
}

func (ln tcpKeepAliveListener) Close() error {
	err := ln.impl.Close()

	if err != nil {
		log.Info("TCP listener close")
	} else {
		log.Infof("TCP connection close error: %v", err)
	}
	return err
}

func (ln tcpKeepAliveListener) Addr() net.Addr {
	addr := ln.impl.Addr()

	log.Info("TCP listener Addr")

	return addr
}


