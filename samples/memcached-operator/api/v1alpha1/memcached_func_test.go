package v1alpha1

import (
	"testing"

	multiphase "github.com/disaster37/operator-sdk-extra/v2/pkg/apis/multiphase"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetStatus(t *testing.T) {
	status := MemcachedStatus{
		DefaultMultiPhaseObjectStatus: multiphase.DefaultMultiPhaseObjectStatus{
			PhaseName: "test",
		},
	}
	o := &Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Status: status,
	}

	assert.Equal(t, &status, o.GetStatus())
}
