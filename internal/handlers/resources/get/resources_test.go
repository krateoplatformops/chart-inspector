package resources

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/krateoplatformops/chart-inspector/internal/handlers"
	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"

	"context"
	"os"

	"github.com/krateoplatformops/snowplow/apis"
	"github.com/krateoplatformops/snowplow/plumbing/e2e"
	xenv "github.com/krateoplatformops/snowplow/plumbing/env"

	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
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

	// kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")

	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), clusterName),
		e2e.CreateNamespace(namespace),
		e2e.CreateNamespace(altNamespace),

		// func(ctx context.Context, c *envconf.Config) (context.Context, error) {

		// 	// update envconfig  with kubeconfig
		// 	c.WithKubeconfigFile(kubeconfig)

		// 	return ctx, nil
		// },

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

func TestConvertHandler(t *testing.T) {
	os.Setenv("DEBUG", "1")
	tests := []struct {
		compositionDefinition string
		composition           string
		expectedStatus        int
		expectedBody          string
	}{
		{
			compositionDefinition: "fireworksapp.yaml",
			composition:           "fireworksapp.yaml",
			expectedStatus:        http.StatusInternalServerError,
		},
		{
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
				t.Fail()
			}

			err = decoder.DecodeEachFile(
				ctx, os.DirFS(filepath.Join(testdataPath, "crds")), "*.yaml",
				decoder.CreateIgnoreAlreadyExists(r),
			)
			if err != nil {
				t.Log("Error decoding CRDs: ", err)
				t.Fail()
			}

			resli, err := decoder.DecodeAllFiles(ctx, os.DirFS(filepath.Join(testdataPath, "crds")), "*.yaml")
			if err != nil {
				t.Log("Error decoding CRDs: ", err)
				t.Fail()
			}

			ress := unstructured.UnstructuredList{}
			for _, res := range resli {
				res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(res)
				if err != nil {
					t.Log("Error converting CRD: ", err)
					t.Fail()
				}
				ress.Items = append(ress.Items, unstructured.Unstructured{Object: res})
			}
			err = wait.For(
				conditions.New(r).ResourcesFound(&ress),
				wait.WithInterval(100*time.Millisecond),
			)
			if err != nil {
				t.Log("Error waiting for CRD: ", err)
				t.Fail()
			}

			apis.AddToScheme(r.GetScheme())

			err = decoder.DecodeEachFile(
				ctx, os.DirFS(filepath.Join(testdataPath, "compositions")), "*.yaml",
				decoder.CreateIgnoreAlreadyExists(r),
			)
			if err != nil {
				t.Log("Error decoding Compositions: ", err)
				t.Fail()
			}

			err = decoder.DecodeEachFile(
				ctx, os.DirFS(filepath.Join(testdataPath, "compositiondefinitions")), "*.yaml",
				decoder.CreateIgnoreAlreadyExists(r),
			)
			if err != nil {
				t.Log("Error decoding CompositionDefinitions: ", err)
				t.Fail()
			}

			r.WithNamespace(namespace)
			return ctx
		}).Assess("Testing GetResource", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		r, err := resources.New(c.Client().RESTConfig())
		if err != nil {
			t.Fail()
		}
		r.WithNamespace(namespace)

		apis.AddToScheme(r.GetScheme())

		dynamic := dynamic.NewForConfigOrDie(c.Client().RESTConfig())
		discovery := discovery.NewDiscoveryClientForConfigOrDie(c.Client().RESTConfig())
		cachedDisc := memory.NewMemCacheClient(discovery)

		for _, tt := range tests {
			composition := &unstructured.Unstructured{}
			err := decoder.DecodeFile(os.DirFS(filepath.Join(testdataPath, "compositions")), tt.composition, composition)
			if err != nil {
				t.Log("Error decoding Composition: ", err)
				t.Fail()
			}

			err = r.Get(ctx, composition.GetName(), composition.GetNamespace(), composition)
			if err != nil {
				t.Log("Error getting Composition: ", err)
				t.Fail()
			}

			cd := &unstructured.Unstructured{}
			err = decoder.DecodeFile(os.DirFS(filepath.Join(testdataPath, "compositiondefinitions")), tt.compositionDefinition, cd)
			if err != nil {
				t.Log("Error decoding CompositionDefinition: ", err)
				t.Fail()
			}

			err = r.Get(ctx, cd.GetName(), cd.GetNamespace(), cd)
			if err != nil {
				t.Log("Error getting CompositionDefinition: ", err)
				t.Fail()
			}

			req := httptest.NewRequest(http.MethodGet, "/resources", nil)
			values := req.URL.Query()
			values.Add("compositionUID", string(composition.GetUID()))
			values.Add("compositionNamespace", composition.GetNamespace())
			values.Add("compositionDefinitionUID", string(cd.GetUID()))
			values.Add("compositionDefinitionNamespace", cd.GetNamespace())
			req.URL.RawQuery = values.Encode()

			rec := httptest.NewRecorder()
			h := GetResources(handlers.HandlerOptions{
				Log:             slog.Default(),
				Client:          http.DefaultClient,
				DiscoveryClient: cachedDisc,
				DynamicClient:   dynamic,
				HelmClientOptions: ptr.To(helmclient.RestConfClientOptions{
					RestConfig: c.Client().RESTConfig(),
				}),
			})

			h.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			if len(tt.expectedBody) > 0 {
				respBody := strings.TrimSpace(rec.Body.String())
				assert.Equal(t, tt.expectedBody, respBody, "unexpected response body")
			}
		}

		return ctx
	}).Feature()

	testenv.Test(t, f)
}
