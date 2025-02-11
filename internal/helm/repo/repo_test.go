//go:build unit
// +build unit

package repo

import (
	"testing"

	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/stretchr/testify/assert"
)

func TestURLJoin(t *testing.T) {
	tests := []struct {
		baseURL string
		paths   []string
		want    string
		wantErr bool
	}{
		{
			baseURL: "https://example.com",
			paths:   []string{"path", "to", "resource"},
			want:    "https://example.com/path/to/resource",
			wantErr: false,
		},
		{
			baseURL: "https://example.com/base",
			paths:   []string{"path", "to", "resource"},
			want:    "https://example.com/base/path/to/resource",
			wantErr: false,
		},
		{
			baseURL: "invalid-url" + string(byte(0x7f)),
			paths:   []string{"path", "to", "resource"},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			got, err := URLJoin(tt.baseURL, tt.paths...)
			if (err != nil) != tt.wantErr {
				t.Errorf("URLJoin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("URLJoin() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	log := logging.NewNopLogger()
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "Valid index file",
			data:    []byte(`apiVersion: v1`),
			wantErr: false,
		},
		{
			name:    "Empty index file",
			data:    []byte(``),
			wantErr: true,
		},
		{
			name:    "Invalid index file",
			data:    []byte(`invalid`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.data, "source", log)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIndexFile(t *testing.T) {
	index := NewIndexFile()
	md := &Metadata{
		Name:    "test-chart",
		Version: "1.0.0",
	}

	err := index.MustAdd(md, "test-chart-1.0.0.tgz", "https://example.com", "digest")
	assert.NoError(t, err)

	chart, err := index.Get("test-chart", "1.0.0")
	assert.NoError(t, err)
	assert.NotNil(t, chart)

	chart, err = index.Get("test-chart", "")
	assert.NoError(t, err)
	assert.NotNil(t, chart)

	chart, err = index.Get("non-existent-chart", "1.0.0")
	assert.Error(t, err)
	assert.Nil(t, chart)
}

func TestIndexFile_Merge(t *testing.T) {
	index1 := NewIndexFile()
	md1 := &Metadata{
		Name:    "test-chart",
		Version: "1.0.0",
	}
	err := index1.MustAdd(md1, "test-chart-1.0.0.tgz", "https://example.com", "digest")
	assert.NoError(t, err)

	index2 := NewIndexFile()
	md2 := &Metadata{
		Name:    "test-chart",
		Version: "2.0.0",
	}
	err = index2.MustAdd(md2, "test-chart-2.0.0.tgz", "https://example.com", "digest")
	assert.NoError(t, err)

	index1.Merge(index2)

	chart, err := index1.Get("test-chart", "1.0.0")
	assert.NoError(t, err)
	assert.NotNil(t, chart)

	chart, err = index1.Get("test-chart", "2.0.0")
	assert.NoError(t, err)
	assert.NotNil(t, chart)
}

func TestIndexFile_SortEntries(t *testing.T) {
	index := NewIndexFile()
	md1 := &Metadata{
		Name:    "test-chart",
		Version: "1.0.0",
	}
	md2 := &Metadata{
		Name:    "test-chart",
		Version: "2.0.0",
	}
	err := index.MustAdd(md1, "test-chart-1.0.0.tgz", "https://example.com", "digest")
	assert.NoError(t, err)
	err = index.MustAdd(md2, "test-chart-2.0.0.tgz", "https://example.com", "digest")
	assert.NoError(t, err)

	index.SortEntries()

	chart, err := index.Get("test-chart", "")
	assert.NoError(t, err)
	assert.Equal(t, "2.0.0", chart.Version)
}
