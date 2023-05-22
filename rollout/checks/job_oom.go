package checks

import (
	"context"
	"fmt"

	"github.com/brancz/locutus/client"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var DefaultChecks = []Check{
	&JobOOMKilledCheck{},
}

type JobOOMKilledCheck struct{}

func (c JobOOMKilledCheck) Name() string {
	return "JobOOMKilled"
}

var (
	ErrOOMKilled = errors.New("at least one pod of job were OOMKilled")
	ErrNotAJob   = errors.New("object is not a Job")
)

func (c *JobOOMKilledCheck) Execute(ctx context.Context, client *client.Client, unstructured *unstructured.Unstructured) error {
	if unstructured.GetKind() != "Job" {
		return ErrNotAJob
	}

	list, err := client.KubeClient().CoreV1().Pods(unstructured.GetNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", unstructured.GetName()),
	})
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}

	for _, pod := range list.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.Reason == "OOMKilled" {
				return fmt.Errorf("pod %s/%s was OOMKilled: %w", pod.GetNamespace(), pod.GetName(), ErrOOMKilled)
			}
		}
	}

	return nil
}

func (c *JobOOMKilledCheck) IsFailedError(err error) bool {
	return errors.Is(err, ErrOOMKilled)
}
