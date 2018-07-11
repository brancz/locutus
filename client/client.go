package client

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

var (
	DefaultUpdatePreparations = []UpdatePreparation{
		UpdatePreparationFunc(PrepareServiceForUpdate),
		UpdatePreparationFunc(PrepareDeploymentForUpdate),
	}
)

type UpdatePreparation interface {
	Prepare(current, updated *unstructured.Unstructured) error
}

type UpdatePreparationFunc func(current, updated *unstructured.Unstructured) error

func (f UpdatePreparationFunc) Prepare(current, updated *unstructured.Unstructured) error {
	return f(current, updated)
}

type Client struct {
	kclient            kubernetes.Interface
	cfg                *rest.Config
	updatePreparations []UpdatePreparation
	logger             log.Logger
}

func NewClient(logger log.Logger, cfg *rest.Config, kclient kubernetes.Interface) *Client {
	c := &Client{
		logger:  logger,
		kclient: kclient,
		cfg:     cfg,
	}

	return c
}

func (c *Client) SetUpdatePreparations(preparations []UpdatePreparation) {
	c.updatePreparations = preparations
	return
}

func (c *Client) ClientForUnstructured(u *unstructured.Unstructured) (*ResourceClient, error) {
	return c.ClientFor(u.GetAPIVersion(), u.GetKind(), u.GetNamespace())
}

func (c *Client) ClientFor(apiVersion, kind, namespace string) (*ResourceClient, error) {
	apiResourceList, apiResource, err := c.getAPIResource(apiVersion, kind)
	if err != nil {
		return nil, errors.Wrapf(err, "discovering resource information failed for %s in %s", kind, apiVersion)
	}

	dc, err := newForConfig(apiResourceList.GroupVersion, c.cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "creating dynamic client failed for %s", apiResourceList.GroupVersion)
	}

	gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing GroupVersion failed %s", apiResourceList.GroupVersion)
	}

	gvr := schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: apiResource.Name,
	}

	return &ResourceClient{ResourceInterface: dc.Resource(gvr).Namespace(namespace), updatePreparations: c.updatePreparations}, nil
}

func (c *Client) getAPIResource(apiVersion, kind string) (*metav1.APIResourceList, *metav1.APIResource, error) {
	apiResourceLists, err := c.kclient.Discovery().ServerResources()
	if err != nil {
		return nil, nil, err
	}

	for _, apiResourceList := range apiResourceLists {
		if apiResourceList.GroupVersion == apiVersion {
			for _, r := range apiResourceList.APIResources {
				if r.Kind == kind {
					return apiResourceList, &r, nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("apiVersion %s and kind %s not found available in Kubernetes cluster", apiVersion, kind)
}

func newForConfig(groupVersion string, c *rest.Config) (dynamic.Interface, error) {
	config := *c
	err := setConfigDefaults(groupVersion, &config)
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(&config)
}

func setConfigDefaults(groupVersion string, config *rest.Config) error {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return err
	}
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	if config.GroupVersion.Group == "" && config.GroupVersion.Version == "v1" {
		config.APIPath = "/api"
	}
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}
	return nil
}

type ResourceClient struct {
	dynamic.ResourceInterface

	updatePreparations []UpdatePreparation
}

func (rc *ResourceClient) UpdateWithCurrent(current, updated *unstructured.Unstructured, subresources ...string) (*unstructured.Unstructured, error) {
	rc.prepareUnstructuredForUpdate(current, updated)

	return rc.ResourceInterface.Update(updated, subresources...)
}

func (rc *ResourceClient) prepareUnstructuredForUpdate(current, updated *unstructured.Unstructured) error {
	updated.SetResourceVersion(current.GetResourceVersion())

	for _, p := range rc.updatePreparations {
		p.Prepare(current, updated)
	}

	return nil
}

func PrepareServiceForUpdate(current, updated *unstructured.Unstructured) error {
	if updated.GetAPIVersion() == "v1" && updated.GetKind() == "Service" {
		clusterIP, found, err := unstructured.NestedString(current.Object, "spec", "clusterIP")
		if err != nil {
			return err
		}

		if found {
			return unstructured.SetNestedField(updated.Object, clusterIP, "spec", "clusterIP")
		}
	}

	return nil
}

const (
	deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"
)

func PrepareDeploymentForUpdate(current, updated *unstructured.Unstructured) error {
	if (updated.GetAPIVersion() == "apps/v1" || updated.GetAPIVersion() == "v1beta2") && updated.GetKind() == "Deployment" {
		updatedAnnotations := updated.GetAnnotations()
		if updatedAnnotations == nil {
			updatedAnnotations = map[string]string{}
		}
		curAnnotations := current.GetAnnotations()

		if curAnnotations != nil {
			updatedAnnotations[deploymentRevisionAnnotation] = curAnnotations[deploymentRevisionAnnotation]
			updated.SetAnnotations(updatedAnnotations)
		}
	}

	return nil
}
