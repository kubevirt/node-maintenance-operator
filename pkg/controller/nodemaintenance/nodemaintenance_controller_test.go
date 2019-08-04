package nodemaintenance

import (
	"context"
	reflect "reflect"
	"time"

	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"

	k8sfakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	kubevirtv1alpha1 "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("updateCondition", func() {

	var r *ReconcileNodeMaintenance
	var ctrl *gomock.Controller
	var mockMaintenanceReconcile *MockReconcileHandler
	var nm *kubevirtv1alpha1.NodeMaintenance
	var cl client.Client
	var cs *k8sfakeclient.Clientset
	var req reconcile.Request

	setFakeClients := func() {
		nm = &kubevirtv1alpha1.NodeMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-maintanance",
			},
			Spec: kubevirtv1alpha1.NodeMaintenanceSpec{
				NodeName: "node01",
				Reason:   "test reason",
			},
		}

		objs := []runtime.Object{
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node01",
				},
			},
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node02",
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					NodeName: "node01",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		}
		clObjs := append(objs, nm)
		cl = fake.NewFakeClient(clObjs...)
		cs = k8sfakeclient.NewSimpleClientset(objs...)
	}

	checkSuccesfulReconcile := func() {
		maintanance := &kubevirtv1alpha1.NodeMaintenance{}
		err := cl.Get(context.TODO(), req.NamespacedName, maintanance)
		Expect(err).NotTo(HaveOccurred())
		Expect(maintanance.Status.Phase).To(Equal(kubevirtv1alpha1.MaintenanceSucceeded))
	}

	checkFailedReconcile := func() {
		maintanance := &kubevirtv1alpha1.NodeMaintenance{}
		err := cl.Get(context.TODO(), req.NamespacedName, maintanance)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(maintanance.Status.LastError)).NotTo(Equal(0))
	}

	reconcileMaintenance := func(nm *kubevirtv1alpha1.NodeMaintenance) {
		// Mock request to simulate Reconcile() being called on an event for a
		// watched resource .
		req = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: nm.ObjectMeta.Name,
			},
		}
		r.Reconcile(req)
	}

	kubevirtTaintExist := func(node *corev1.Node) bool {
		kubevirtDrainTaint := corev1.Taint{
			Key:    "kubevirt.io/drain",
			Effect: corev1.TaintEffectNoSchedule,
		}
		taints := node.Spec.Taints
		for _, taint := range taints {
			if reflect.DeepEqual(taint, kubevirtDrainTaint) {
				return true
			}
		}
		return false
	}

	BeforeEach(func() {

		ctrl = gomock.NewController(GinkgoT())
		mockMaintenanceReconcile = NewMockReconcileHandler(ctrl)
		Handler = mockMaintenanceReconcile

		s := scheme.Scheme
		s.AddKnownTypes(kubevirtv1alpha1.SchemeGroupVersion, nm)

		setFakeClients()

		kubeSharedInformer := informers.NewSharedInformerFactoryWithOptions(cs, 2*time.Minute)
		fakePodInformer := kubeSharedInformer.Core().V1().Pods()

		// Create a ReconcileNodeMaintenance object with the scheme and fake client
		r = &ReconcileNodeMaintenance{client: cl, scheme: s, podInformer: fakePodInformer.Informer()}
		initDrainer(r, &rest.Config{})
		r.drainer.Client = cs
		mockMaintenanceReconcile.EXPECT().StartPodInformer(gomock.Any(), gomock.Any()).Return(nil)
	})
	Context("Node maintenanace controller reconciles a maintenanace CR for a node in the cluster", func() {

		It("should reconcile once without failing", func() {
			reconcileMaintenance(nm)
			checkSuccesfulReconcile()
		})
		It("should reconcile and cordon node", func() {
			reconcileMaintenance(nm)
			checkSuccesfulReconcile()
			node, err := cs.CoreV1().Nodes().Get(nm.Spec.NodeName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Spec.Unschedulable).To(Equal(true))
		})
		It("should reconcile and taint node", func() {
			reconcileMaintenance(nm)
			checkSuccesfulReconcile()
			node, err := cs.CoreV1().Nodes().Get(nm.Spec.NodeName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kubevirtTaintExist(node)).To(Equal(true))
		})
		It("should fail on non existing node", func() {
			nmCopy := nm.DeepCopy()
			nmCopy.Spec.NodeName = "non-existing"
			err := cl.Delete(context.TODO(), nm)
			Expect(err).NotTo(HaveOccurred())
			err = cl.Create(context.TODO(), nmCopy)
			Expect(err).NotTo(HaveOccurred())
			reconcileMaintenance(nmCopy)
			checkFailedReconcile()
		})

	})
})
