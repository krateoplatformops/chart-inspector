package getter

import (
	"context"
	"fmt"
	"strings"

	coreprovv1 "github.com/krateoplatformops/core-provider/apis/compositiondefinitions/v1alpha1"
	rtv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c *Client) GetComposition(uid string, namespace string) (*unstructured.Unstructured, error) {

	_, apiResourceList, err := c.discovery.ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover API resources: %v", err)
	}

	for _, apiResource := range apiResourceList {
		group, err := schema.ParseGroupVersion(apiResource.GroupVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to parse group version: %v", err)
		}

		if group.Group == "composition.krateo.io" {
			// b, _ := json.MarshalIndent(apiResource, "", "  ")
			// fmt.Println("group", string(b))

			var resource string
			for _, r := range apiResource.APIResources {
				if !strings.Contains(r.Name, "/status") {
					resource = r.Name
					break
				}
			}

			gvr := schema.GroupVersionResource{
				Group:    group.Group,
				Version:  group.Version,
				Resource: resource,
			}
			li, err := c.dynamic.Resource(gvr).Namespace(namespace).List(context.Background(), v1.ListOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to list composition: %v", err)
			}

			for _, item := range li.Items {
				if string(item.GetUID()) == uid {
					return &item, nil
				}
			}

		}
	}

	return nil, errors.NewNotFound(schema.GroupResource{
		Group:    "composition.krateo.io",
		Resource: "compositions",
	}, uid)
}

const (
	CompositionDefinitionGroup    = "core.krateo.io"
	CompositionDefinitionVersion  = "v1alpha1"
	CompositionDefinitionResource = "compositiondefinitions"
)

func (c *Client) GetCompositionDefinition(uid string, namespace string) (*coreprovv1.CompositionDefinition, error) {
	li, err := c.dynamic.Resource(schema.GroupVersionResource{
		Group:    CompositionDefinitionGroup,
		Version:  CompositionDefinitionVersion,
		Resource: CompositionDefinitionResource,
	}).Namespace(namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list composition definitions: %v", err)
	}

	for _, item := range li.Items {
		if string(item.GetUID()) == uid {
			var compDef coreprovv1.CompositionDefinition
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &compDef)
			return &compDef, err
		}
	}

	return nil, errors.NewNotFound(schema.GroupResource{
		Group:    CompositionDefinitionGroup,
		Resource: CompositionDefinitionResource,
	}, uid)
}

func (c *Client) GetSecret(selector rtv1.SecretKeySelector) (string, error) {
	sec, err := c.dynamic.Resource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
		Group:    "",
	}).Namespace(selector.Namespace).Get(context.Background(), selector.Name, v1.GetOptions{})
	if err != nil {
		return "", errors.NewNotFound(schema.GroupResource{
			Resource: "secrets",
		}, selector.Name)
	}
	var secret corev1.Secret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(sec.Object, &secret); err != nil {
		return "", fmt.Errorf("failed to convert secret: %v", err)
	}

	return string(secret.Data[selector.Key]), nil
}
