package v1beta1

import(
	"context"
	"fmt"
	"os"
	"path/filepath"
	log "github.com/sirupsen/logrus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ webhook.Validator = &NodeMaintenance{}

const (
	WebhookPort     = 4543
	WebhookCertDir  = "/apiserver.local.config/certificates"
	WebhookCertName = "apiserver.crt"
	WebhookKeyName  = "apiserver.key"
)

func (r *NodeMaintenance) SetupWebhookWithManager(ctx context.Context, mgr ctrl.Manager) error {
	// Make sure the certificates are mounted, this should be handled by the OLM
	certs := []string{filepath.Join(WebhookCertDir, WebhookCertName), filepath.Join(WebhookCertDir, WebhookKeyName)}
	for _, fname := range certs {
		if _, err := os.Stat(fname); err != nil {
			log.Info("CSV certificates were not found, skipping webhook initialization")
			return nil
		}
	}

	bldr := ctrl.NewWebhookManagedBy(mgr).For(r)
	srv := mgr.GetWebhookServer()
	srv.CertDir = WebhookCertDir
	srv.CertName = WebhookCertName
	srv.KeyName = WebhookKeyName
	srv.Port = WebhookPort
	return bldr.Complete()
}

func (r *NodeMaintenance) ValidateCreate() error {
	nodeName := r.Spec.NodeName
	log.Infof("Webhook: validating creation of nmo object on node %s", nodeName)
	valid, err := isValidNodeName(nodeName)
	if err != nil {
		rerr := fmt.Errorf("failed to validate node name : %v", err)
		log.Errorf("Webhook : %v", rerr)
		return rerr
	}
	if !valid {
		rerr := fmt.Errorf("Can't create NMO object for node %s. The node does not exist", nodeName)
		log.Errorf("Webhook : %v", rerr)
		return rerr
	}

	workingOnNode, err := isNMOObjectWorkingOnNode(nodeName)
	if workingOnNode {
		rerr := fmt.Errorf("Can't create NMO object for node %s. NMO object already working with this node", nodeName)
		log.Errorf("Webhook : %v", rerr)
		return rerr
	}
	return nil
}

func (r *NodeMaintenance) ValidateUpdate(old runtime.Object) error {
	log.Infof("Webhook: ValidateUpdate")
	return nil
}

func (r *NodeMaintenance) ValidateDelete() error {
	log.Infof("Webhook: ValidateDelete")
	return nil
}

