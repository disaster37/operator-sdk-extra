package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

func CleanCrd(c *cli.Context) error {

	fileMatches, err := filepath.Glob(c.String("crd-file"))
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range fileMatches {

		logrus.Infof("Start to process file %s", file)

		// Read current CRD file
		f, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		currentCrd := new(apiv1.CustomResourceDefinition)
		if err = yaml.Unmarshal(f, currentCrd); err != nil {
			panic(err)
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
			panic(err)
		}
		if err = os.WriteFile(file, b, 0644); err != nil {
			panic(err)
		}

		logrus.Infof("Successfully processed file %s", file)

	}

	return nil

}

func recursiveCleanCrd(item apiv1.JSONSchemaProps) apiv1.JSONSchemaProps {
	if strings.Contains(item.Description, "@clean") {
		item.Description = strings.Replace(item.Description, "@clean", "", 0)
		item.Properties = nil
		item.XPreserveUnknownFields = ptr.To[bool](true)

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
