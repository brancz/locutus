package rollout

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/brancz/locutus/client"
)

var (
	DefaultObjectActions = []ObjectAction{
		&CreateOrUpdateObjectAction{},
		&CreateIfNotExistObjectAction{},
	}
)

type ObjectAction interface {
	Execute(*client.ResourceClient, *unstructured.Unstructured) error
	Name() string
}

type CreateOrUpdateObjectAction struct{}

func (a *CreateOrUpdateObjectAction) Execute(rc *client.ResourceClient, unstructured *unstructured.Unstructured) error {
	current, err := rc.Get(unstructured.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := rc.Create(unstructured, metav1.CreateOptions{})
		return err
	}

	_, err = rc.UpdateWithCurrent(current, unstructured)
	return err
}

func (a *CreateOrUpdateObjectAction) Name() string {
	return "CreateOrUpdate"
}

type CreateIfNotExistObjectAction struct{}

func (a *CreateIfNotExistObjectAction) Execute(rc *client.ResourceClient, unstructured *unstructured.Unstructured) error {
	_, err := rc.Get(unstructured.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := rc.Create(unstructured, metav1.CreateOptions{})
		return err
	}

	return nil
}

func (a *CreateIfNotExistObjectAction) Name() string {
	return "CreateIfNotExist"
}
