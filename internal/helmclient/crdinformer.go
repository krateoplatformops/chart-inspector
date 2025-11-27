package helmclient

import (
	"context"
	"log/slog"
	"reflect"
	"time"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func StartCRDInformer(ctx context.Context, cfg *rest.Config, clients *CachedClients, log *slog.Logger) error {
	apiExtCli, err := apiextclient.NewForConfig(cfg)
	if err != nil {
		return err
	}
	// reuse testable implementation
	return StartCRDInformerWithClientset(ctx, apiExtCli, clients.mapper, log)
}

func StartCRDInformerWithClientset(ctx context.Context, apiExtCli apiextclient.Interface, invalidator interface{ Reset() }, log *slog.Logger) error {
	if apiExtCli == nil {
		return nil
	}

	factory := apiextinformers.NewSharedInformerFactory(apiExtCli, 30*time.Second)
	crdInformer := factory.Apiextensions().V1().CustomResourceDefinitions().Informer()

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if invalidator != nil {
				invalidator.Reset()
				if log != nil {
					crd, ok := obj.(*apixv1.CustomResourceDefinition)
					if !ok {
						return
					}
					log.Debug("discovery cache invalidated: CRD added", "name", crd.Name)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldCRD, ok1 := oldObj.(*apixv1.CustomResourceDefinition)
			newCRD, ok2 := newObj.(*apixv1.CustomResourceDefinition)
			if !ok1 || !ok2 {
				return
			}

			// compare Specs: only invalidate if Spec actually changed
			if reflect.DeepEqual(oldCRD.Spec, newCRD.Spec) {
				// nothing meaningful changed
				return
			}

			if invalidator != nil {
				invalidator.Reset()
				if log != nil {
					log.Debug("discovery cache invalidated: CRD spec changed", "name", newCRD.Name)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			if invalidator != nil {
				invalidator.Reset()
				if log != nil {
					crd, ok := obj.(*apixv1.CustomResourceDefinition)
					if !ok {
						return
					}
					log.Debug("discovery cache invalidated: CRD deleted", "name", crd.Name)
				}
			}
		},
	}

	crdInformer.AddEventHandler(handler)

	// start informers
	factory.Start(ctx.Done())

	// wait for sync in background
	go func() {
		if ok := cache.WaitForCacheSync(ctx.Done(), crdInformer.HasSynced); !ok {
			if log != nil {
				log.Warn("CRD informer failed to sync")
			}
			return
		}
		if log != nil {
			log.Debug("CRD informer synced")
		}
	}()

	return nil
}
