package checks

import (
	"context"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/db"
	"github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Checks struct {
	logger              log.Logger
	client              *client.Client
	databaseConnections *db.Connections
	knownChecks         map[string]Check
}

type CheckReport struct {
	CheckName string
	Message   string
}

func NewChecks(
	logger log.Logger,
	client *client.Client,
	databaseConnections *db.Connections,
	checks []Check,
) (*Checks, error) {
	knownChecks := map[string]Check{}
	for _, c := range checks {
		name := c.Name()
		if _, ok := knownChecks[name]; ok {
			return nil, errors.Errorf("duplicate check with name %q already registered", name)
		}
		knownChecks[c.Name()] = c
	}

	return &Checks{
		logger:              logger,
		client:              client,
		databaseConnections: databaseConnections,
		knownChecks:         knownChecks,
	}, nil
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
	sc, err := NewCheckRunner(
		c.logger,
		c.client,
		successDef,
		c.databaseConnections,
		c.knownChecks,
	)
	if err != nil {
		return err
	}
	return sc.Execute(ctx, u)
}
