package helper

import (
	"fmt"
	"maps"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kr/pretty"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Diff is cmp.Diff with custom function to compare slices with pretty.Sprint
func Diff(expected, current any) string {
	return cmp.Diff(expected, current, cmpopts.SortSlices(func(x, y any) bool {
		return pretty.Sprint(x) < pretty.Sprint(y)
	}))
}

// DiffMapString permit to diff map[string]string
func DiffMapString(expected, current map[string]string, excludeKeys []string) string {
	tmpExpected := maps.Clone(expected)
	tmpCurrent := maps.Clone(current)
	for _, key := range excludeKeys {
		delete(tmpExpected, key)
		delete(tmpCurrent, key)
	}

	return cmp.Diff(tmpCurrent, tmpExpected)
}

// DiffOwnerReferences permit to diff owner references
func DiffOwnerReferences(owner client.Object, object client.Object) (diff string, err error) {
	existing := metav1.GetControllerOf(object)

	if existing != nil {
		group, err := schema.ParseGroupVersion(existing.APIVersion)
		if err != nil {
			return "", err
		}
		gvk := owner.GetObjectKind().GroupVersionKind()
		if group.Group == gvk.Group && existing.Kind == gvk.Kind {
			if group.Version != gvk.Version {
				return fmt.Sprintf("Owner references differ: %s vs %s/%s", existing.APIVersion, gvk.Group, gvk.Version), nil
			}
		} else {
			return fmt.Sprintf("Owner references differ: %s.%s vs %s.%s/%s", existing.Kind, existing.APIVersion, gvk.Kind, gvk.Group, gvk.Version), nil
		}

	} else {
		return "Need to set owner references", nil
	}

	return "", nil
}
