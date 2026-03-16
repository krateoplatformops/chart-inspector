package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	coreprovv1 "github.com/krateoplatformops/core-provider/apis/compositiondefinitions/v1alpha1"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"

	"github.com/krateoplatformops/chart-inspector/internal/getter"
	"github.com/krateoplatformops/chart-inspector/internal/handlers"
	"github.com/krateoplatformops/chart-inspector/internal/handlers/resources"
	"github.com/krateoplatformops/chart-inspector/internal/helper"
	"github.com/krateoplatformops/chart-inspector/internal/tracer"
	compositionMeta "github.com/krateoplatformops/composition-dynamic-controller/pkg/meta"
	helmconfig "github.com/krateoplatformops/plumbing/helm"
	helmutils "github.com/krateoplatformops/plumbing/helm/utils"
	helmv3 "github.com/krateoplatformops/plumbing/helm/v3"
	"github.com/krateoplatformops/plumbing/http/response"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

const (
	AnnotationKeyReconciliationGracefullyPaused = "krateo.io/gracefully-paused"
)

type handler struct {
	handlers.HandlerOptions
}

func GetResources(opts handlers.HandlerOptions) http.Handler {
	return &handler{
		HandlerOptions: opts,
	}
}

var _ http.Handler = (*handler)(nil)

