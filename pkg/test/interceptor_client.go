package test

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type InterceptorClient struct {
	client.Client

	GetInterceptor          func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	ListInterceptor         func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	CreateInterceptor       func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	DeleteInterceptor       func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
	UpdateInterceptor       func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	PatchInterceptor        func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
	DeleteAllOfInterceptor  func(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error
	StatusUpdateInterceptor func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error
	StatusPatchInterceptor  func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error
	StatusCreateInterceptor func(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error
	StatusApplyInterceptor  func(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error
}

func NewInterceptorClient(c client.Client) *InterceptorClient {
	return &InterceptorClient{Client: c}
}

func (c *InterceptorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.GetInterceptor != nil {
		return c.GetInterceptor(ctx, key, obj, opts...)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *InterceptorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.ListInterceptor != nil {
		return c.ListInterceptor(ctx, list, opts...)
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *InterceptorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.CreateInterceptor != nil {
		return c.CreateInterceptor(ctx, obj, opts...)
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *InterceptorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.DeleteInterceptor != nil {
		return c.DeleteInterceptor(ctx, obj, opts...)
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *InterceptorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.UpdateInterceptor != nil {
		return c.UpdateInterceptor(ctx, obj, opts...)
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *InterceptorClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.PatchInterceptor != nil {
		return c.PatchInterceptor(ctx, obj, patch, opts...)
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *InterceptorClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if c.DeleteAllOfInterceptor != nil {
		return c.DeleteAllOfInterceptor(ctx, obj, opts...)
	}
	return c.Client.DeleteAllOf(ctx, obj, opts...)
}

func (c *InterceptorClient) Status() client.SubResourceWriter {
	return &InterceptorStatusClient{
		client:            c.Client,
		updateInterceptor: c.StatusUpdateInterceptor,
		patchInterceptor:  c.StatusPatchInterceptor,
		createInterceptor: c.StatusCreateInterceptor,
		applyInterceptor:  c.StatusApplyInterceptor,
	}
}

func (c *InterceptorClient) Scheme() *runtime.Scheme {
	return c.Client.Scheme()
}

func (c *InterceptorClient) RESTMapper() meta.RESTMapper {
	return c.Client.RESTMapper()
}

func (c *InterceptorClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.Client.GroupVersionKindFor(obj)
}

func (c *InterceptorClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.Client.IsObjectNamespaced(obj)
}

func (c *InterceptorClient) SubResource(subResource string) client.SubResourceClient {
	return c.Client.SubResource(subResource)
}

type InterceptorStatusClient struct {
	client            client.Client
	updateInterceptor func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error
	patchInterceptor  func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error
	createInterceptor func(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error
	applyInterceptor  func(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error
}

var _ client.SubResourceWriter = &InterceptorStatusClient{}

func (s *InterceptorStatusClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
	if s.applyInterceptor != nil {
		return s.applyInterceptor(ctx, obj, opts...)
	}
	return s.client.Status().Apply(ctx, obj, opts...)
}

func (s *InterceptorStatusClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if s.createInterceptor != nil {
		return s.createInterceptor(ctx, obj, subResource, opts...)
	}
	return s.client.Status().Create(ctx, obj, subResource, opts...)
}

func (s *InterceptorStatusClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if s.updateInterceptor != nil {
		return s.updateInterceptor(ctx, obj, opts...)
	}
	return s.client.Status().Update(ctx, obj, opts...)
}

func (s *InterceptorStatusClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if s.patchInterceptor != nil {
		return s.patchInterceptor(ctx, obj, patch, opts...)
	}
	return s.client.Status().Patch(ctx, obj, patch, opts...)
}
