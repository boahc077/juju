// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/json"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ListControllersSuite struct {
	baseControllerSuite
	api func(string) controller.ControllerAccessAPI
}

var _ = gc.Suite(&ListControllersSuite{})

func (s *ListControllersSuite) TestListControllersEmptyStore(c *gc.C) {
	s.store = jujuclient.NewMemStore()
	_, err := s.runListControllers(c)
	c.Check(errors.Cause(err), gc.Equals, modelcmd.ErrNoControllersDefined)
}

func (s *ListControllersSuite) TestListControllers(c *gc.C) {
	store := s.createTestClientStore(c)
	delete(store.Accounts, "aws-test")
	originallyInStore := &jujuclient.ControllerModels{
		CurrentModel: "admin/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"model0":   {ModelUUID: "abc", ModelType: model.IAAS},
			"my-model": {ModelUUID: "def", ModelType: model.IAAS},
		},
	}
	c.Assert(store.Models["mallards"], gc.DeepEquals, originallyInStore)

	s.expectedOutput = `
Use --refresh flag with this command to see the latest information.

Controller           Model             User   Access     Cloud/Region        Models  Machines  HA  Version
aws-test             admin/controller  -      -          aws/us-east-1            1         5   -  2.0.1      
mallards*            my-model          admin  superuser  mallards/mallards1       2         -   -  (unknown)  
mark-test-prodstack  -                 admin  (unknown)  prodstack                -         -   -  (unknown)  

`[1:]

	s.assertListControllers(c)
	// Check store was not updated.
	c.Assert(store.Models["mallards"], gc.DeepEquals, originallyInStore)
}

func (s *ListControllersSuite) TestListControllersRefresh(c *gc.C) {
	store := s.createTestClientStore(c)
	originallyInStore := &jujuclient.ControllerModels{
		CurrentModel: "admin/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"model0":   {ModelUUID: "abc", ModelType: model.IAAS},
			"my-model": {ModelUUID: "def", ModelType: model.IAAS},
		},
	}
	c.Assert(store.Models["mallards"], gc.DeepEquals, originallyInStore)
	s.api = func(controllerName string) controller.ControllerAccessAPI {
		fakeController := &fakeController{controllerName: controllerName}
		return fakeController
	}
	s.expectedOutput = `
Controller           Model       User   Access     Cloud/Region        Models  Machines  HA  Version
aws-test             controller  admin  (unknown)  aws/us-east-1            1         2   -  2.0.1      
mallards*            my-model    admin  superuser  mallards/mallards1       2         4   -  (unknown)  
mark-test-prodstack  -           admin  (unknown)  prodstack                -         -   -  (unknown)  

`[1:]
	s.assertListControllers(c, "--refresh")
	// Check store was updated.
	c.Assert(store.Models["mallards"], gc.DeepEquals, &jujuclient.ControllerModels{
		CurrentModel: "admin/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {ModelUUID: "abc", ModelType: model.IAAS},
			"admin/my-model":   {ModelUUID: "def", ModelType: model.IAAS},
		},
	})
}

func (s *ListControllersSuite) setupAPIForControllerMachines() {
	s.api = func(controllerName string) controller.ControllerAccessAPI {
		fakeController := &fakeController{controllerName: controllerName}
		switch controllerName {
		case "aws-test":
			fakeController.machines = map[string][]base.Machine{
				"ghi": {
					{Id: "1", HasVote: true, WantsVote: true, Status: "active"},
					{Id: "2", HasVote: true, WantsVote: true, Status: "down"},
					{Id: "3", HasVote: false, WantsVote: true, Status: "active"},
				},
			}
		case "mallards":
			fakeController.machines = map[string][]base.Machine{
				"abc": {
					{Id: "1", HasVote: true, WantsVote: true, Status: "active"},
				},
				"def": {
					{Id: "1", HasVote: true, WantsVote: true, Status: "active"},
				},
			}
		}
		return fakeController
	}
}

func (s *ListControllersSuite) TestListControllersKnownHAStatus(c *gc.C) {
	s.createTestClientStore(c)
	s.setupAPIForControllerMachines()
	s.expectedOutput = `
Controller           Model       User   Access     Cloud/Region        Models  Machines    HA  Version
aws-test             controller  admin  (unknown)  aws/us-east-1            1         2   1/3  2.0.1      
mallards*            my-model    admin  superuser  mallards/mallards1       2         4  none  (unknown)  
mark-test-prodstack  -           admin  (unknown)  prodstack                -         -     -  (unknown)  

`[1:]
	s.assertListControllers(c, "--refresh")
}

