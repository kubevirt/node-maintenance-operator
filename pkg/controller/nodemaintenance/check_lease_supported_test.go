package nodemaintenance

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ExpectEqualWithNil(actual, expected interface{}, description string) {
	// heads up: interfaces representing a pointer are not nil when the pointer is nil
	if expected == nil || (reflect.ValueOf(expected).Kind() == reflect.Ptr && reflect.ValueOf(expected).IsNil()) {
		// BeNil() handles pointers correctly
		ExpectWithOffset(1, actual).To(BeNil(), description)
	} else {
		// compare unix time, precision of MicroTime is sometimes different
		if e, ok := expected.(*v1.MicroTime); ok {
			expected = e.Unix()
			if actual != nil && reflect.ValueOf(actual).Kind() == reflect.Ptr && !reflect.ValueOf(actual).IsNil(){
				actual = actual.(*v1.MicroTime).Unix()
			}
		}
		ExpectWithOffset(1, actual).To(Equal(expected), description)
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

					ExpectEqualWithNil(err, c.expectedErr, "error should match")
					Expect(isLeaseSupported).To(Equal(c.expectedRes))

			})
		}
	})
})
