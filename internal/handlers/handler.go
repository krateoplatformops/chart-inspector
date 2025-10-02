package handlers

import (
	"log/slog"

	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"k8s.io/client-go/dynamic"
)

type HandlerOptions struct {
	Log *slog.Logger
	// Client            *http.Client
	HelmClientOptions helmclient.RestConfClientOptions
	Clientset         helmclient.CachedClients
	DynamicClient     dynamic.Interface
	KrateoNamespace   string
}
