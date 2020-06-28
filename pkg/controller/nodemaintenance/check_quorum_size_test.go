package nodemaintenance

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfakeclient "k8s.io/client-go/kubernetes/fake"
)

func areErrorsEqual(err1, err2 error) bool {
	return (err1 != nil && err2 != nil && err1.Error() == err2.Error()) ||
		(err1 == nil && err2 == nil)
}

var _ = Describe("checkQuorumValidity", func() {
	type TestCaseDefinition struct {
		info            string
		currentHealthy  int32
		requiredHealthy int32
		disruptedPods   map[string]metav1.Time
		isValidQuorum   bool
	}
	It("check fallback option of quorum check", func() {
		testCases := []TestCaseDefinition{
			{
				info:            "quorum broken",
				currentHealthy:  2,
				requiredHealthy: 3,
				isValidQuorum:   false,
			},
			{

				info:            "quorum valid",
				currentHealthy:  3,
				requiredHealthy: 2,
				isValidQuorum:   true,
			},
			{
				info:            "quorum not broken with dirsupted pods",
				currentHealthy:  4,
				requiredHealthy: 2,
				disruptedPods: map[string]metav1.Time{
					"disruptedPod": metav1.Time{},
				},
				isValidQuorum: true,
			},
			{
				info:            "quorum broken with dirsupted pods",
				currentHealthy:  4,
				requiredHealthy: 4,
				disruptedPods: map[string]metav1.Time{
					"disruptedPod": metav1.Time{},
				},
				isValidQuorum: false,
			},
		}
		for _, c := range testCases {
			By("test:" + c.info)
			Context(c.info, func() {

				masterNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "masternode",
						Labels: map[string]string{
							MasterNodeLabel: "",
						},
					},
					Spec: corev1.NodeSpec{},
				}
				objs := []runtime.Object{
					&policy.PodDisruptionBudgetList{
						Items: []policy.PodDisruptionBudget{
							policy.PodDisruptionBudget{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: OpenshiftMachineConfigNamespace,
								},
								Spec: policy.PodDisruptionBudgetSpec{
									MinAvailable: &intstr.IntOrString{
										Type:   intstr.Int,
										IntVal: int32(c.requiredHealthy),
									},
								},
								Status: policy.PodDisruptionBudgetStatus{
									CurrentHealthy: c.currentHealthy,
									DisruptedPods:  c.disruptedPods,
								},
							},
						},
					},
					masterNode,
				}
				cs := k8sfakeclient.NewSimpleClientset(objs...)
				isValidQuorum, err := checkValidQuorum(cs, masterNode)
				Expect(isValidQuorum).To(Equal(c.isValidQuorum))
				Expect(err).NotTo(HaveOccurred())
			})
		}
	})
})

