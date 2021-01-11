package checks

import (
	"context"
	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type SuccessChecks struct {
	logger log.Logger
	client *client.Client
}

type CheckReport struct {
	CheckName string
	Message   string
}

func NewSuccessChecks(logger log.Logger, client *client.Client) *SuccessChecks {
	return &SuccessChecks{
		logger: logger,
		client: client,
	}
}

func (c *SuccessChecks) RunChecks(ctx context.Context, successDefs []*types.SuccessDefinition, u *unstructured.Unstructured) error {
	for _, d := range successDefs {
		err := c.runCheck(ctx, d, u)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SuccessChecks) runCheck(ctx context.Context, successDef *types.SuccessDefinition, u *unstructured.Unstructured) error {
	if len(successDef.FieldComparisons) > 0 {
		c, err := NewFieldCheck(c.logger, c.client, successDef.FieldComparisons)
		if err != nil {
			return err
		}
		return c.Execute(ctx, u)
	}

	return nil
}
