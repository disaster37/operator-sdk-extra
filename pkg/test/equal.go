package test

import (
	"bytes"
	"os"
	"testing"

	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func EqualFromYamlFile[k8sobject comparable](t *testing.T, expectedYamlFile string, actual client.Object, s runtime.ObjectTyper) {

	if expectedYamlFile == "" {
		panic("expectedYamlFile must be provided")
	}

	// Read file
	f, err := os.ReadFile(expectedYamlFile)
	if err != nil {
		panic(err)
	}
	expectedObject := new(k8sobject)
	if err = yaml.Unmarshal(f, expectedObject); err != nil {
		panic(err)
	}

	y := printers.NewTypeSetter(s).ToPrinter(&printers.JSONPrinter{})
	buf := new(bytes.Buffer)
	if err := y.PrintObj(actual, buf); err != nil {
		panic(err)
	}
	currentObject := new(k8sobject)
	if err = json.Unmarshal(buf.Bytes(), currentObject); err != nil {
		panic(err)
	}

	diff := helper.Diff(*expectedObject, *currentObject)

	if diff != "" {
		assert.Fail(t, diff)
	}

}
