package resource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resource interface {
	client.Object
	GetObjectMeta() metav1.ObjectMeta
}
