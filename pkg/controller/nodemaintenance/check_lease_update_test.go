package nodemaintenance

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
    . "github.com/onsi/ginkgo/extensions/table"
	corev1 "k8s.io/api/core/v1"
	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	apitypes "k8s.io/apimachinery/pkg/types"
	"time"
	"context"
	"fmt"
)

const NowTime = 100000

func compareString(rval *string, lval *string) bool {
	return (rval == nil && lval == nil) || (rval != nil && lval != nil && *rval == *lval)
}

func compareTime(rval *metav1.MicroTime, lval *metav1.MicroTime) bool {
	return (rval == nil && lval == nil) || (rval != nil && lval != nil && (*rval).Time == (*lval).Time)
}

func compareInt32(rval *int32, lval *int32) bool {
	return (rval == nil && lval == nil) || (rval != nil && lval != nil && *rval == *lval)
}

func makeString(val string) (*string) {
	return &val
}

func getTimeLease(tmsec int64) (*metav1.MicroTime) {
	tm  := time.Unix(int64(tmsec),0)
	ret := metav1.MicroTime{Time: tm}
	return &ret
}

func getMockNode() (*corev1.Node) {
	node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "miau",
					UID:  "foobar",
				},
	}
	return node
}

var _ = Describe("checkLeaseUpdate", func() {

	DescribeTable("check lease supported",
		func(initLease *coordv1beta1.Lease, leasePostcondition *coordv1beta1.Lease, expectedError error) {
			node := getMockNode()
			objs := []runtime.Object{
				initLease,
			}
			cl  := fake.NewFakeClient(objs...)
			timeNow :=  time.Unix(NowTime,0)

			name := apitypes.NamespacedName{Namespace: LeaseNamespace, Name: node.Name}
			leaseIn := &coordv1beta1.Lease{}
			err := cl.Get(context.TODO(), name, leaseIn)
			Expect(err).NotTo(HaveOccurred())

			leaseRet, err, failedUpdateOwnedLease := updateLease(cl, node, leaseIn, timeNow, LeaseDurationInSeconds)

			if leasePostcondition == nil {
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedError))

				var nilRef *coordv1beta1.Lease
				Expect(leaseRet).To(Equal(nilRef))
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(expectedError).NotTo(HaveOccurred())

				Expect(failedUpdateOwnedLease).To(BeFalse())
				lease := &coordv1beta1.Lease{}
				err = cl.Get(context.TODO(), name, lease)
				Expect(err).NotTo(HaveOccurred())
				Expect(leaseRet).To(Equal(lease))

				Expect(len(lease.ObjectMeta.OwnerReferences)).To(Equal(1))
				Expect(len(leasePostcondition.ObjectMeta.OwnerReferences)).To(Equal(1))

				leaseRef := lease.ObjectMeta.OwnerReferences[0]
				leasePost := leasePostcondition.ObjectMeta.OwnerReferences[0]

				Expect(leasePost.APIVersion).To(Equal(leaseRef.APIVersion))
				Expect(leasePost.Kind).To(Equal(leaseRef.Kind))
				Expect(leasePost.Name).To(Equal(leaseRef.Name))
				Expect(leasePost.UID).To(Equal(leaseRef.UID))

				Expect(compareString(lease.Spec.HolderIdentity, leasePostcondition.Spec.HolderIdentity)).To(Equal(true))
				Expect(compareTime(lease.Spec.RenewTime, leasePostcondition.Spec.RenewTime)).To(Equal(true))
				Expect(compareTime(lease.Spec.AcquireTime, leasePostcondition.Spec.AcquireTime)).To(Equal(true))
				Expect(compareInt32(lease.Spec.LeaseDurationSeconds, leasePostcondition.Spec.LeaseDurationSeconds)).To(Equal(true))

				Expect(compareInt32(lease.Spec.LeaseTransitions, leasePostcondition.Spec.LeaseTransitions)).To(Equal(true))
			}
		},

		Entry("fail to update valid lease with different holder identity",
				&coordv1beta1.Lease{
					ObjectMeta :metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString("miau"),
						LeaseDurationSeconds: makeInt32(3200),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime-1)),
						LeaseTransitions:     nil,
					},
				},
				nil,
				fmt.Errorf("Can't update valid lease held by different owner"),
			 ),
		Entry("update lease with different holder identity (full init)",
				&coordv1beta1.Lease{
					ObjectMeta :metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString("miau"),
						LeaseDurationSeconds: makeInt32(44),
						AcquireTime:          getTimeLease(42),
						RenewTime:            getTimeLease(43),
						LeaseTransitions:     nil,
					},
				},
				&coordv1beta1.Lease{
					ObjectMeta :metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime)),
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     makeInt32(1),
					},
				},
				nil,
			 ),
		Entry("update lease with different holder identity (with transition update)",
				&coordv1beta1.Lease{
					ObjectMeta :metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString("miau"),
						LeaseDurationSeconds: makeInt32(44),
						AcquireTime:          getTimeLease(42),
						RenewTime:            getTimeLease(43),
						LeaseTransitions:     makeInt32(3),
					},
				},
				&coordv1beta1.Lease{
					ObjectMeta :metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime)),
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     makeInt32(4),
					},
				},
				nil,
			 ),
		Entry("extend lease if same holder and zero renew time (invalid lease)",
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: nil,
						AcquireTime:          getTimeLease(int64(NowTime-599)),
						RenewTime:            nil,
						LeaseTransitions:     makeInt32(3),
						},
				},
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime)),
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     makeInt32(4),
					},
				},
				nil,
			 ),
		Entry("update lease if same holder and expired lease - check modified lease duration",
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds-42),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime-LeaseDurationInSeconds-1)),
						LeaseTransitions:     nil,
						},
				},
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime)),
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     makeInt32(1),
					},
				},
				nil,
			 ),
		Entry("extend lease if same holder and expired lease (acquire time previously not nil)",
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime-LeaseDurationInSeconds-1)),
						RenewTime:            getTimeLease(int64(NowTime-LeaseDurationInSeconds-1)),
						LeaseTransitions:     makeInt32(1),
						},
				},
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime-LeaseDurationInSeconds-1)),
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     makeInt32(1),
					},
				},
				nil,
			 ),
		Entry("extend lease if same holder and expired lease (acquire time previously nil)",
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime-LeaseDurationInSeconds-1)),
						LeaseTransitions:     nil,
						},
				},
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          getTimeLease(int64(NowTime)),
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     makeInt32(1),
					},
				},
				nil,
			 ),
		Entry("extend lease if same holder and lease will expire before current Time + two times the drainertimeout",
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime) - LeaseDurationInSeconds + 2 * int64(drainerTimeout) / int64(time.Second) - 1 ),
						LeaseTransitions:     nil,
						},
				},
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
								APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
								Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
								Name:       getMockNode().Name,
								UID:        getMockNode().UID,
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime)),
						LeaseTransitions:     nil,
					},
				},
				nil,
			 ),
		Entry("dont extend lease if same holder and lease not about to expire before current Time + two times the drainertimeout",
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{
							metav1.OwnerReference{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "@",
								UID:        "#",
							},
						},
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime) - LeaseDurationInSeconds + 2 * int64(drainerTimeout) / int64(time.Second) + 1 ),
						LeaseTransitions:     nil,
						},
				},
				&coordv1beta1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:	getMockNode().Name,
						Namespace: LeaseNamespace,
						OwnerReferences: []metav1.OwnerReference{ {
							APIVersion: "v1",
							Kind:       "Node",
							Name:       "@",
							UID:        "#",
						} },
					},
					Spec: coordv1beta1.LeaseSpec{
						HolderIdentity:       makeString(LeaseHolderIdentity),
						LeaseDurationSeconds: makeInt32(LeaseDurationInSeconds),
						AcquireTime:          nil,
						RenewTime:            getTimeLease(int64(NowTime) - LeaseDurationInSeconds + 2 * int64(drainerTimeout) / int64(time.Second) + 1 ),
						LeaseTransitions:     nil,
					},
				},
				nil,
			 ),

	)
})
