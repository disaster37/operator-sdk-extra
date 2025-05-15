package helper

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kr/pretty"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Diff is cmp.Diff with custom function to compare slices with pretty.Sprint
func Diff(expected, current any) string {
	return cmp.Diff(expected, current, cmpopts.SortSlices(func(x, y any) bool {
		return pretty.Sprint(x) < pretty.Sprint(y)
	}))
}

// DiffMapString permit to diff map[string]string
func DiffMapString(expected, current map[string]string, excludeKeys []string) string {
	tmpExpected := map[string]string{}
	tmpCurrent := map[string]string{}

Loop:
	for key, val := range expected {
		for _, excludeKey := range excludeKeys {
			if key == excludeKey {
				continue Loop
			}
		}
		tmpExpected[key] = val
	}

Loop2:
	for key, val := range current {
		for _, excludeKey := range excludeKeys {
			if key == excludeKey {
				continue Loop2
			}
		}
		tmpCurrent[key] = val
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
		if group.Group == owner.GetObjectKind().GroupVersionKind().Group && existing.Kind == owner.GetObjectKind().GroupVersionKind().Kind {
			if group.Version != owner.GetObjectKind().GroupVersionKind().Version {
				return fmt.Sprintf("Owner references differ: %s vs %s/%s", existing.APIVersion, owner.GetObjectKind().GroupVersionKind().Group, owner.GetObjectKind().GroupVersionKind().Version), nil
			}
		} else {
			return fmt.Sprintf("Owner references differ: %s.%s vs %s.%s/%s", existing.Kind, existing.APIVersion, owner.GetObjectKind().GroupVersionKind().Kind, owner.GetObjectKind().GroupVersionKind().Group, owner.GetObjectKind().GroupVersionKind().Version), nil
		}

	} else {
		return "Need to set owner references", nil
	}

	return "", nil
}
