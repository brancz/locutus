package checks

import (
	"context"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/db"
	"github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Checks struct {
	logger              log.Logger
	client              *client.Client
	databaseConnections *db.Connections
}

type CheckReport struct {
	CheckName string
	Message   string
}

func NewChecks(
	logger log.Logger,
	client *client.Client,
	databaseConnections *db.Connections,
) *Checks {
	return &Checks{
		logger:              logger,
		client:              client,
		databaseConnections: databaseConnections,
	}
}

func (c *Checks) RunChecks(
	ctx context.Context,
	successDefs []*types.SuccessDefinition,
	u *unstructured.Unstructured,
) error {
	for _, d := range successDefs {
		err := c.runCheck(ctx, d, u)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Checks) runCheck(
	ctx context.Context,
	successDef *types.SuccessDefinition,
	u *unstructured.Unstructured,
) error {
	sc, err := NewCheckRunner(c.logger, c.client, successDef, c.databaseConnections)
	if err != nil {
		return err
	}
	return sc.Execute(ctx, u)
}
