package helper

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

// HasCRD checks if the Kubernetes server supports the given groupVersion for CustomResourceDefinition.
//
// Parameters:
// - kclient: A pointer to a Kubernetes clientset.
// - groupVersion: The groupVersion of the CustomResourceDefinition to check.
//
// Returns:
// - bool: True if the server supports the given groupVersion, false otherwise.
func HasCRD(kclient *kubernetes.Clientset, groupVersion schema.GroupVersion) bool {

	if err := discovery.ServerSupportsVersion(kclient, groupVersion); err != nil {
		return false
	}

	return true
}
