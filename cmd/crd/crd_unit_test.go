package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestCleanCrd(t *testing.T) {
	t.Run("nominal case with @clean tag", func(t *testing.T) {
		// Create a temporary directory
		tempDir := t.TempDir()

		// Create a sample CRD with @clean tag
		crdContent := `
apiVersion: apiextensions.k8s.io/v1
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
          testProperty:
            type: object
            description: "A property to clean @clean"
            properties:
              nested:
                type: string
  names:
    kind: Test
    plural: tests
`

		tempFile := filepath.Join(tempDir, "test_crd.yaml")
		err := os.WriteFile(tempFile, []byte(crdContent), 0o644)
		assert.NoError(t, err)

		// Create a CLI context with the temp file
		app := &cli.App{
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "crd-file"},
			},
			Action: func(c *cli.Context) error {
				return CleanCrd(c)
			},
		}

		err = app.Run([]string{"", "--crd-file", tempFile})
		assert.NoError(t, err)

		// Read the modified file to check if it was cleaned
		modifiedContent, err := os.ReadFile(tempFile)
		assert.NoError(t, err)

		var modifiedCrd apiv1.CustomResourceDefinition
		err = yaml.Unmarshal(modifiedContent, &modifiedCrd)
		assert.NoError(t, err)

		// Check that the @clean tag was removed and properties were cleaned
		prop := modifiedCrd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["testProperty"]
		assert.NotContains(t, prop.Description, "@clean")
		assert.Nil(t, prop.Properties) // Should be nil after cleaning
	})

	t.Run("file does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentFile := filepath.Join(tempDir, "nonexistent.yaml")

		app := &cli.App{
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "crd-file"},
			},
			Action: func(c *cli.Context) error {
				return CleanCrd(c)
			},
		}

		// This should panic when trying to read a non-existent file
		assert.Panics(t, func() {
			_ = app.Run([]string{"", "--crd-file", nonExistentFile})
		})
	})

	t.Run("invalid YAML file", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidYAMLFile := filepath.Join(tempDir, "invalid.yaml")
		err := os.WriteFile(invalidYAMLFile, []byte("invalid: ["), 0o644) // Invalid YAML
		assert.NoError(t, err)

		app := &cli.App{
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "crd-file"},
			},
			Action: func(c *cli.Context) error {
				return CleanCrd(c)
			},
		}

		// This should panic when trying to unmarshal invalid YAML
		assert.Panics(t, func() {
			_ = app.Run([]string{"", "--crd-file", invalidYAMLFile})
		})
	})
}
