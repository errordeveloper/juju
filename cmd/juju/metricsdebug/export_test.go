// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"errors"
	"time"

	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

var (
	NewClient            = &newClient
	NewRunClient         = &newRunClient
	NewApplicationClient = &newApplicationClient
	NewAPIConn           = &newAPIConn
)

// NewRunClientFnc returns a function that returns a struct that implements the
// runClient interface. This function can be used to patch the NewRunClient
// variable in tests.
func NewRunClientFnc(client runClient) func(api.Connection) runClient {
	return func(_ api.Connection) runClient {
		return client
	}
}

// NewApplicationClientFnc returns a function that returns a struct that implements the
// applicationClient interface. This function can be used to patch the NewApplicationClient
// variable in tests.
func NewApplicationClientFnc(client applicationClient) func(api.Connection) applicationClient {
	return func(_ api.Connection) applicationClient {
		return client
	}
}

func PatchGetActionResult(patchValue func(interface{}, interface{}), actions map[string]params.ActionResult) {
	patchValue(&getActionResult, func(_ runClient, id string, _ *time.Timer) (params.ActionResult, error) {
		if res, ok := actions[id]; ok {
			return res, nil
		}
		return params.ActionResult{}, errors.New("plm")
	})
}

func NewCollectMetricsCommandForTest() cmd.Command {
	cmd := &collectMetricsCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}

func NewMetricsCommandForTest() cmd.Command {
	cmd := &MetricsCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}
