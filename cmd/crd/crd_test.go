package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestRecursiveCleanCrd(t *testing.T) {
	t.Run("clean property with @clean tag - object type", func(t *testing.T) {
		item := apiv1.JSONSchemaProps{
			Description: "test @clean",
			Type:        "object",
			Properties: map[string]apiv1.JSONSchemaProps{
				"nested": {Type: "string"},
			},
		}

		result := recursiveCleanCrd(item)

		assert.NotContains(t, result.Description, "@clean")
		assert.Nil(t, result.Properties)
		assert.True(t, *result.XPreserveUnknownFields)
	})

	t.Run("clean property with @clean tag - array type", func(t *testing.T) {
		item := apiv1.JSONSchemaProps{
			Description: "test @clean",
			Type:        "array",
			Items: &apiv1.JSONSchemaPropsOrArray{
				Schema: &apiv1.JSONSchemaProps{
					Properties: map[string]apiv1.JSONSchemaProps{
						"nested": {Type: "string"},
					},
				},
			},
		}

		result := recursiveCleanCrd(item)

		assert.NotContains(t, result.Description, "@clean")
		assert.Nil(t, result.Items.Schema.Properties)
		assert.True(t, *result.XPreserveUnknownFields)
	})

	t.Run("recursively clean nested properties", func(t *testing.T) {
		item := apiv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiv1.JSONSchemaProps{
				"outer": {
					Type: "object",
					Properties: map[string]apiv1.JSONSchemaProps{
						"inner": {
							Description: "needs @clean",
							Type:        "object",
							Properties: map[string]apiv1.JSONSchemaProps{
								"deep": {Type: "string"},
							},
						},
					},
				},
			},
		}

		result := recursiveCleanCrd(item)

		innerProp := result.Properties["outer"].Properties["inner"]
		assert.NotContains(t, innerProp.Description, "@clean")
		assert.Nil(t, innerProp.Properties)
		assert.True(t, *innerProp.XPreserveUnknownFields)
	})

	t.Run("no @clean tag - unchanged", func(t *testing.T) {
		item := apiv1.JSONSchemaProps{
			Description: "no tag here",
			Type:        "object",
			Properties: map[string]apiv1.JSONSchemaProps{
				"nested": {Type: "string"},
			},
		}

		result := recursiveCleanCrd(item)
		assert.Contains(t, result.Description, "no tag here")
		assert.NotNil(t, result.Properties)
	})

	t.Run("no @clean tag - array recursively cleans children", func(t *testing.T) {
		item := apiv1.JSONSchemaProps{
			Type: "array",
			Items: &apiv1.JSONSchemaPropsOrArray{
				Schema: &apiv1.JSONSchemaProps{
					Properties: map[string]apiv1.JSONSchemaProps{
						"child": {
							Description: "needs @clean",
							Type:        "object",
							Properties: map[string]apiv1.JSONSchemaProps{
								"nested": {Type: "string"},
							},
						},
					},
				},
			},
		}

		result := recursiveCleanCrd(item)
		childProp := result.Items.Schema.Properties["child"]
		assert.NotContains(t, childProp.Description, "@clean")
		assert.Nil(t, childProp.Properties)
	})

	t.Run("non-object non-array with no @clean - returns unchanged", func(t *testing.T) {
		item := apiv1.JSONSchemaProps{
			Description: "simple string field",
			Type:        "string",
		}

		result := recursiveCleanCrd(item)
		assert.Equal(t, item, result)
	})
}

func TestRun(t *testing.T) {
	t.Run("run with help flag returns no error", func(t *testing.T) {
		err := run([]string{"cmd", "--help"})
		assert.NoError(t, err)
	})

	t.Run("run with clean-crd help", func(t *testing.T) {
		err := run([]string{"cmd", "clean-crd", "--help"})
		assert.NoError(t, err)
	})

	t.Run("run with debug flag", func(t *testing.T) {
		// Create a temp CRD file for the clean-crd command
		tempDir := t.TempDir()
		crdContent := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test.example.com
spec:
  group: example.com
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          testProp:
            type: string
  names:
    kind: Test
    plural: tests
`
		crdFile := filepath.Join(tempDir, "test.yaml")
		err := os.WriteFile(crdFile, []byte(crdContent), 0644)
		assert.NoError(t, err)

		err = run([]string{"cmd", "--debug", "clean-crd", "--crd-file", crdFile})
		assert.NoError(t, err)
	})
}

func TestRunWithGlobPattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple CRD files
	for i := 0; i < 3; i++ {
		crdContent := fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-%d.example.com
spec:
  group: example.com
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          prop%d:
            type: string
  names:
    kind: Test
    plural: tests
`, i, i)
		crdFile := filepath.Join(tempDir, fmt.Sprintf("test_%d.yaml", i))
		err := os.WriteFile(crdFile, []byte(crdContent), 0644)
		assert.NoError(t, err)
	}

	// Use glob pattern
	globPattern := filepath.Join(tempDir, "*.yaml")
	err := run([]string{"cmd", "clean-crd", "--crd-file", globPattern})
	assert.NoError(t, err)
}

func TestCleanCrdPanicsOnInvalidGlob(t *testing.T) {
	t.Run("no files matching glob panics", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentGlob := filepath.Join(tempDir, "nonexistent_*.yaml")

		assert.Panics(t, func() {
			_ = run([]string{"cmd", "clean-crd", "--crd-file", nonExistentGlob})
		})
	})

	t.Run("invalid yaml file panics", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidFile := filepath.Join(tempDir, "invalid.yaml")
		err := os.WriteFile(invalidFile, []byte("invalid: ["), 0644)
		assert.NoError(t, err)

		assert.Panics(t, func() {
			_ = run([]string{"cmd", "clean-crd", "--crd-file", invalidFile})
		})
	})
}

func TestCleanCrdWithYamlFormatting(t *testing.T) {
	t.Run("yaml marshal error panics", func(t *testing.T) {
		// Create a CRD file that is valid YAML but has unusual structure
		tempDir := t.TempDir()
		crdContent := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test.example.com
spec:
  group: example.com
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          testProp:
            type: object
            description: "clean @clean"
            properties:
              nested:
                type: string
  names:
    kind: Test
    plural: tests
`
		crdFile := filepath.Join(tempDir, "test.yaml")
		err := os.WriteFile(crdFile, []byte(crdContent), 0644)
		assert.NoError(t, err)

		err = run([]string{"cmd", "clean-crd", "--crd-file", crdFile})
		assert.NoError(t, err)

		// Verify the output is valid YAML
		content, err := os.ReadFile(crdFile)
		assert.NoError(t, err)

		var crd apiv1.CustomResourceDefinition
		err = yaml.Unmarshal(content, &crd)
		assert.NoError(t, err)
		assert.False(t, strings.Contains(string(content), "@clean"))
	})
}