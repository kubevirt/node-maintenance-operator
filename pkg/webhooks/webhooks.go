package webhooks

import (
	"context"
	"os"
	"time"
	"fmt"
	"strconv"
	"strings"
	"net"
	"net/http"
	"encoding/json"
	"io/ioutil"
	"crypto/tls"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/api/errors"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	DefaultListenPort =  8454
	DefaultListenAddress = "0.0.0.0"
	ShutdownTimeoutInSeconds = 30 * time.Second
	defaultKeepAlivePeriod = 5 * time.Minute
	WebhookServiceName = "webhook"
	CreationHookUrl = "/creation-hook"
)

type WebhookCallback func(runtime.Object) error

type WebHooks struct {
	ListenIp		string
	ListenIpAndPort string
	ListenPort		int
	Client			kubernetes.Interface
	ErrChan			chan error
	GroupVersion	schema.GroupVersion
	PluralName		string
	Namespace		string
	ServiceName		string
	UrlPath			string
	WebhookCallback WebhookCallback
	Codecs			serializer.CodecFactory
}


func NewWebHooks(config *rest.Config, scheme *runtime.Scheme, groupVersion schema.GroupVersion, plurarlName string, namespace string, callback WebhookCallback) (*WebHooks, error) {

	codecs := serializer.NewCodecFactory(scheme)

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	svcName := WebhookServiceName + groupVersion.Group + groupVersion.Version
	svcName = strings.Replace(svcName, ".", "-", -1)

	return &WebHooks{
			Client: cs,
			GroupVersion: groupVersion,
			PluralName: plurarlName,
			Namespace: namespace,
			WebhookCallback: callback,
			ServiceName:  svcName,
			UrlPath: CreationHookUrl,
			Codecs: codecs,

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
		listenIp = DefaultListenAddress;
	}

	ipAndPort := fmt.Sprintf("%s:%s", listenIp, listenPort)

	obj.ListenIpAndPort = ipAndPort
	obj.ListenPort = numListenPort
	obj.ListenIp = listenIp

	return nil
}



func (obj *WebHooks) initService() (*corev1.Service) {
	return  &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.ServiceName,
			Namespace: obj.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:     int32(443),
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(obj.ListenPort),
					},
				},
			},
			Selector: map[string]string {
				// Note: the service needs a matching label to the pod definition
				// otherwise IP traffic is not routed to the pod.
				// somehow the pod of the operator gets this label, but I am not exactly sure why.
				"name": obj.Namespace,
			},
		},
	}

}

func (obj *WebHooks) registerService() error {
	svc := obj.initService()

//	err := obj.Client.CoreV1().Services(obj.Namespace).Delete(obj.ServiceName,&metav1.DeleteOptions{})
//	if err != nil {
//		if !errors.IsNotFound(err) {
//			return fmt.Errorf("can't delete service: %v",err)
//		}
//	}

	_, err := obj.Client.CoreV1().Services(obj.Namespace).Create(svc)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	log.Info("Service object created")
	return nil

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

	hostname := obj.ServiceName + "." + obj.Namespace + ".svc"
	cert, key, err := certutil.GenerateSelfSignedCertKeyWithFixtures(hostname, alternateIPs, alternateDNS, "")

	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate self signed cert: %v", err)
	}

	log.Debugf("Key: %s",key)
	log.Debugf("Cert: %s",cert)

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
					Resources:   []string{obj.PluralName},
				},
			}},
			ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
				Service: &admissionregistrationv1beta1.ServiceReference{
					Namespace: obj.Namespace,
					Name:      obj.ServiceName,
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

func handleCRDAdmissionRequest(ar v1beta1.AdmissionReview, obj *WebHooks ) (*v1beta1.AdmissionResponse) {

	if ar.Kind == "AdmissionReview" &&	ar.APIVersion != "admission.k8s.io/v1beta1"  {
		err := fmt.Errorf("wrong admission review object. actual kind %s apiversion %s", ar.Kind, ar.APIVersion )
		log.Error(err)
		return makeAddmissionErrorResponse(err)
	}

	log.Debug("deserializing admission request")
	if ar.Request.Object.Object == nil {
		var err error

		ar.Request.Object.Object, _, err = obj.Codecs.UniversalDeserializer().Decode(ar.Request.Object.Raw, nil, nil)
		if err != nil {
			return makeAddmissionErrorResponse(fmt.Errorf("failed to deserialize request obj : %v", err))
		}
	}

	if ar.Request.Object.Object == nil {
		return makeAddmissionErrorResponse(fmt.Errorf("failed to deserialize request obj - nil object"))
	}

	err := obj.WebhookCallback(ar.Request.Object.Object)
	if err != nil {
		log.Errorf("Error while handling callback: %v", err)
		return makeAddmissionErrorResponse(err)
	}

	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	log.Debug("crd admission validated!")

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

		log.Debug("service handler called")

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

		log.Debug(fmt.Sprintf("handling request body: %s", body))

		// The AdmissionReview that was sent to the webhook
		requestedAdmissionReview := v1beta1.AdmissionReview{}

		responseAdmissionReview := v1beta1.AdmissionReview{}

		deserializer := obj.Codecs.UniversalDeserializer()
		if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
			log.Error(err)
			responseAdmissionReview.Response = makeAddmissionErrorResponse(err)
		} else {
			// pass to admitFunc
			responseAdmissionReview.Response = handleCRDAdmissionRequest(requestedAdmissionReview, obj)
		}

		// Return the same UID
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

		log.Debug(fmt.Sprintf("sending response: %v", responseAdmissionReview.Response))

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
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{keyPair},
		},
	}

	keepAliveListener := tcpKeepAliveListener{impl: netListener.(*net.TCPListener)}
	tlsListener := tls.NewListener(keepAliveListener, secureServer.TLSConfig)

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

		log.Infof("web hook server: start listening on %s", tlsListener.Addr().String())
		err := secureServer.Serve(tlsListener)
		msg := fmt.Sprintf("Stopped listening on %s", tlsListener.Addr().String())
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

func StartAdmissionWebhook(config *rest.Config, scheme *runtime.Scheme, stopChan <-chan struct{}, gvk schema.GroupVersion, plurarlName string, namespace string, callback WebhookCallback) error {

	obj, err := NewWebHooks(config, scheme, gvk, plurarlName, namespace, callback)
	if err != nil {
		return err
	}

	err = obj.Start(stopChan)
	if err != nil {
		return err
	}
	return nil
}

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
		log.Debugf("TCP listener accept error?: %v", err)
	} else {
		log.Debug("TCP listener accepted!")
	}

	return tc, err
}

func (ln tcpKeepAliveListener) Close() error {
	err := ln.impl.Close()

	if err != nil {
		log.Debug("TCP listener close")
	} else {
		log.Debugf("TCP connection close error: %v", err)
	}
	return err
}

func (ln tcpKeepAliveListener) Addr() net.Addr {
	return ln.impl.Addr()
}


