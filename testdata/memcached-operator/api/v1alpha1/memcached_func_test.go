package v1alpha1

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/pkg/apis"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetStatus(t *testing.T) {
	status := MemcachedStatus{
		BasicMultiPhaseObjectStatus: apis.BasicMultiPhaseObjectStatus{
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
