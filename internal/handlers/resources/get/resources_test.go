//go:build integration
// +build integration

package resources

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gobuffalo/flect"
	"github.com/krateoplatformops/chart-inspector/internal/handlers"
	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"

	"context"
	"os"

	"github.com/krateoplatformops/plumbing/e2e"

	xenv "github.com/krateoplatformops/plumbing/env"

	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/support/kind"
)

var (
	testenv     env.Environment
	clusterName string
	namespace   string
)

const (
	testdataPath = "../../../../testdata"
)

func TestMain(m *testing.M) {
	xenv.SetTestMode(true)

	namespace = "demo-system"
	altNamespace := "krateo-system"
	clusterName = "krateo"
	testenv = env.New()

	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), clusterName),
		e2e.CreateNamespace(namespace),
		e2e.CreateNamespace(altNamespace),

		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				return ctx, err
			}

			r.WithNamespace(namespace)

			return ctx, nil
		},
	).Finish(
		envfuncs.DeleteNamespace(namespace),
		envfuncs.DestroyCluster(clusterName),
	)

	os.Exit(testenv.Run(m))
}

func TestResourcesHandler(t *testing.T) {
	os.Setenv("DEBUG", "1")
	tests := []struct {
		name                  string
		compositionDefinition string
		composition           string
		expectedStatus        int
		expectedBody          string
	}{
		{
			name:                  "fireworks app - should fail",
			compositionDefinition: "fireworksapp.yaml",
			composition:           "fireworksapp.yaml",
			expectedStatus:        http.StatusInternalServerError,
		},
		{
			name:                  "focus - should succeed",
			compositionDefinition: "focus.yaml",
			composition:           "focus.yaml",
			expectedStatus:        http.StatusOK,
			expectedBody:          `[{"group":"finops.krateo.io","version":"v1alpha1","resource":"datapresentationazures","name":"focus-1-focus-data-presentation-azure","namespace":"krateo-system"},{"group":"finops.krateo.io","version":"v1alpha1","resource":"datapresentationazures","name":"focus-1-focus-data-presentation-azure","namespace":"krateo-system"}]`,
		},
	}

	f := features.New("Setup").
		Setup(e2e.Logger("test")).
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatal("Failed to create resources client:", err)
			}
			r.WithNamespace(namespace)

			err = decoder.ApplyWithManifestDir(ctx, r, filepath.Join(testdataPath, "crds", "next"), "*.yaml", nil)
			if err != nil {
				t.Fatal("Failed to apply CRDs:", err)
			}

			err = decoder.ApplyWithManifestDir(ctx, r, filepath.Join(testdataPath, "crds"), "*.yaml", nil)
			if err != nil {
				t.Fatal("Failed to apply test data:", err)
			}

			time.Sleep(5 * time.Second) // Wait for CRDs to be ready

			return ctx
		}).
		Assess("Testing Resources Endpoint", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			r, err := resources.New(c.Client().RESTConfig())
			if err != nil {
				t.Fatal("Failed to create resources client:", err)
			}
			r.WithNamespace(namespace)

			dynamic := dynamic.NewForConfigOrDie(c.Client().RESTConfig())

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					composition := &unstructured.Unstructured{}
					err := decoder.DecodeFile(os.DirFS(filepath.Join(testdataPath, "compositions")), tt.composition, composition)
					if err != nil {
						t.Fatal("Error decoding Composition:", err)
					}

					err = decoder.ApplyWithManifestDir(ctx, r, filepath.Join(testdataPath, "compositions"), tt.composition, nil)
					if err != nil {
						t.Fatal("Error applying Composition:", err)
					}

					cd := &unstructured.Unstructured{}
					err = decoder.DecodeFile(os.DirFS(filepath.Join(testdataPath, "compositiondefinitions")), tt.compositionDefinition, cd)
					if err != nil {
						t.Fatal("Error decoding CompositionDefinition:", err)
					}

					err = decoder.ApplyWithManifestDir(ctx, r, filepath.Join(testdataPath, "compositiondefinitions"), tt.compositionDefinition, nil)
					if err != nil {
						t.Fatal("Error applying CompositionDefinition:", err)
					}

					time.Sleep(5 * time.Second) // Wait for resources to be created

					// Create new URL path following RESTful pattern
					url := "/resources/"
					req := httptest.NewRequest(http.MethodGet, url, nil)

					// Add required query parameters for composition definition and resource details

					values := req.URL.Query()
					values.Add("compositionDefinitionName", cd.GetName())
					values.Add("compositionDefinitionNamespace", cd.GetNamespace())
					values.Add("compositionVersion", composition.GroupVersionKind().Version)                                 // Add required version
					values.Add("compositionResource", flect.Pluralize(strings.ToLower(composition.GroupVersionKind().Kind))) // Add required resource
					values.Add("compositionName", composition.GetName())
					values.Add("compositionNamespace", composition.GetNamespace())
					req.URL.RawQuery = values.Encode()

					rec := httptest.NewRecorder()
					h := GetResources(handlers.HandlerOptions{
						Log:           slog.Default(),
						Client:        http.DefaultClient,
						DynamicClient: dynamic,
						HelmClientOptions: ptr.To(helmclient.RestConfClientOptions{
							RestConfig: c.Client().RESTConfig(),
						}),
					})

					h.ServeHTTP(rec, req)

					res := rec.Result()
					defer res.Body.Close()

					if res.StatusCode != tt.expectedStatus {
						t.Errorf("Expected status code %d, got %d", tt.expectedStatus, res.StatusCode)
						t.Logf("Response body: %s", rec.Body.String())
					}

					if len(tt.expectedBody) > 0 {
						respBody := strings.TrimSpace(rec.Body.String())
						assert.Equal(t, tt.expectedBody, respBody, "unexpected response body")
					}
				})
			}

			return ctx
		}).Feature()

	testenv.Test(t, f)
}

func TestResourcesHandlerErrorCases(t *testing.T) {
	f := features.New("Error Cases").
		Setup(e2e.Logger("test")).
		Assess("Testing Resources Endpoint Error Cases", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			dynamic := dynamic.NewForConfigOrDie(c.Client().RESTConfig())

			tests := []struct {
				name           string
				url            string
				expectedStatus int
			}{
				{
					name:           "missing composition definition name",
					url:            "/resources",
					expectedStatus: http.StatusBadRequest,
				},
				{
					name:           "missing composition version",
					url:            "/resources?compositionDefinitionName=test&compositionDefinitionNamespace=demo-system&compositionResource=compositions",
					expectedStatus: http.StatusBadRequest,
				},
				{
					name:           "non-existent composition",
					url:            "/resources?compositionDefinitionName=test&compositionDefinitionNamespace=demo-system&compositionVersion=v1alpha1&compositionResource=compositions&compositionName=nonexistent&compositionNamespace=demo-system",
					expectedStatus: http.StatusInternalServerError,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, tt.url, nil)
					rec := httptest.NewRecorder()

					h := GetResources(handlers.HandlerOptions{
						Log:           slog.Default(),
						Client:        http.DefaultClient,
						DynamicClient: dynamic,
						HelmClientOptions: ptr.To(helmclient.RestConfClientOptions{
							RestConfig: c.Client().RESTConfig(),
						}),
					})

					h.ServeHTTP(rec, req)

					if rec.Code != tt.expectedStatus {
						t.Errorf("Expected status code %d, got %d", tt.expectedStatus, rec.Code)
						t.Logf("Response body: %s", rec.Body.String())
					}
				})
			}

			return ctx
		}).Feature()

	testenv.Test(t, f)
}
