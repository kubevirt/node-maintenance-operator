package v1beta1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	machinev1beta1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("NodeMaintenance Validation", func() {

	const (
		nonExistingNodeName = "node-not-exists"
		existingNodeName    = "node-exists"
		machineName         = "machine1"
	)


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
		machinev1beta1.AddToScheme(scheme)

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

		Context("for unhealthy machine", func(){
			BeforeEach(func() {
				machine := getTestMachine(machineName, true)
				node := getTestNode(existingNodeName, false)
				linkNodeToMachine(node, machine)
				objects = append(objects, node, machine)
			})

			It("should be rejected", func(){
				nm := getTestNMO(existingNodeName)
				err := nm.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorUnhealthyMachine, existingNodeName, machineName))
			})
		})

		Context("for healthy machine", func(){
			BeforeEach(func() {
				machine := getTestMachine(machineName, false)
				node := getTestNode(existingNodeName, false)
				linkNodeToMachine(node, machine)
				objects = append(objects, node, machine)
			})

			It("should not be rejected", func(){
				nm := getTestNMO(existingNodeName)
				err := nm.ValidateCreate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("for node with a broken machine ref", func(){
			var node *v1.Node

			BeforeEach(func(){
				node = getTestNode(existingNodeName, false)
				objects = append(objects, node)
			})

			Context("node without annotations", func(){
				It("should not be rejected", func(){
					nm := getTestNMO(existingNodeName)
					err := nm.ValidateCreate()
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("node with empty annotations", func() {
				BeforeEach(func() {
					node.Annotations = make(map[string]string)
				})

				It("should not be rejected", func() {
					nm := getTestNMO(existingNodeName)
					err := nm.ValidateCreate()
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("node with invalid machine ref format should be rejected", func(){
				invalidMachineRef := "blabla" // missing '/'
				BeforeEach(func() {
					node.Annotations = make(map[string]string)
					node.Annotations[MachineRefAnnotation] = invalidMachineRef
				})
				It("should be rejected", func(){
					nm := getTestNMO(existingNodeName)
					err := nm.ValidateCreate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(InvalidMachineFormat,invalidMachineRef))
				})
			})

			Context("node with invalid machine ref format", func(){
				BeforeEach(func(){
					node.Annotations = make(map[string]string)
					node.Annotations[MachineRefAnnotation] = "foo/bar"
				})

				It("node with non existent machine should not be rejected", func(){
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

func linkNodeToMachine(node *v1.Node, machine *machinev1beta1.Machine) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	node.Annotations[MachineRefAnnotation] = machine.Namespace + string(types.Separator) + machine.Name
}

func getTestMachine(name string, isUnhealthy bool) *machinev1beta1.Machine {
	machine := &machinev1beta1.Machine{}
	machine.Name = name

	if isUnhealthy {
		machine.Annotations = make(map[string]string)
		machine.Annotations[UnhealthyAnnotation] = ""
	}
	return machine

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
