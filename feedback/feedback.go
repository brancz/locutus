package feedback

import (
	"context"
	"reflect"
	"time"

	"github.com/brancz/locutus/client"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Status struct {
	Conditions []*StatusCondition `json:"conditions"`
}

type StatusCondition struct {
	LastTransitionTime metav1.Time   `json:"lastTransitionTime"`
	Name               string        `json:"name"`
	CurrentStatus      CurrentStatus `json:"currentStatus"`
}

type CurrentStatus string

const (
	StatusConditionNotStarted CurrentStatus = "Not Started"
	StatusConditionInProgress CurrentStatus = "In Progress"
	StatusConditionFinished   CurrentStatus = "Finished"
)

func extractStatus(u *unstructured.Unstructured) *Status {
	field, found, err := unstructured.NestedFieldNoCopy(u.Object, "status", "conditions")
	if !found || err != nil {
		return nil
	}
	original, ok := field.([]interface{})
	if !ok {
		return nil
	}
	ret := make([]*StatusCondition, 0, len(original))
	for _, obj := range original {
		o, ok := obj.(map[string]interface{})
		if !ok {
			// expected map[string]interface{}, got something else
			return nil
		}
		sc := extractStatusCondition(o)
		ret = append(ret, sc)
	}
	return &Status{
		Conditions: ret,
	}
}

func getNestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}

func extractStatusCondition(v map[string]interface{}) *StatusCondition {
	pt, _ := time.Parse(time.RFC3339, getNestedString(v, "lastTransitionTime"))
	t := metav1.Time{Time: pt.Local()}

	return &StatusCondition{
		Name:               getNestedString(v, "name"),
		CurrentStatus:      CurrentStatus(getNestedString(v, "currentStatus")),
		LastTransitionTime: t,
	}
}

type Feedback interface {
	Initialize(groups []string) error
	SetCondition(name string, currentStatus CurrentStatus) error
}

type feedback struct {
	logger        log.Logger
	client        *client.Client
	oldStatus     *Status
	currentStatus *Status
	obj           *unstructured.Unstructured
}

func NewFeedback(logger log.Logger, client *client.Client, u *unstructured.Unstructured) Feedback {
	oldStatus := extractStatus(u)

	return &feedback{
		logger:    logger,
		client:    client,
		oldStatus: oldStatus,
		obj:       u,
	}
}

func (f *feedback) Initialize(groups []string) error {
	level.Debug(f.logger).Log("msg", "initializing status", "namespace", f.obj.GetNamespace(), "name", f.obj.GetName(), "kind", f.obj.GetKind(), "apiVersion", f.obj.GetAPIVersion())
	f.initializeStatus(groups)
	return f.updateStatus()
}

func (f *feedback) SetCondition(name string, currentStatus CurrentStatus) error {
	level.Debug(f.logger).Log("msg", "setting condition status", "namespace", f.obj.GetNamespace(), "name", f.obj.GetName(), "kind", f.obj.GetKind(), "apiVersion", f.obj.GetAPIVersion(), "condition", name, "status", currentStatus)
	for i, c := range f.currentStatus.Conditions {
		if c.Name == name {
			if c.CurrentStatus != currentStatus {
				f.currentStatus.Conditions[i] = &StatusCondition{
					Name:               name,
					CurrentStatus:      currentStatus,
					LastTransitionTime: metav1.Now(),
				}
			}
		}
	}

	return f.updateStatus()
}

func (f *feedback) updateStatus() error {
	if reflect.DeepEqual(f.oldStatus, f.currentStatus) {
		return nil
	}

	status := map[string]interface{}{
		"kind":       f.obj.GetKind(),
		"apiVersion": f.obj.GetAPIVersion(),
		"metadata": map[string]interface{}{
			"name":            f.obj.GetName(),
			"namespace":       f.obj.GetNamespace(),
			"resourceVersion": f.obj.GetResourceVersion(),
		},
		"status": f.currentStatus,
	}

	c, err := f.client.ClientForUnstructured(f.obj)
	if err != nil {
		return err
	}

	f.obj, err = c.UpdateStatus(context.TODO(), &unstructured.Unstructured{Object: status}, metav1.UpdateOptions{})
	return err
}

func (f *feedback) initializeStatus(groups []string) {
	f.currentStatus = &Status{}

	if f.oldStatus == nil || len(f.oldStatus.Conditions) == 0 {
		for _, g := range groups {
			f.currentStatus.Conditions = append(f.currentStatus.Conditions, &StatusCondition{
				LastTransitionTime: metav1.Now(),
				CurrentStatus:      StatusConditionNotStarted,
				Name:               g,
			})
		}

		return
	}

	availableOldConditions := map[string]*StatusCondition{}
	for _, oldCondition := range f.oldStatus.Conditions {
		if oldCondition != nil {
			availableOldConditions[oldCondition.Name] = oldCondition
		}
	}

	for _, g := range groups {
		if oldCondition, found := availableOldConditions[g]; found {
			f.currentStatus.Conditions = append(f.currentStatus.Conditions, oldCondition)
			continue
		}

		f.currentStatus.Conditions = append(f.currentStatus.Conditions, &StatusCondition{
			LastTransitionTime: metav1.Now(),
			CurrentStatus:      StatusConditionNotStarted,
			Name:               g,
		})
	}
}
