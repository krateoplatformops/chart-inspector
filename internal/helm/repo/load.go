package repo

import (
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"sigs.k8s.io/yaml"
)

// loadIndex loads an index file and does minimal validity checking.
//
// The source parameter is only used for logging.
// This will fail if API Version is not set (ErrNoAPIVersion) or if the unmarshal fails.
func Load(data []byte, source string, log logging.Logger) (*IndexFile, error) {
	i := &IndexFile{}

	if len(data) == 0 {
		return i, ErrEmptyIndexYaml
	}

	if err := yaml.UnmarshalStrict(data, i); err != nil {
		return i, err
	}

	for name, cvs := range i.Entries {
		for idx := len(cvs) - 1; idx >= 0; idx-- {
			if cvs[idx] == nil {
				log.Debug("skipping invalid entry", "chart", name, "source", source, "reason", "empty entry")
				continue
			}
			if cvs[idx].APIVersion == "" {
				cvs[idx].APIVersion = APIVersionV1
			}
		}
	}
	i.SortEntries()
	if i.APIVersion == "" {
		return i, ErrNoAPIVersion
	}
	return i, nil
}
