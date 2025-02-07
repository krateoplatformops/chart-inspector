package handlers

import (
	"log/slog"
	"net/http"

	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

type HandlerOptions struct {
	Log               *slog.Logger
	Client            *http.Client
	HelmClientOptions *helmclient.RestConfClientOptions
	DiscoveryClient   discovery.CachedDiscoveryInterface
	DynamicClient     dynamic.Interface
}
