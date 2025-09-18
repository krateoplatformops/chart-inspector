package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	coreprovv1 "github.com/krateoplatformops/core-provider/apis/compositiondefinitions/v1alpha1"

	"github.com/krateoplatformops/chart-inspector/internal/getter"
	"github.com/krateoplatformops/chart-inspector/internal/handlers"
	"github.com/krateoplatformops/chart-inspector/internal/handlers/resources"
	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"github.com/krateoplatformops/chart-inspector/internal/helmclient/tools"
	"github.com/krateoplatformops/chart-inspector/internal/helper"
	"github.com/krateoplatformops/chart-inspector/internal/tracer"
	"github.com/krateoplatformops/plumbing/http/response"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"
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
	defer func() {
		if rec := recover(); rec != nil {
			h.Log.Error("panic in ServeHTTP",
				slog.Any("panic", rec))
			response.InternalError(w, fmt.Errorf("internal server error"))
		}
	}()

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

	tracer := &tracer.Tracer{}
	// Getting the resources
	h.HelmClientOptions.RestConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return tracer.WithRoundTripper(rt)
	}

	h.HelmClientOptions.Options = &helmclient.Options{
		Namespace: composition.GetNamespace(),
	}

	helmcli, err := helmclient.NewClientFromRestConf(h.HelmClientOptions)
	if err != nil {
		h.Log.Error("unable to create helm client",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	bValues, err := ExtractValuesFromSpec(composition)
	if err != nil {
		h.Log.Error("unable to extract values from composition",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	compositionDefinitionU, err := h.DynamicClient.
		Resource(compositionDefinitionGVR).
		Namespace(compositionDefinitionNamespace).
		Get(context.Background(), compositionDefinitionName, v1.GetOptions{})
	if err != nil {
		h.Log.Error("unable to get composition definition",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}
	var compositionDefinition coreprovv1.CompositionDefinition
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(compositionDefinitionU.Object, &compositionDefinition)
	if err != nil {
		h.Log.Error("unable to convert composition definition",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	bValues, err = tools.InjectValues(bValues, tools.CompositionValues{
		KrateoNamespace:      h.KrateoNamespace,
		CompositionName:      compositionName,
		CompositionNamespace: compositionNamespace,
		CompositionId:        string(composition.GetUID()),
		CompositionGroup:     compositionGroup,
		CompositionResource:  compositionResource,
		CompositionKind:      composition.GetKind(),
		GracefullyPaused:     composition.GetAnnotations()[AnnotationKeyReconciliationGracefullyPaused] == "true",
	})

	chartSpec := helmclient.ChartSpec{
		InsecureSkipTLSverify: compositionDefinition.Spec.Chart.InsecureSkipVerifyTLS,
		ReleaseName:           meta.GetReleaseName(composition),
		Namespace:             composition.GetNamespace(),
		ChartName:             compositionDefinition.Spec.Chart.Url,
		Version:               compositionDefinition.Spec.Chart.Version,
		Repo:                  compositionDefinition.Spec.Chart.Repo,
		ValuesYaml:            string(bValues),
	}
	if compositionDefinition.Spec.Chart != nil && compositionDefinition.Spec.Chart.Credentials != nil {
		passwd, err := k8scli.GetSecret(compositionDefinition.Spec.Chart.Credentials.PasswordRef)
		if err != nil {
			h.Log.Error("unable to get secret",
				slog.Any("err", err),
			)
			response.InternalError(w, err)
			return
		}

		chartSpec.Username = compositionDefinition.Spec.Chart.Credentials.Username
		chartSpec.Password = passwd
	}

	_, err = helmcli.TemplateChartRaw(&chartSpec, nil)
	if err != nil {
		h.Log.Error("unable to template chart",
			slog.Any("err", err),
		)

		response.InternalError(w, err)
		return
	}

	// Getting the resources
	resLi := tracer.GetResources()

	// assicurarsi di rispondere sempre con un array JSON invece di null/vuoto
	if resLi == nil {
		resLi = []resources.Resource{}
	}

	// write the response in JSON format
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err = enc.Encode(resLi)
	if err != nil {
		h.Log.Error("unable to marshal resources",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	h.Log.Info("Successfully handled request to get resources")
}

func ExtractValuesFromSpec(un *unstructured.Unstructured) ([]byte, error) {
	if un == nil {
		return nil, nil
	}

	spec, ok, err := unstructured.NestedMap(un.UnstructuredContent(), "spec")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	return sigsyaml.Marshal(spec)
}
