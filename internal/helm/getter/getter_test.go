//go:build unit
// +build unit

package getter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name    string
		opts    GetOptions
		wantErr bool
	}{
		// {
		// 	name: "OCI URI",
		// 	opts: GetOptions{
		// 		URI: "oci://example.com/chart",
		// 	},
		// 	wantErr: false,
		// },
		{
			name: "TGZ URI",
			opts: GetOptions{
				URI: "https://raw.githubusercontent.com/krateoplatformops/helm-charts/refs/heads/gh-pages/api-1.0.0.tgz",
			},
			wantErr: false,
		},
		{
			name: "HTTP URI",
			opts: GetOptions{
				URI:     "https://charts.krateo.io",
				Repo:    "fireworks-app",
				Version: "1.1.10",
			},
			wantErr: false,
		},
		{
			name: "Invalid URI",
			opts: GetOptions{
				URI: "invalid://example.com/chart",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Get(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTGZGetter(t *testing.T) {
	opts := GetOptions{
		URI: "https://raw.githubusercontent.com/krateoplatformops/helm-charts/refs/heads/gh-pages/api-1.0.0.tgz",
	}

	g := &tgzGetter{}
	_, _, err := g.Get(opts)
	assert.NoError(t, err)
}

// func TestOCIGetter(t *testing.T) {
// 	opts := GetOptions{
// 		URI: "oci://example.com/chart",
// 	}

// 	g, err := newOCIGetter()
// 	assert.NoError(t, err)

// 	_, _, err = g.Get(opts)
// 	assert.NoError(t, err)
// }

func TestRepoGetter(t *testing.T) {
	opts := GetOptions{
		URI:     "https://charts.krateo.io",
		Repo:    "fireworks-app",
		Version: "1.1.10",
	}

	g := &repoGetter{}
	_, _, err := g.Get(opts)
	assert.NoError(t, err)
}

func TestFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data"))
	}))
	defer server.Close()

	opts := GetOptions{
		URI: server.URL,
	}

	data, err := fetch(opts)
	assert.NoError(t, err)
	assert.Equal(t, "test data", string(data))
}
