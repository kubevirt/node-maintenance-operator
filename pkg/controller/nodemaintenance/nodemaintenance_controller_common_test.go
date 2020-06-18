package nodemaintenance

import (

	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sdiscovery "k8s.io/client-go/discovery"
	k8sfakeclient "k8s.io/client-go/kubernetes/fake"
	nodemaintenanceapi "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
)


func getCommonTestObjs() (*nodemaintenanceapi.NodeMaintenance, []runtime.Object) {
		nm := &nodemaintenanceapi.NodeMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-maintanance",
			},
			Spec: nodemaintenanceapi.NodeMaintenanceSpec{
				NodeName: "node01",
				Reason:   "test reason",
			},
		}

		return nm, []runtime.Object{
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node01",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{{
						Key:    "test",
						Effect: corev1.TaintEffectPreferNoSchedule},
					},
				},
			},
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node02",
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-1",
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
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-2",
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


}

type FakeDiscoveryThatReturnsError struct {
	k8sdiscovery.DiscoveryInterface
}

func (self *FakeDiscoveryThatReturnsError) ServerGroups() (*metav1.APIGroupList, error) {
	return nil, fmt.Errorf("ServerGroupsFails")
}

type FakeDiscovery struct {
	k8sdiscovery.DiscoveryInterface
	groupList *metav1.APIGroupList
}

func (self *FakeDiscovery) ServerGroups() (*metav1.APIGroupList, error) {
	return self.groupList, nil
}


const (
	FakeClientReturnError = 0
	FakeClientReturnLeasePackage = 1
	FakeClientReturnWrongPackage = 2
)

type FakeClient struct {
	k8sfakeclient.Clientset
	clientType int
}

func (self *FakeClient) Discovery() k8sdiscovery.DiscoveryInterface {
	if self.clientType == FakeClientReturnError {
		return &FakeDiscoveryThatReturnsError{}
	}
	if self.clientType == FakeClientReturnWrongPackage {
		return &FakeDiscovery{groupList: &metav1.APIGroupList{
					Groups: []metav1.APIGroup{{
						Name: "lease",
						Versions: []metav1.GroupVersionForDiscovery{{
							GroupVersion: LeaseApiPackage + "notQuite",
							Version:      "v1beta1",
						}},
					}},
				}}
	}
	if self.clientType == FakeClientReturnLeasePackage {
		return &FakeDiscovery{groupList: &metav1.APIGroupList{
					Groups: []metav1.APIGroup{{
						Name: "lease",
						Versions: []metav1.GroupVersionForDiscovery{{
							GroupVersion: LeaseApiPackage,
							Version:      "v1beta1",
						}},
					}},
				}}
	}
	return nil
}
