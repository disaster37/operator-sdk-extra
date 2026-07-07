package test

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CRBuilder[T client.Object] struct {
	obj T
}

func NewCR[T client.Object](name, namespace string) *CRBuilder[T] {
	obj := reflectNewObject[T]()
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return &CRBuilder[T]{obj: obj}
}

func (b *CRBuilder[T]) WithLabels(labels map[string]string) *CRBuilder[T] {
	b.obj.SetLabels(labels)
	return b
}

func (b *CRBuilder[T]) WithAnnotations(annotations map[string]string) *CRBuilder[T] {
	b.obj.SetAnnotations(annotations)
	return b
}

func (b *CRBuilder[T]) WithTypeMeta(gvk schema.GroupVersionKind) *CRBuilder[T] {
	b.obj.GetObjectKind().SetGroupVersionKind(gvk)
	return b
}

func (b *CRBuilder[T]) Build() T {
	return b.obj
}
