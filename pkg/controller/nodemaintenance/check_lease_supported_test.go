package nodemaintenance

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)


func ExpectEqualWithNil(actual, expected interface{}) {
	if expected == nil {
		ExpectWithOffset(1, actual).To(BeNil())
	} else if actual == nil {
		ExpectWithOffset(1, expected).To(BeNil())
	} else {
		ExpectWithOffset(1, actual).To(Equal(expected))
	}
}

type TestCaseDefinition struct {
	info        string
	mock		*FakeClient
	expectedErr error
	expectedRes bool
}

var _ = Describe("checkIfLeaseAPISupported", func() {
	It("check lease api test cases", func() {
		testCases := []TestCaseDefinition{
			{
				"should fail with error if Discovery API returns error",
				&FakeClient{clientType: FakeClientReturnError},
				fmt.Errorf("ServerGroupsFails"),
				false,
			},
			{
				"should succeed if lease API is supported",
				&FakeClient{clientType: FakeClientReturnLeasePackage},
				nil,
				true,
			},
			{
				"should fail if lease API is not supported",
				&FakeClient{clientType: FakeClientReturnWrongPackage},
				nil,
				false,
			},
		}
		for _, c := range testCases {
			Context(c.info, func() {
					By(c.info)

					isLeaseSupported, err := checkLeaseSupportedInternal(c.mock);

					ExpectEqualWithNil(err, c.expectedErr)
					Expect(isLeaseSupported).To(Equal(c.expectedRes))

			})
		}
	})
})
