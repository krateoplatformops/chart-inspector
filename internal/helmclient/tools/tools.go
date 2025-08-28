package tools

import (
	"fmt"

	sigsyaml "sigs.k8s.io/yaml"
)

func AddOrUpdateFieldInValues(values []byte, value interface{}, fields ...string) ([]byte, error) {
	var valuesMap map[string]interface{}
	if err := sigsyaml.Unmarshal(values, &valuesMap); err != nil {
		return nil, err
	}

	// Recursive function to add the value to the map creating nested maps if needed
	var addOrUpdateField func(map[string]interface{}, []string, interface{}) error
	addOrUpdateField = func(m map[string]interface{}, fields []string, value interface{}) error {
		if len(fields) == 1 {
			m[fields[0]] = value
			return nil
		}

		if _, ok := m[fields[0]]; !ok {
			m[fields[0]] = map[string]interface{}{}
		}

		if nestedMap, ok := m[fields[0]].(map[string]interface{}); ok {
			return addOrUpdateField(nestedMap, fields[1:], value)
		} else {
			return fmt.Errorf("field %s is not a map", fields[0])
		}
	}

	if err := addOrUpdateField(valuesMap, fields, value); err != nil {
		return nil, err
	}

	return sigsyaml.Marshal(valuesMap)
}

type CompositionValues struct {
	KrateoNamespace             string
	CompositionName             string
	CompositionNamespace        string
	CompositionId               string
	compositionApiVersion       string // DEPRECATED: Remove in future versions in favor of compositionGroup and compositionInstalledVersion
	CompositionGroup            string
	compositionInstalledVersion string
	CompositionResource         string
	CompositionKind             string
	GracefullyPaused            bool
}

// InjectValues injects composition related values into the provided Helm chart values. It adds fields under the "global" key.
// The modified values are returned as a byte slice.
func InjectValues(dat []byte, opts CompositionValues) ([]byte, error) {
	var err error
	if opts.GracefullyPaused {
		dat, err = AddOrUpdateFieldInValues(dat, true, "global", "gracefullyPaused")
		if err != nil {
			return dat, fmt.Errorf("failed to add gracefullyPaused to values: %w", err)
		}
	}

	dat, err = AddOrUpdateFieldInValues(dat, opts.CompositionNamespace, "global", "compositionNamespace")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionNamespace to values: %w", err)
	}
	dat, err = AddOrUpdateFieldInValues(dat, opts.CompositionName, "global", "compositionName")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionName to values: %w", err)
	}
	dat, err = AddOrUpdateFieldInValues(dat, opts.KrateoNamespace, "global", "krateoNamespace")
	if err != nil {
		return dat, fmt.Errorf("failed to add krateoNamespace to values: %w", err)
	}
	dat, err = AddOrUpdateFieldInValues(dat, opts.CompositionId, "global", "compositionId")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionId to values: %w", err)
	}
	// DEPRECATED: Remove in future versions in favor of compositionGroup and compositionInstalledVersion
	dat, err = AddOrUpdateFieldInValues(dat, opts.compositionApiVersion, "global", "compositionApiVersion")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionApiVersion to values: %w", err)
	}
	// END DEPRECATED
	dat, err = AddOrUpdateFieldInValues(dat, opts.CompositionGroup, "global", "compositionGroup")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionGroup to values: %w", err)
	}
	dat, err = AddOrUpdateFieldInValues(dat, opts.compositionInstalledVersion, "global", "compositionInstalledVersion")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionInstalledVersion to values: %w", err)
	}
	dat, err = AddOrUpdateFieldInValues(dat, opts.CompositionResource, "global", "compositionResource")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionResource to values: %w", err)
	}
	dat, err = AddOrUpdateFieldInValues(dat, opts.CompositionKind, "global", "compositionKind")
	if err != nil {
		return dat, fmt.Errorf("failed to add compositionKind to values: %w", err)
	}
	return dat, nil

}
