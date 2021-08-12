package controllers

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nodemaintenancev1beta1 "github.com/kubevirt/node-maintenance-operator/api/v1beta1"
)

var _ = Describe("updateCondition", func() {

	var r *NodeMaintenanceReconciler
	var nm *nodemaintenancev1beta1.NodeMaintenance
	var cl client.Client
	var cs *k8sfakeclient.Clientset
	var req reconcile.Request

	setFakeClients := func(s *runtime.Scheme) {
		var objs []runtime.Object

		nm, objs = getCommonTestObjs()

		clObjs := append(objs, nm)

		cl = fake.NewClientBuilder().WithRuntimeObjects(clObjs...).Build()
		cs = k8sfakeclient.NewSimpleClientset(objs...)
	}

	checkSuccesfulReconcile := func() {
		maintanance := &nodemaintenancev1beta1.NodeMaintenance{}
		err := cl.Get(context.TODO(), req.NamespacedName, maintanance)
		Expect(err).NotTo(HaveOccurred())
		Expect(maintanance.Status.Phase).To(Equal(nodemaintenancev1beta1.MaintenanceSucceeded))
	}

	checkFailedReconcile := func() {
		maintanance := &nodemaintenancev1beta1.NodeMaintenance{}
		err := cl.Get(context.TODO(), req.NamespacedName, maintanance)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(maintanance.Status.LastError)).NotTo(Equal(0))
	}

	reconcileMaintenance := func(nm *nodemaintenancev1beta1.NodeMaintenance) {
		r.Reconcile(context.Background(), req)
	}

	taintExist := func(node *corev1.Node, key string, effect corev1.TaintEffect) bool {
		checkTaint := corev1.Taint{
			Key:    key,
			Effect: effect,
		}
		taints := node.Spec.Taints
		for _, taint := range taints {
			if reflect.DeepEqual(taint, checkTaint) {
				return true
			}
		}
		return false
	}

	BeforeEach(func() {

		s := scheme.Scheme
		nodemaintenancev1beta1.AddToScheme(s)

		setFakeClients(s)

		// Create a ReconcileNodeMaintenance object with the scheme and fake client
		r = &NodeMaintenanceReconciler{Client: cl, Scheme: s}
		initDrainer(r, &rest.Config{})
		r.drainer.Client = cs

		// Mock request to simulate Reconcile() being called on an event for a
		// watched resource .
		req = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: nm.ObjectMeta.Name,
			},
		}

	})

	Context("Node maintenance controller initialization test", func() {

		It("Node maintenance should be initialized properly", func() {
			r.initMaintenanceStatus(nm)
			maintenance := &nodemaintenancev1beta1.NodeMaintenance{}
			err := cl.Get(context.TODO(), req.NamespacedName, maintenance)
			Expect(err).NotTo(HaveOccurred())
			Expect(maintenance.Status.Phase).To(Equal(nodemaintenancev1beta1.MaintenanceRunning))
			Expect(len(maintenance.Status.PendingPods)).To(Equal(2))
			Expect(maintenance.Status.EvictionPods).To(Equal(2))
			Expect(maintenance.Status.TotalPods).To(Equal(2))
		})
		It("owner ref should be set properly", func() {
			r.initMaintenanceStatus(nm)
			maintanance := &nodemaintenancev1beta1.NodeMaintenance{}
			err := cl.Get(context.TODO(), req.NamespacedName, maintanance)
			node, err := cs.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			setOwnerRefToNode(maintanance, node)
			Expect(len(maintanance.ObjectMeta.GetOwnerReferences())).To(Equal(1))
			ref := maintanance.ObjectMeta.GetOwnerReferences()[0]
			Expect(ref.Name).To(Equal(node.ObjectMeta.Name))
			Expect(ref.UID).To(Equal(node.ObjectMeta.UID))
			Expect(ref.APIVersion).To(Equal(node.TypeMeta.APIVersion))
			Expect(ref.Kind).To(Equal(node.TypeMeta.Kind))
			setOwnerRefToNode(maintanance, node)
			Expect(len(maintanance.ObjectMeta.GetOwnerReferences())).To(Equal(1))
		})
		It("Should not init Node maintenance if already set", func() {
			nmCopy := nm.DeepCopy()
			nmCopy.Status.Phase = nodemaintenancev1beta1.MaintenanceRunning
			r.initMaintenanceStatus(nmCopy)
			maintanance := &nodemaintenancev1beta1.NodeMaintenance{}
			err := cl.Get(context.TODO(), req.NamespacedName, maintanance)
			Expect(err).NotTo(HaveOccurred())
			Expect(maintanance.Status.Phase).NotTo(Equal(nodemaintenancev1beta1.MaintenanceRunning))
			Expect(len(maintanance.Status.PendingPods)).NotTo(Equal(2))
			Expect(maintanance.Status.EvictionPods).NotTo(Equal(2))
			Expect(maintanance.Status.TotalPods).NotTo(Equal(2))
		})

	})

	Context("Node maintenance controller taint function test", func() {
		It("should add kubevirt NoSchedule taint and keep other existing taints", func() {
			node, err := cs.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			AddOrRemoveTaint(cs, node, true)
			taintedNode, err := cs.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(taintExist(taintedNode, "kubevirt.io/drain", corev1.TaintEffectNoSchedule)).To(Equal(true))
			Expect(taintExist(taintedNode, "node.kubernetes.io/unschedulable", corev1.TaintEffectNoSchedule)).To(Equal(true))
			Expect(taintExist(taintedNode, "test", corev1.TaintEffectPreferNoSchedule)).To(Equal(true))
			Expect(len(taintedNode.Spec.Taints)).To(Equal(3))
		})

		It("should remove kubevirt NoSchedule taint and keep other existing taints", func() {
			node, err := cs.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(taintExist(node, "kubevirt.io/drain", corev1.TaintEffectNoSchedule)).To(BeFalse())
			AddOrRemoveTaint(cs, node, true)
			taintedNode, err := cs.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(taintExist(taintedNode, "kubevirt.io/drain", corev1.TaintEffectNoSchedule)).To(Equal(true))
			AddOrRemoveTaint(cs, taintedNode, false)
			unTaintedNode, err := cs.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(taintExist(unTaintedNode, "kubevirt.io/drain", corev1.TaintEffectNoSchedule)).To(BeFalse())
			Expect(taintExist(unTaintedNode, "test", corev1.TaintEffectPreferNoSchedule)).To(Equal(true))
			Expect(len(unTaintedNode.Spec.Taints)).To(Equal(1))
		})
	})

	Context("Node maintenance controller reconciles a maintenance CR for a node in the cluster", func() {

		It("should reconcile once without failing", func() {
			reconcileMaintenance(nm)
			checkSuccesfulReconcile()
		})
		It("should reconcile and cordon node", func() {
			reconcileMaintenance(nm)
			checkSuccesfulReconcile()
			node, err := cs.CoreV1().Nodes().Get(context.TODO(), nm.Spec.NodeName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Spec.Unschedulable).To(Equal(true))
		})
		It("should reconcile and taint node", func() {
			reconcileMaintenance(nm)
			checkSuccesfulReconcile()
			node, err := cs.CoreV1().Nodes().Get(context.TODO(), nm.Spec.NodeName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(taintExist(node, "kubevirt.io/drain", corev1.TaintEffectNoSchedule)).To(Equal(true))
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
