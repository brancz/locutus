package client

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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

func NewClient(cfg *rest.Config, kclient kubernetes.Interface) *Client {
	c := &Client{
		logger:  log.NewNopLogger(),
		kclient: kclient,
		cfg:     cfg,
	}

	return c
}

func (c *Client) WithLogger(logger log.Logger) {
	c.logger = logger
}

func (c *Client) SetUpdatePreparations(preparations []UpdatePreparation) {
	c.updatePreparations = preparations
}

func (c *Client) ClientForUnstructured(u *unstructured.Unstructured) (*ResourceClient, error) {
	return c.ClientFor(u.GetAPIVersion(), u.GetKind(), u.GetNamespace())
}

func (c *Client) ClientFor(apiVersion, kind, namespace string) (*ResourceClient, error) {

	apiResourceList, err := c.kclient.Discovery().ServerResourcesForGroupVersion(apiVersion)
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

	resourceName := ""
	for _, r := range apiResourceList.APIResources {
		if r.Kind == kind {
			resourceName = r.Name
			break
		}
	}

	gvr := schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: resourceName,
	}

	return &ResourceClient{ResourceInterface: dc.Resource(gvr).Namespace(namespace), updatePreparations: c.updatePreparations}, nil
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
	return nil
}

type ResourceClient struct {
	dynamic.ResourceInterface

	updatePreparations []UpdatePreparation
}

func (rc *ResourceClient) UpdateWithCurrent(ctx context.Context, current, updated *unstructured.Unstructured, subresources ...string) (*unstructured.Unstructured, error) {
	if err := rc.prepareUnstructuredForUpdate(current, updated); err != nil {
		return nil, err
	}

	return rc.ResourceInterface.Update(ctx, updated, v1.UpdateOptions{}, subresources...)
}

func (rc *ResourceClient) prepareUnstructuredForUpdate(current, updated *unstructured.Unstructured) error {
	updated.SetResourceVersion(current.GetResourceVersion())

	for _, p := range rc.updatePreparations {
		if err := p.Prepare(current, updated); err != nil {
			return err
		}
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
