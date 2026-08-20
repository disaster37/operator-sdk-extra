package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestEqualFromYamlFile(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
		Data: map[string]string{
			"fu": "bar",
		},
	}

	EqualFromYamlFile[*corev1.ConfigMap](t, "testdata/configmap.yaml", cm, scheme.Scheme)
}

func TestEqualFromYamlFileEmptyPath(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r)
		assert.Contains(t, r.(string), "expectedYamlFile must be provided")
	}()
	EqualFromYamlFile[*corev1.ConfigMap](t, "", &corev1.ConfigMap{}, scheme.Scheme)
}

func TestEqualFromYamlFileNonExistentFile(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r)
	}()
	EqualFromYamlFile[*corev1.ConfigMap](t, "/nonexistent/path/to/file.yaml", &corev1.ConfigMap{}, scheme.Scheme)
}

func TestEqualFromYamlFileInvalidYaml(t *testing.T) {
	tmpDir := t.TempDir()
	badYamlFile := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(badYamlFile, []byte("{{invalid yaml content}}"), 0o644)
	assert.NoError(t, err)

	defer func() {
		r := recover()
		assert.NotNil(t, r)
	}()
	EqualFromYamlFile[*corev1.ConfigMap](t, badYamlFile, &corev1.ConfigMap{}, scheme.Scheme)
}

// Note: TestEqualFromYamlFileMismatch is not implemented as the function uses assert.Fail
// which signals test failure but doesn't panic, making the test difficult to validate programmatically
