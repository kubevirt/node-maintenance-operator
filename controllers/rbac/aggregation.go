package rbac

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	roleName        = "node-maintenance-operator-role"
	roleBindingName = "node-maintenance-operator-role-binding"
	// aggregationLabelKey = "rbac.ext-remediation/aggregate-to-ext-remediation"
	saName            = "node-maintenance-operator-sa"
	default_namespace = "default"
	// deploymentName      = "node-maintenance-operator-controller-manager"
)

// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=*,resourceNames=node-maintenance-operator-role
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=*,resourceNames=node-maintenance-operator-role-binding

// Aggregation defines the functions needed for setting up RBAC aggregation
type Aggregation interface {
	CreateOrUpdateAggregation() error
}

type aggregation struct {
	client.Client
	reader    client.Reader
	namespace string
}

var _ Aggregation = aggregation{}

func GetLeaderElectionNamespace() string {
	return default_namespace
}

// NewAggregation create a new Aggregation struct
func NewAggregation(mgr ctrl.Manager, namespace string) Aggregation {
	return &aggregation{
		Client:    mgr.GetClient(),
		reader:    mgr.GetAPIReader(),
		namespace: namespace,
	}
}

func (a aggregation) CreateOrUpdateAggregation() error {
	fmt.Println("Got to CreateOrUpdateAggregation")
	if err := a.createOrUpdateRole(); err != nil {
		return err
	}
	return a.createOrUpdateRoleBinding()

}

func (a aggregation) createOrUpdateRole() error {
	// check if the role exists
	fmt.Println("Got to createOrUpdateRole")
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: default_namespace,
			// OwnerReferences: getOwnerRefs(reader, namespace),
		},
		// AggregationRule: &rbacv1.AggregationRule{
		// 	ClusterRoleSelectors: []metav1.LabelSelector{
		// 		{
		// 			MatchLabels: map[string]string{
		// 				aggregationLabelKey: "true",
		// 			},
		// 		},
		// 	},
		// },
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"*"}, // "get",
				// "list",
				// "watch",
				// "create",
				// "update",
				// "patch",
				// "delete",
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"*"}, // "get",
				// "list",
				// "watch",
				// "create",
				// "update",
				// "patch",
				// "delete",

			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs: []string{
					"create",
					"patch",
				},
			},
		},
	}
	err := a.reader.Get(context.Background(), client.ObjectKeyFromObject(role), role)
	if errors.IsNotFound(err) {
		return a.createRole()
	} else if err != nil {
		return fmt.Errorf("failed to get cluster role: %v", err)
	}
	return a.updateRole(role)
}
func (a aggregation) createRole() error {
	err := a.Create(context.Background(), getRole())
	return err
}

func (a aggregation) updateRole(oldRole *rbacv1.Role) error {
	newRole := getRole()
	oldRole.Rules = newRole.Rules
	// oldRole.AggregationRule = newRole.AggregationRule
	return a.Update(context.Background(), oldRole)
}

// func getRole(reader client.Reader, namespace string) *rbacv1.ClusterRole {
func getRole() *rbacv1.Role {
	fmt.Println("Got to getRole")
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: default_namespace,
			// OwnerReferences: getOwnerRefs(reader, namespace),
		},
		// AggregationRule: &rbacv1.AggregationRule{
		// 	ClusterRoleSelectors: []metav1.LabelSelector{
		// 		{
		// 			MatchLabels: map[string]string{
		// 				aggregationLabelKey: "true",
		// 			},
		// 		},
		// 	},
		// },
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"*"}, // "get",
				// "list",
				// "watch",
				// "create",
				// "update",
				// "patch",
				// "delete",
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"*"}, // "get",
				// "list",
				// "watch",
				// "create",
				// "update",
				// "patch",
				// "delete",

			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs: []string{
					"create",
					"patch",
				},
			},
		},
	}
}

func (a aggregation) createOrUpdateRoleBinding() error {
	// check if the role exists
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: default_namespace,
		},
	}
	err := a.reader.Get(context.Background(), client.ObjectKeyFromObject(binding), binding)
	if errors.IsNotFound(err) {
		return a.createRoleBinding()
	} else if err != nil {
		return fmt.Errorf("failed to get  role binding: %v", err)
	}
	return a.updateRoleBinding(binding)
}

func (a aggregation) createRoleBinding() error {
	err := a.Create(context.Background(), getRoleBinding(a.namespace))
	return err
}

func (a aggregation) updateRoleBinding(oldBinding *rbacv1.RoleBinding) error {
	newBinding := getRoleBinding(a.namespace)
	oldBinding.RoleRef = newBinding.RoleRef
	oldBinding.Subjects = newBinding.Subjects
	return a.Update(context.Background(), oldBinding)
}

// func getRoleBinding(reader client.Reader, namespace string) *rbacv1.RoleBinding {
func getRoleBinding(namespace string) *rbacv1.RoleBinding {
	fmt.Println("Got to getRoleBinding")
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: default_namespace,
			// OwnerReferences: getOwnerRefs(reader, namespace),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
	}
}

// func getOwnerRefs(reader client.Reader, namespace string) []metav1.OwnerReference {

// 	depl := &appsv1.Deployment{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      deploymentName,
// 			Namespace: namespace,
// 		},
// 	}
// 	if err := reader.Get(context.Background(), client.ObjectKeyFromObject(depl), depl); err != nil {
// 		// ignore for now, skip owner refs
// 		return nil
// 	}

// 	return []metav1.OwnerReference{
// 		{
// 			// at least in tests, TypeMeta is empty for the test deployment...
// 			APIVersion: fmt.Sprintf("%s/%s", appsv1.SchemeGroupVersion.Group, appsv1.SchemeGroupVersion.Version),
// 			Kind:       "Deployment",
// 			Name:       depl.Name,
// 			UID:        depl.UID,
// 		},
// 	}
// }
