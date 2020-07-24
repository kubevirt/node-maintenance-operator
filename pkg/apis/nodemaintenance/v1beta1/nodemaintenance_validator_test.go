package v1beta1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
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
		objects = make([]runtime.Object, 0)
	)

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		// add our own scheme
		SchemeBuilder.AddToScheme(scheme)
		// add more schemes
		v1.AddToScheme(scheme)

		client = fake.NewFakeClientWithScheme(scheme, objects...)
		InitValidator(client)
	})

	Context("creating NodeMaintenance", func() {

		Context("for not existing node", func() {

			It("should be rejected", func() {
				nm := &NodeMaintenance{
					Spec: NodeMaintenanceSpec{
						NodeName: nonExistingNodeName,
					},
				}
				err := nm.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorNodeNotExists, nonExistingNodeName))
			})

		})

		Context("for node already in maintenance", func() {

			BeforeEach(func() {
				// add a node and node maintenance CR to fake client
				node := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: existingNodeName,
					},
				}
				nmExisting := &NodeMaintenance{
					Spec: NodeMaintenanceSpec{
						NodeName: existingNodeName,
					},
				}
				objects = append(objects, node, nmExisting)
			})

			It("should be rejected", func() {
				nm := NodeMaintenance{
					Spec: NodeMaintenanceSpec{
						NodeName: existingNodeName,
					},
				}
				err := nm.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorNodeMaintenanceExists, existingNodeName))
			})

		})
	})

	Context("updating NodeMaintenance", func() {

		Context("with new nodeName", func() {

			It("should be rejected", func() {
				nmOld := NodeMaintenance{
					Spec: NodeMaintenanceSpec{
						NodeName: existingNodeName,
					},
				}
				nm := NodeMaintenance{
					Spec: NodeMaintenanceSpec{
						NodeName: "newNodeName",
					},
				}
				err := nm.ValidateUpdate(&nmOld)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(ErrorNodeNameUpdateForbidden))
			})

		})
	})
})
