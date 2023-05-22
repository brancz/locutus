package checks

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/brancz/locutus/client"
	"github.com/pkg/errors"
)

func TestJobOOMError(t *testing.T) {
	testErr := errors.New("test error")

	fakeKubeClient := fake.NewSimpleClientset()
	fakeKubeClient.PrependReactor("*", "*", kubetesting.ReactionFunc(func(action kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, testErr
	}))
	fakeClient := client.NewClient(nil, fakeKubeClient)

	c := &JobOOMKilledCheck{}
	err := c.Execute(context.Background(), fakeClient, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Job",
			"metadata": map[string]interface{}{
				"name":      "test",
				"namespace": "test-namespace",
			},
		},
	})
	if !errors.Is(err, testErr) {
		t.Fatal(err)
	}
}

func TestJobOOMErrorIfNotJob(t *testing.T) {
	c := &JobOOMKilledCheck{}
	err := c.Execute(context.Background(), nil, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Deployment",
			"metadata": map[string]interface{}{
				"name":      "test",
				"namespace": "test-namespace",
			},
		},
	})
	if err != ErrNotAJob {
		t.Fatalf("expected NotAJobError, but got: %v", err)
	}
}

func TestIsFailedError(t *testing.T) {
	c := &JobOOMKilledCheck{}
	err := fmt.Errorf("pod %s/%s was OOMKilled: %w", "default", "test", ErrOOMKilled)

	if !c.IsFailedError(err) {
		t.Fatalf("expected error to be failed, but it was not")
	}
}