func (s *ListControllersSuite) TestListControllersYaml(c *gc.C) {
	s.expectedOutput = `
controllers:
  aws-test:
    current-model: controller
    user: admin
    recent-server: this-is-aws-test-of-many-api-endpoints
    controller-uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
    cloud: aws
    region: us-east-1
    agent-version: 2.0.1
    model-count: 1
    machine-count: 2
    controller-machines:
      active: 1
      total: 3
  mallards:
    current-model: my-model
    user: admin
    access: superuser
    recent-server: this-is-another-of-many-api-endpoints
    controller-uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
    cloud: mallards
    region: mallards1
    model-count: 2
    machine-count: 4
    controller-machines:
      active: 1
      total: 1
  mark-test-prodstack:
    user: admin
    recent-server: this-is-one-of-many-api-endpoints
    controller-uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
    cloud: prodstack
current-controller: mallards
`[1:]

	s.createTestClientStore(c)
	s.setupAPIForControllerMachines()
	s.assertListControllers(c, "--format", "yaml", "--refresh")
}

func intPtr(i int) *int {
	return &i
}

func (s *ListControllersSuite) TestListControllersJson(c *gc.C) {
	s.expectedOutput = ""
	s.createTestClientStore(c)
	jsonOut := s.assertListControllers(c, "--format", "json")
	var result controller.ControllerSet
	err := json.Unmarshal([]byte(jsonOut), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, controller.ControllerSet{
		Controllers: map[string]controller.ControllerItem{
			"aws-test": {
				ControllerUUID: "this-is-the-aws-test-uuid",
				ModelName:      "controller",
				User:           "admin",
				Server:         "this-is-aws-test-of-many-api-endpoints",
				APIEndpoints:   []string{"this-is-aws-test-of-many-api-endpoints"},
				CACert:         "this-is-aws-test-ca-cert",
				Cloud:          "aws",
				CloudRegion:    "us-east-1",
				AgentVersion:   "2.0.1",
				ModelCount:     intPtr(1),
				MachineCount:   intPtr(5),
			},
			"mallards": {
				ControllerUUID: "this-is-another-uuid",
				ModelName:      "my-model",
				User:           "admin",
				Access:         "superuser",
				Server:         "this-is-another-of-many-api-endpoints",
				APIEndpoints:   []string{"this-is-another-of-many-api-endpoints", "this-is-one-more-of-many-api-endpoints"},
				CACert:         "this-is-another-ca-cert",
				Cloud:          "mallards",
				CloudRegion:    "mallards1",
				ModelCount:     intPtr(2),
			},
			"mark-test-prodstack": {
				ControllerUUID: "this-is-a-uuid",
				User:           "admin",
				Server:         "this-is-one-of-many-api-endpoints",
				APIEndpoints:   []string{"this-is-one-of-many-api-endpoints"},
				CACert:         "this-is-a-ca-cert",
				Cloud:          "prodstack",
			},
		},
		CurrentController: "mallards",
	})
}

func (s *ListControllersSuite) TestListControllersReadFromStoreErr(c *gc.C) {
	msg := "fail getting all controllers"
	errStore := jujuclienttesting.NewStubStore()
	errStore.SetErrors(errors.New(msg))
	s.store = errStore
	s.expectedErr = fmt.Sprintf("failed to list controllers: %v", msg)
	s.assertListControllersFailed(c)
	errStore.CheckCallNames(c, "AllControllers")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedArg(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `unrecognized args: \["whoops"\]`
	s.assertListControllersFailed(c, "whoops")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedFlag(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `flag provided but not defined: -m`
	s.assertListControllersFailed(c, "-m", "my.world")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedOptionFlag(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `flag provided but not defined: --model`
	s.assertListControllersFailed(c, "--model", "still.my.world")
}

func (s *ListControllersSuite) TestListControllersNoControllers(c *gc.C) {
	store := s.createTestClientStore(c)
	store.Controllers = map[string]jujuclient.ControllerDetails{}
	s.expectedErr = `No controllers registered.

Please either create a new controller using "juju bootstrap" or connect to
another controller that you have been given access to using "juju register".
`
	s.assertListControllersFailed(c)
}

func (s *ListControllersSuite) runListControllers(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, controller.NewListControllersCommandForTest(s.store, s.api), args...)
}

func (s *ListControllersSuite) assertListControllersFailed(c *gc.C, args ...string) {
	_, err := s.runListControllers(c, args...)
	c.Assert(err, gc.ErrorMatches, s.expectedErr)
}

func (s *ListControllersSuite) assertListControllers(c *gc.C, args ...string) string {
	context, err := s.runListControllers(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	if s.expectedOutput != "" {
		c.Assert(output, gc.Equals, s.expectedOutput)
	}
	return output
}
