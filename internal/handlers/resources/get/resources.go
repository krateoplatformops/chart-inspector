package resources

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/krateoplatformops/chart-inspector/internal/getter"
	"github.com/krateoplatformops/chart-inspector/internal/handlers"
	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"github.com/krateoplatformops/chart-inspector/internal/tracer"
	"github.com/krateoplatformops/snowplow/plumbing/http/response"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
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
// @Param compositionUID query string true "Composition name"
// @Param compositionNamespace query string true "Composition namespace"
// @Param compositionDefinitionUID query string true "Composition definition name"
// @Param compositionDefinitionNamespace query string true "Composition definition namespace"
// @Produce json
// @Success 200 {object} []Resource
// @Router /resources [get]
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	compositionUID := r.URL.Query().Get("compositionUID")
	compositionNamespace := r.URL.Query().Get("compositionNamespace")
	compositionDefinitionUID := r.URL.Query().Get("compositionDefinitionUID")
	compositionDefinitionNamespace := r.URL.Query().Get("compositionDefinitionNamespace")

	if compositionUID == "" || compositionNamespace == "" || compositionDefinitionUID == "" || compositionDefinitionNamespace == "" {
		h.Log.Error("missing required query parameters",
			slog.String("compositionUID", compositionUID),
			slog.String("compositionNamespace", compositionNamespace),
			slog.String("compositionDefinitionUID", compositionDefinitionUID),
			slog.String("compositionDefinitionNamespace", compositionDefinitionNamespace),
		)
		response.BadRequest(w, fmt.Errorf("missing required query parameters"))
		return
	}

	// Getting the resources
	k8scli := getter.NewClient(
		h.DynamicClient,
		h.DiscoveryClient,
	)

	composition, err := k8scli.GetComposition(compositionUID, compositionNamespace)
	if err != nil {
		h.Log.Error("unable to get composition",
			slog.String("compositionUID", compositionUID),
			slog.String("compositionNamespace", compositionNamespace),
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

	compositionDefinition, err := k8scli.GetCompositionDefinition(compositionDefinitionUID, compositionDefinitionNamespace)
	if err != nil {
		h.Log.Error("unable to get composition definition",
			slog.String("compositionDefinitionUID", compositionDefinitionUID),
			slog.String("compositionDefinitionNamespace", compositionDefinitionNamespace),
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
			slog.String("compositionUID", compositionUID),
			slog.String("compositionNamespace", compositionNamespace),
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}

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
				slog.String("compositionUID", compositionUID),
				slog.String("compositionNamespace", compositionNamespace),
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
	resources := tracer.GetResources()

	// write the response in JSON format
	w.Header().Set("Content-Type", "application/json")
	// w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	err = enc.Encode(resources)
	if err != nil {
		h.Log.Error("unable to marshal resources",
			slog.Any("err", err),
		)
		response.InternalError(w, err)
		return
	}
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
