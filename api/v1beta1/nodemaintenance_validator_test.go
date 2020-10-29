package v1beta1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("NodeMaintenance Validation", func() {

	const nonExistingNodeName = "node-not-exists"
	const existingNodeName = "node-exists"

	var (
		client  client.Client
		objects []runtime.Object
	)

	BeforeEach(func() {
		objects = make([]runtime.Object, 0)
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		// add our own scheme
		SchemeBuilder.AddToScheme(scheme)
		// add more schemes
		v1.AddToScheme(scheme)
		v1beta1.AddToScheme(scheme)

		client = fake.NewFakeClientWithScheme(scheme, objects...)
		InitValidator(client)
	})

	Describe("creating NodeMaintenance", func() {

		Context("for not existing node", func() {

			It("should be rejected", func() {
				nm := getTestNMO(nonExistingNodeName)
				err := nm.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorNodeNotExists, nonExistingNodeName))
			})

		})

		Context("for node already in maintenance", func() {

			BeforeEach(func() {
				// add a node and node maintenance CR to fake client
				node := getTestNode(existingNodeName, false)
				nmExisting := getTestNMO(existingNodeName)
				objects = append(objects, node, nmExisting)
			})

			It("should be rejected", func() {
				nm := getTestNMO(existingNodeName)
				err := nm.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorNodeMaintenanceExists, existingNodeName))
			})

		})

		Context("for master node", func() {

			BeforeEach(func() {
				node := getTestNode(existingNodeName, true)
				objects = append(objects, node)
			})

			Context("with potential quorum violation", func() {

				BeforeEach(func() {
					pdb := getTestPDB(0)
					objects = append(objects, pdb)
				})

				It("should be rejected", func() {
					nm := getTestNMO(existingNodeName)
					err := nm.ValidateCreate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(ErrorMasterQuorumViolation))
				})

			})

			Context("without potential quorum violation", func() {

				BeforeEach(func() {
					pdb := getTestPDB(1)
					objects = append(objects, pdb)
				})

				It("should not be rejected", func() {
					nm := getTestNMO(existingNodeName)
					err := nm.ValidateCreate()
					Expect(err).ToNot(HaveOccurred())
				})

			})

			Context("without etcd quorum guard PDB", func() {

				It("should not be rejected", func() {
					nm := getTestNMO(existingNodeName)
					err := nm.ValidateCreate()
					Expect(err).ToNot(HaveOccurred())
				})

			})
		})

	})

	Describe("updating NodeMaintenance", func() {

		Context("with new nodeName", func() {

			It("should be rejected", func() {
				nmOld := getTestNMO(existingNodeName)
				nm := getTestNMO("newNodeName")
				err := nm.ValidateUpdate(nmOld)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorNodeNameUpdateForbidden))
			})

		})
	})
})

func getTestNMO(nodeName string) *NodeMaintenance {
	return &NodeMaintenance{
		Spec: NodeMaintenanceSpec{
			NodeName: nodeName,
		},
	}
}

func getTestNode(name string, isMaster bool) *v1.Node {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if isMaster {
		node.ObjectMeta.Labels = map[string]string{
			LabelNameRoleMaster: "",
		}
	}
	return node
}

func getTestPDB(allowed int) *v1beta1.PodDisruptionBudget {
	return &v1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: EtcdQuorumPDBNamespace,
			Name:      EtcdQuorumPDBName,
		},
		Status: v1beta1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: int32(allowed),
		},
	}
}
