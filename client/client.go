package client

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
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
	DefaultUpdateChecks = []UpdateCheck{
		UpdateCheckFunc(CheckServiceAccountForUpdate),
	}
)

type UpdatePreparation interface {
	Prepare(current, updated *unstructured.Unstructured) error
}

type UpdatePreparationFunc func(current, updated *unstructured.Unstructured) error

func (f UpdatePreparationFunc) Prepare(current, updated *unstructured.Unstructured) error {
	return f(current, updated)
}

type UpdateCheck interface {
	Check(current, updated *unstructured.Unstructured) (bool, error)
}

type UpdateCheckFunc func(current, updated *unstructured.Unstructured) (bool, error)

func (f UpdateCheckFunc) Check(current, updated *unstructured.Unstructured) (bool, error) {
	return f(current, updated)
}

type Client struct {
	kclient            kubernetes.Interface
	cfg                *rest.Config
	updatePreparations []UpdatePreparation
	updateChecks       []UpdateCheck
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

func (c *Client) KubeClient() kubernetes.Interface {
	return c.kclient
}

func (c *Client) WithLogger(logger log.Logger) {
	c.logger = logger
}

func (c *Client) SetUpdatePreparations(preparations []UpdatePreparation) {
	c.updatePreparations = preparations
}

func (c *Client) SetUpdateChecks(checks []UpdateCheck) {
	c.updateChecks = checks
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

	return &ResourceClient{ResourceInterface: dc.Resource(gvr).Namespace(namespace), updatePreparations: c.updatePreparations, updateChecks: c.updateChecks}, nil
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
	updateChecks       []UpdateCheck
}

func (rc *ResourceClient) UpdateWithCurrent(ctx context.Context, current, updated *unstructured.Unstructured, subresources ...string) (*unstructured.Unstructured, error) {
	if err := rc.prepareUnstructuredForUpdate(current, updated); err != nil {
		return nil, err
	}

	needUpdate, err := rc.checkUnstructuredForUpdate(current, updated)
	if err != nil {
		return nil, err
	}
	if !needUpdate {
		return nil, nil
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

func (rc *ResourceClient) checkUnstructuredForUpdate(current, updated *unstructured.Unstructured) (bool, error) {
	for _, p := range rc.updateChecks {
		needUpdate, err := p.Check(current, updated)
		if err != nil {
			return needUpdate, err
		}
		if !needUpdate {
			return needUpdate, nil
		}
	}

	return true, nil
}

// For ServiceAccount, check if name/namespace/labels/annotations change, then do update.
// or else, it creates secrets per time when do update.
func CheckServiceAccountForUpdate(current, updated *unstructured.Unstructured) (bool, error) {
	if updated.GetAPIVersion() == "v1" && updated.GetKind() == "ServiceAccount" {
		currentJSON, _ := current.MarshalJSON()
		currentSA := &corev1.ServiceAccount{}
		err := json.Unmarshal(currentJSON, currentSA)
		if err != nil {
			return true, err
		}

		updatedJSON, _ := updated.MarshalJSON()
		updatedSA := &corev1.ServiceAccount{}
		err = json.Unmarshal(updatedJSON, updatedSA)
		if err != nil {
			return true, err
		}

		if currentSA.ObjectMeta.GetName() == updatedSA.ObjectMeta.GetName() &&
			currentSA.ObjectMeta.GetNamespace() == updatedSA.ObjectMeta.GetNamespace() &&
			reflect.DeepEqual(currentSA.ObjectMeta.GetLabels(), updatedSA.ObjectMeta.GetLabels()) &&
			reflect.DeepEqual(currentSA.ObjectMeta.GetAnnotations(), updatedSA.ObjectMeta.GetAnnotations()) {
			return false, nil
		}
	}
	return true, nil
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
