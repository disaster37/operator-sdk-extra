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
	// relative (non-escaping) path that does not exist
	defer func() {
		r := recover()
		assert.NotNil(t, r)
	}()
	EqualFromYamlFile[*corev1.ConfigMap](t, "testdata/does-not-exist.yaml", &corev1.ConfigMap{}, scheme.Scheme)
}

func TestEqualFromYamlFileInvalidYaml(t *testing.T) {
	// NOTE: t.TempDir() returns an absolute path which os.OpenRoot(".") rejects.
	// Create the invalid fixture relative to CWD instead:
	dir, err := os.MkdirTemp(".", "equal-invalid-")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }() // ignore error on purpose

	badYamlFile := filepath.Join(dir, "invalid.yaml")
	assert.NoError(t, os.WriteFile(badYamlFile, []byte("{{invalid yaml content}}"), 0o644))

	defer func() {
		r := recover()
		assert.NotNil(t, r)
	}()
	EqualFromYamlFile[*corev1.ConfigMap](t, badYamlFile, &corev1.ConfigMap{}, scheme.Scheme)
}

func TestEqualFromYamlFileTraversalBlocked(t *testing.T) {
	// Symlink pointing outside the working directory: os.Root must refuse to
	// follow it (symlink-escape vector, not just `..` traversal).
	dir, err := os.MkdirTemp(".", "equal-symlink-")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }() // ignore error on purpose
	assert.NoError(t, os.Symlink("/etc/passwd", filepath.Join(dir, "escape.yaml")))

	cases := []string{
		"../../etc/passwd",      // parent traversal
		"testdata/../../go.mod", // traversal after a valid prefix
		"testdata/configmap.yaml/../../../../etc/passwd",
		"/etc/passwd",                     // absolute path
		filepath.Join(dir, "escape.yaml"), // symlink escape
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			defer func() {
				r := recover()
				assert.NotNil(t, r, "expected panic for traversal path %q", path)
			}()
			EqualFromYamlFile[*corev1.ConfigMap](t, path, &corev1.ConfigMap{}, scheme.Scheme)
		})
	}
}

// Note: TestEqualFromYamlFileMismatch is not implemented as the function uses assert.Fail
// which signals test failure but doesn't panic, making the test difficult to validate programmatically
