package handlers

import (
	"log/slog"

	helmconfig "github.com/krateoplatformops/plumbing/helm"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type HandlerOptions struct {
	Log             *slog.Logger
	DynamicClient   dynamic.Interface
	KrateoNamespace string
	HelmClient      helmconfig.Client
	RestConfig      *rest.Config
}
