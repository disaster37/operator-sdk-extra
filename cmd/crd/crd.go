package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

func CleanCrd(c *cli.Context) error {
	pattern := c.String("crd-file")
	dryRun := c.Bool("dry-run")

	fileMatches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	if len(fileMatches) == 0 {
		return fmt.Errorf("no files matching: %s", pattern)
	}

	for _, file := range fileMatches {
		log.Infof("Start to process file %s", file)

		// Read current CRD file
		f, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		currentCrd := new(apiv1.CustomResourceDefinition)
		if err = yaml.Unmarshal(f, currentCrd); err != nil {
			return fmt.Errorf("unmarshal %s: %w", file, err)
		}

		// Search special tag on description to clean properties
		for i, version := range currentCrd.Spec.Versions {
			for key, item := range version.Schema.OpenAPIV3Schema.Properties {
				currentCrd.Spec.Versions[i].Schema.OpenAPIV3Schema.Properties[key] = recursiveCleanCrd(item)
			}
		}

		// Write clean CRD
		b, err := yaml.Marshal(currentCrd)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", file, err)
		}

		if dryRun {
			log.Infof("[dry-run] would write %d bytes to %s", len(b), file)
			continue
		}
		if err = os.WriteFile(file, b, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}

		log.Infof("Successfully processed file %s", file)

	}

	return nil
}

func recursiveCleanCrd(item apiv1.JSONSchemaProps) apiv1.JSONSchemaProps {
	if strings.Contains(item.Description, "@clean") {
		item.Description = strings.ReplaceAll(item.Description, "@clean", "")
		item.Properties = nil
		item.XPreserveUnknownFields = ptr.To(true)

		if item.Type == "array" {
			item.Items.Schema.Properties = nil
		} else {
			item.Properties = nil
		}

		return item
	} else {
		switch item.Type {
		case "array":
			for key, val := range item.Items.Schema.Properties {
				item.Items.Schema.Properties[key] = recursiveCleanCrd(val)
			}
		case "object":
			for key, val := range item.Properties {
				item.Properties[key] = recursiveCleanCrd(val)
			}
		default:
			return item
		}
	}

	return item
}
