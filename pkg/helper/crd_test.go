package helper

import (
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakeDiscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHasCRD(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	gv := schema.GroupVersion{Group: "test.group", Version: "v1"}
	fakeClient.Discovery().(*fakeDiscovery.FakeDiscovery).Resources = []*v1.APIResourceList{
		{
			GroupVersion: gv.String(),
			APIResources: []v1.APIResource{
				{
					Kind:    "test",
					Group:   gv.Group,
					Version: gv.Version,
				},
			},
		},
	}

	tests := []struct {
		name          string
		groupVersion  schema.GroupVersion
		serverSupport bool
		want          bool
	}{
		{
			name:          "supported group version",
			groupVersion:  gv,
			serverSupport: true,
			want:          true,
		},
		{
			name:          "unsupported group version",
			groupVersion:  schema.GroupVersion{Group: "bad", Version: "v1"},
			serverSupport: false,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasCRD(fakeClient, tt.groupVersion); got != tt.want {
				t.Errorf("HasCRD() = %v, want %v", got, tt.want)
			}
		})
	}
}
