package rollout

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/brancz/locutus/client"
)

var (
	DefaultObjectActions = []ObjectAction{
		&CreateOrUpdateObjectAction{},
		&CreateIfNotExistObjectAction{},
		&DeleteIfExistsObjectAction{},
	}
)

type ObjectAction interface {
	Execute(context.Context, *client.ResourceClient, *unstructured.Unstructured) error
	Name() string
}

type CreateOrUpdateObjectAction struct{}

func (a *CreateOrUpdateObjectAction) Execute(ctx context.Context, rc *client.ResourceClient, unstructured *unstructured.Unstructured) error {
	current, err := rc.Get(ctx, unstructured.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := rc.Create(ctx, unstructured, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	_, err = rc.UpdateWithCurrent(ctx, current, unstructured)
	return err
}

func (a *CreateOrUpdateObjectAction) Name() string {
	return "CreateOrUpdate"
}

type CreateIfNotExistObjectAction struct{}

func (a *CreateIfNotExistObjectAction) Execute(ctx context.Context, rc *client.ResourceClient, unstructured *unstructured.Unstructured) error {
	_, err := rc.Get(ctx, unstructured.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := rc.Create(ctx, unstructured, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	return nil
}

func (a *CreateIfNotExistObjectAction) Name() string {
	return "CreateIfNotExist"
}

type DeleteIfExistsObjectAction struct{}

func (a *DeleteIfExistsObjectAction) Execute(ctx context.Context, rc *client.ResourceClient, unstructured *unstructured.Unstructured) error {
	propagationPolicy := metav1.DeletePropagationForeground
	err := rc.Delete(ctx, unstructured.GetName(), metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return nil
}

func (a *DeleteIfExistsObjectAction) Name() string {
	return "DeleteIfExist"
}
