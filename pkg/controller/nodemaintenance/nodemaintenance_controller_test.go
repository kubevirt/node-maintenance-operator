package nodemaintenance

import (
	"fmt"
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"time"

	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	k8sfakeclient "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("updateCondition", func() {

	var r *ReconcileNodeMaintenance
	var cl client.Client
	var cs *k8sfakeclient.Clientset

	setLogOn := func() {
		logrus.SetLevel(logrus.DebugLevel)
	}

	getNodeStatus := func(name types.NamespacedName) string {
		//node := &corev1.Node{}
		//cl.Get(context.TODO(), name , node)

		client := cs.Core().Nodes()
		node, _ := client.Get(name.Name, metav1.GetOptions{})

		if node.ObjectMeta.Annotations == nil {
			fmt.Printf("???")
			return ""
		}

		val := node.ObjectMeta.Annotations[NodeMaintenanceStatusAnnotation]
		fmt.Printf("node: %s:%s status: %s\n", name.Name, name.Namespace, val)
		return val
	}

	checkSpecExists := func(name types.NamespacedName) bool {
		//node := &corev1.Node{}
		//cl.Get(context.TODO(), name , node)

		client := cs.Core().Nodes()
		node, _ := client.Get(name.Name, metav1.GetOptions{})

		_, exists := node.ObjectMeta.Annotations[NodeMaintenanceSpecAnnotation]
		fmt.Printf("node: %s:%s annotation %s exists: %t\n", name.Name, name.Namespace, NodeMaintenanceSpecAnnotation, exists)
		return exists
	}

	checkTaintOk := func(name  types.NamespacedName,setOnOff bool) bool {
		client := cs.Core().Nodes()
		node1, err := client.Get(name.Name, metav1.GetOptions{})
		if err != nil {
		   panic("failed with client for get taint")
		}
                taintCount, taintSize := CountDesiredTaintOnNode(node1)
		fmt.Printf("taintCount %d taintSize %d", taintCount, taintSize)
		return (taintCount == taintSize && setOnOff) || (taintCount == 0 && !setOnOff)
	}

	getLeaseDuration := func(nodeName string) int32 {
		lease := &coordv1beta1.Lease{}

		nName := types.NamespacedName{Namespace: corev1.NamespaceNodeLease, Name: nodeName}

		cl.Get(context.TODO(), nName, lease)

		duration := *lease.Spec.LeaseDurationSeconds

		fmt.Printf("node: %s leaseDuration: %d\n", nodeName, duration )
		return duration
	}


	setFakeClients := func() {

		tmpDurationInSeconds := int32(3600)
		tmpDurationInSeconds301 := int32(301) // add padding

		leaseExpiredWithinPadding := "lease-expired-within-padding"
		leaseHolderMe := LeaseHolderIdentity
		timeExpiredWithinPadding := time.Now().Add(time.Duration(int64(5-tmpDurationInSeconds301-LeasePaddingSeconds) * int64(time.Second)))

		timeDurationInSeconds301WithPadding := tmpDurationInSeconds301 + LeasePaddingSeconds

		objs := []runtime.Object{
			// ReconcileToMaintStateActive test 2:
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-without-annotations",
				},
			},
			// ReconcileToMaintStateActive test 3:
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-without-relevant-annotations",
					Annotations: map[string]string{"different-annotation": "different-value"},
				},
			},
			// ReconcileToMaintStateActive test 4
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-without-illegal-maint-annotation",
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance": "not-a-number"},
				},
			},
			// ReconcileToMaintStateActive test 5
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-without-illegal-maint-to-small",
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance": "1"},
				},
				Spec: corev1.NodeSpec{
				},
			},
			// ReconcileToMaintStateActive test 6
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "lease-not-owned-by-me-active",
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance": "301"},
				},
			},
			&coordv1beta1.Lease{
				TypeMeta: metav1.TypeMeta{
					Kind: "Lease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lease-not-owned-by-me-active",
					Namespace: corev1.NamespaceNodeLease,
				},
				Spec: coordv1beta1.LeaseSpec{
					AcquireTime:          &metav1.MicroTime{Time: time.Now()},
					LeaseDurationSeconds: &tmpDurationInSeconds,
				},
			},
			// ReconcileToMaintStateActive test 7
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "lease-nf",
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance": "301"},
				},
			},
			// ReconcileToMaintStateActive test 8
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        leaseExpiredWithinPadding,
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance": "301"},
				},
			},
			&coordv1beta1.Lease{
				TypeMeta: metav1.TypeMeta{
					Kind: "Lease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      leaseExpiredWithinPadding,
					Namespace: corev1.NamespaceNodeLease,
				},
				Spec: coordv1beta1.LeaseSpec{
					AcquireTime:          &metav1.MicroTime{Time: timeExpiredWithinPadding},
					LeaseDurationSeconds: &timeDurationInSeconds301WithPadding,
					HolderIdentity:       &leaseHolderMe,
				},
			},

			// ReconcileToMainStatusEnded test 1
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "stat-active-valid-lease-me-owner",
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance-status": "active"},
				},
				Spec:  corev1.NodeSpec {

					Taints: []corev1.Taint{
						corev1.Taint{
							Key:    "kubevirt.io/drain",
							Effect: corev1.TaintEffectNoSchedule,
						},
						corev1.Taint{
							Key:    "node.kubernetes.io/unschedulable",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					Unschedulable: true,
				},
			},
			&coordv1beta1.Lease{
				TypeMeta: metav1.TypeMeta{
					Kind: "Lease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stat-active-valid-lease-me-owner",
					Namespace: corev1.NamespaceNodeLease,
				},
				Spec: coordv1beta1.LeaseSpec{
					AcquireTime:          &metav1.MicroTime{Time: time.Now()},
					LeaseDurationSeconds: &timeDurationInSeconds301WithPadding,
					HolderIdentity:       &leaseHolderMe,
				},
			},
			// ReconcileToMainStatusEnded test 2
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "stat-active-expired-lease-me-owner",
					Annotations: map[string]string{"lifecycle.openshift.io/maintenance-status": "active"},
				},
				Spec:  corev1.NodeSpec {

					Taints: []corev1.Taint {
						corev1.Taint{
							Key:    "kubevirt.io/drain",
							Effect: corev1.TaintEffectNoSchedule,
						},
						corev1.Taint{
							Key:    "node.kubernetes.io/unschedulable",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					Unschedulable: true,
				},
			},
			&coordv1beta1.Lease{
				TypeMeta: metav1.TypeMeta{
					Kind: "Lease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stat-active-expired-lease-me-owner",
					Namespace: corev1.NamespaceNodeLease,
				},
				Spec: coordv1beta1.LeaseSpec{
					AcquireTime:          &metav1.MicroTime{Time: time.Now().Add( time.Duration( (-1 * int64(timeDurationInSeconds301WithPadding) - 10) *  int64(time.Second)  ) ) },
					LeaseDurationSeconds: &timeDurationInSeconds301WithPadding,
					HolderIdentity:       &leaseHolderMe,

				},
			},
		}

		cl = fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
		cs = k8sfakeclient.NewSimpleClientset(objs...)
	}
	BeforeEach(func() {

		setFakeClients()

		f := &ClientFactoryTest{client: cs}

		// Create a ReconcileNodeMaintenance object with the scheme and fake client
		r = &ReconcileNodeMaintenance{client: cl, clientFactory: f}
		r.initDrainer()

	})

	setLogOn()

	Context("ReconcileToMaintStateActive", func() {

		It("node-not-found", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "no-such-test",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))
		})
		It("node without attributes", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "node-without-annotations",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))
		})
		It("node with non empty annotations, but not relevant ones", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "node-without-relevant-annotations",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))
		})
		It("node with non empty annotations, but invalid value of main-anntation", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "node-without-illegal-maint-annotation",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))
		})
		It("node with non empty annotations, but maintenance window to small", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "node-without-illegal-maint-to-small",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			hasSpec := checkSpecExists(req.NamespacedName)
			Expect(hasSpec).To(Equal(true))
		})
		It("valid maintenance window, lease with different owner and active", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "lease-not-owned-by-me-active",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}))

			status := getNodeStatus(req.NamespacedName)
			Expect(status).To(Equal(string(NodeStateWaiting)))

			hasSpec := checkSpecExists(req.NamespacedName)
			Expect(hasSpec).To(Equal(true))

		})
		It("valid maintenance window, lease not found", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "lease-nf",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			status := getNodeStatus(req.NamespacedName)
			Expect(status).To(Equal(string(NodeStateActive)))

			hasSpec := checkSpecExists(req.NamespacedName)
			Expect(hasSpec).To(Equal(true))
		})
		It("valid maintenance window, right holder, lease expired within padding", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "lease-expired-within-padding",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			status := getNodeStatus(req.NamespacedName)
			Expect(status).To(Equal(string(NodeStateEnded)))

			hasSpec := checkSpecExists(req.NamespacedName)
			Expect(hasSpec).To(Equal(false))
		})

	})

	Context("ReconcileToMainStatusEnded", func() {

		It("valid lease and current owner", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "stat-active-valid-lease-me-owner",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			status := getNodeStatus(req.NamespacedName)
			Expect(status).To(Equal(string(NodeStateEnded)))

			hasSpec := checkSpecExists(req.NamespacedName)
			Expect(hasSpec).To(Equal(false))

			leaseDuration := getLeaseDuration(req.NamespacedName.Name)
			Expect(leaseDuration).To(Equal(int32(0)))

			Expect(checkTaintOk(req.NamespacedName, false)).To(Equal(true))
		})

		It("leaseDuration>0 and current owner", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "stat-active-expired-lease-me-owner",
				},
			}
			res, err := r.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			status := getNodeStatus(req.NamespacedName)
			Expect(status).To(Equal(string(NodeStateActive)))

			hasSpec := checkSpecExists(req.NamespacedName)
			Expect(hasSpec).To(Equal(false))

			leaseDuration := getLeaseDuration(req.NamespacedName.Name)
			Expect(leaseDuration).To(Equal(int32(0)))

			Expect(checkTaintOk(req.NamespacedName, false)).To(Equal(true))

		})
	})

})