// @Summary Get Helm chart resources
// @Description Get Helm chart resources
// @ID get-chart-resources
// @Param compositionName query string true "Composition name"
// @Param compositionNamespace query string true "Composition namespace"
// @Param compositionDefinitionName query string true "Composition definition name"
// @Param compositionDefinitionNamespace query string true "Composition definition namespace"
// @Param compositionDefinitionGroup query string false "Composition definition group" default(core.krateo.io)
// @Param compositionDefinitionVersion query string false "Composition definition version" default(v1alpha1)
// @Param compositionDefinitionResource query string false "Composition definition resource name" default(compositiondefinitions)
// @Param compositionGroup query string false "Composition group" default(composition.krateo.io)
// @Param compositionVersion query string true "Composition version"
// @Param compositionResource query string true "Composition resource name"
// @Produce json
// @Success 200 {object} []Resource
// @Router /resources [get]
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	compositionName := r.URL.Query().Get("compositionName")
	compositionNamespace := r.URL.Query().Get("compositionNamespace")
	compositionDefinitionName := r.URL.Query().Get("compositionDefinitionName")
	compositionDefinitionNamespace := r.URL.Query().Get("compositionDefinitionNamespace")
	compositionVersion := r.URL.Query().Get("compositionVersion")
	compositionResource := r.URL.Query().Get("compositionResource")

	compositionGroup := helper.GetQueryParamWithDefault(r, "compositionGroup", "composition.krateo.io")
	compositionDefinitionGroup := helper.GetQueryParamWithDefault(r, "compositionDefinitionGroup", "core.krateo.io")
	compositionDefinitionVersion := helper.GetQueryParamWithDefault(r, "compositionDefinitionVersion", "v1alpha1")
	compositionDefinitionResource := helper.GetQueryParamWithDefault(r, "compositionDefinitionResource", "compositiondefinitions")

	log := h.Log.With(slog.String(
		"compositionName", compositionName),
		slog.String("compositionNamespace", compositionNamespace),
		slog.String("compositionDefinitionName", compositionDefinitionName),
		slog.String("compositionDefinitionNamespace", compositionDefinitionNamespace))

	if compositionName == "" || compositionNamespace == "" || compositionDefinitionName == "" || compositionDefinitionNamespace == "" || compositionVersion == "" || compositionResource == "" {
		log.Error("missing required query parameters")
		response.BadRequest(w, fmt.Errorf("missing required query parameters"))
		return
	}

	k8scli := getter.NewClient(
		h.DynamicClient,
	)

	compositionGVR := schema.GroupVersionResource{
		Group:    compositionGroup,
		Version:  compositionVersion,
		Resource: compositionResource,
	}
	compositionDefinitionGVR := schema.GroupVersionResource{
		Group:    compositionDefinitionGroup,
		Version:  compositionDefinitionVersion,
		Resource: compositionDefinitionResource,
	}

	log.Info("Handling request to get resources")

	composition, err := h.DynamicClient.
		Resource(compositionGVR).
		Namespace(compositionNamespace).
		Get(context.Background(), compositionName, v1.GetOptions{})
	if err != nil {
		log.Error("unable to get composition",
			slog.String("compositionName", compositionName),
			slog.String("compositionNamespace", compositionNamespace),
			slog.String("compositionVersion", compositionVersion),
			slog.String("compositionResource", compositionResource),
			slog.String("compositionGroup", compositionGroup),
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	// NOTE: bValues are extracted and injected with composition context
	bValuesMap, err := helmutils.ValuesFromSpec(composition)
	if err != nil {
		log.Error("unable to extract values from composition",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}
	err = bValuesMap.InjectGlobalValues(composition, h.Plurarizer, h.KrateoNamespace)
	if err != nil {
		log.Error("unable to inject global values",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	tracer := &tracer.Tracer{}
	// Create a wrapped REST config with the tracer RoundTripper for this request
	wrappedCfg := rest.CopyConfig(h.RestConfig)
	wrappedCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return tracer.WithRoundTripper(rt)
	}

	// Create a temporary helm client with tracer integration for this request
	tracedHelmClient, err := helmv3.NewClient(wrappedCfg,
		helmv3.WithLogger(func(format string, v ...interface{}) {
			log.Debug(fmt.Sprintf(format, v...))
		}),
		helmv3.WithNamespace(compositionNamespace),
		helmv3.WithCRDInformer(wrappedCfg, 30*time.Minute),
	)
	if err != nil {
		log.Error("unable to create traced helm client",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}
	defer tracedHelmClient.Close()

	compositionDefinitionU, err := h.DynamicClient.
		Resource(compositionDefinitionGVR).
		Namespace(compositionDefinitionNamespace).
		Get(context.Background(), compositionDefinitionName, v1.GetOptions{})
	if err != nil {
		log.Error("unable to get composition definition",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}
	var compositionDefinition coreprovv1.CompositionDefinition
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(compositionDefinitionU.Object, &compositionDefinition)
	if err != nil {
		log.Error("unable to convert composition definition",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	// Install the Helm chart with DryRun to capture templated resources
	// Set namespace to composition's namespace for proper resource isolation
	installCfg := &helmconfig.InstallConfig{
		ActionConfig: &helmconfig.ActionConfig{
			ChartVersion:          compositionDefinition.Spec.Chart.Version,
			ChartName:             compositionDefinition.Spec.Chart.Repo,
			Values:                bValuesMap,
			Username:              "",
			Password:              "",
			InsecureSkipTLSverify: compositionDefinition.Spec.Chart.InsecureSkipVerifyTLS,
			DryRun:                helmconfig.DryRunServer,
			IncludeCRDs:           true,
			SkipCRDs:              false,
		},
		CreateNamespace: true,
	}

	// Retrieve credentials from secret if specified
	if compositionDefinition.Spec.Chart != nil && compositionDefinition.Spec.Chart.Credentials != nil {
		installCfg.ActionConfig.Username = compositionDefinition.Spec.Chart.Credentials.Username

		// Retrieve password from secret
		passwd, err := k8scli.GetSecret(compositionDefinition.Spec.Chart.Credentials.PasswordRef)
		if err != nil {
			log.Error("unable to get secret",
				slog.Any("err", err),
			)
			response.InternalError(w, err)
			return
		}
		installCfg.ActionConfig.Password = passwd
	}

	// Install with DryRun to get templated manifest using traced helm client
	_, err = tracedHelmClient.Install(context.Background(), compositionMeta.GetReleaseName(composition), compositionDefinition.Spec.Chart.Url, installCfg)
	if err != nil {
		log.Error("unable to template chart",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	// Getting the resources
	resLi := tracer.GetResources()

	// Ensure resLi is not nil to avoid null in JSON response
	if resLi == nil {
		resLi = []resources.Resource{}
	}
	if meta.IsVerbose(composition) {
		b, err := json.Marshal(resLi)
		if err != nil {
			log.Error("unable to marshal resources for logging",
				slog.Any("err", err),
			)
		} else {
			log.Debug("Retrieved resources",
				slog.String("resources", string(b)),
			)
		}
	}

	// write the response in JSON format
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err = enc.Encode(resLi)
	if err != nil {
		log.Error("unable to marshal resources",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	log.Info("Successfully handled request to get resources")
}
