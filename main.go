package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/krateoplatformops/chart-inspector/docs"
	"github.com/krateoplatformops/chart-inspector/internal/handlers"
	getresources "github.com/krateoplatformops/chart-inspector/internal/handlers/resources/get"
	"github.com/krateoplatformops/chart-inspector/internal/helmclient"
	"github.com/krateoplatformops/plumbing/env"
	prettylog "github.com/krateoplatformops/plumbing/slogs/pretty"
	httpSwagger "github.com/swaggo/http-swagger"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	serviceName = "chart-inspector"
)

// @title 		 Chart Inspector API
// @version         1.0
// @description   This is the API for the Chart Inspector service. It provides endpoints for inspecting Helm charts.
// @BasePath		/
func main() {
	debugOn := flag.Bool("debug", env.Bool("DEBUG", false), "dump verbose output")
	port := flag.Int("port", env.Int("PLUGIN_PORT", 8081), "port to listen on")
	kubeconfig := flag.String("kubeconfig", env.String("KUBECONFIG", ""),
		"absolute path to the kubeconfig file")
	krateoNamespace := env.String("KRATEO_NAMESPACE", "krateo-system")

	flag.Parse()

	mux := http.NewServeMux()

	logLevel := slog.LevelInfo
	if *debugOn {
		logLevel = slog.LevelDebug
	}

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	lh := prettylog.New(&slog.HandlerOptions{
		Level:     logLevel,
		AddSource: false,
	},
		prettylog.WithDestinationWriter(os.Stderr),
		prettylog.WithColor(),
		prettylog.WithOutputEmptyAttrs(),
	)
	log := slog.New(lh)

	log = log.With("service", serviceName)

	// Kubernetes configuration
	var cfg *rest.Config
	var err error
	if len(*kubeconfig) > 0 {
		cfg, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Error("Building kubeconfig.", "error", err)
		os.Exit(1)
	}

	cfg.QPS = -1 // rely on k8s api server rate limiting

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Error("Creating dynamic client.", "error", err)
		os.Exit(1)
	}

	clientset, err := helmclient.NewCachedClients(cfg)
	if err != nil {
		log.Error("Creating cached clientset.", "error", err)
		os.Exit(1)
	}

	// Start CRD informer to invalidate discovery cache on CRD changes
	go func() {
		if err := helmclient.StartCRDInformer(context.Background(), cfg, &clientset, log); err != nil {
			log.Error("Starting CRD informer", "error", err)
		}
	}()

	opts := handlers.HandlerOptions{
		Log:             log,
		DynamicClient:   dyn,
		KrateoNamespace: krateoNamespace,
		HelmClientOptions: helmclient.RestConfClientOptions{
			RestConfig: cfg,
		},
		Clientset: &clientset,
	}

	healthy := int32(0)

	mux.Handle("/resources", getresources.GetResources(opts))
	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 50 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), []os.Signal{
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGKILL,
		syscall.SIGHUP,
		syscall.SIGQUIT,
	}...)
	defer stop()

	go func() {
		atomic.StoreInt32(&healthy, 1)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("could not listen on %s - %v", server.Addr, err)
			os.Exit(1)
		}
	}()

	// Listen for the interrupt signal.
	log.Info("server is ready to handle requests", slog.Any("port", *port))
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	log.Info("server is shutting down gracefully, press Ctrl+C again to force")
	atomic.StoreInt32(&healthy, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server.SetKeepAlivesEnabled(false)
	if err := server.Shutdown(ctx); err != nil {
		log.Error("server forced to shutdown", slog.Any("error", err))
		os.Exit(1)
	}

	log.Info("server gracefully stopped")
}
