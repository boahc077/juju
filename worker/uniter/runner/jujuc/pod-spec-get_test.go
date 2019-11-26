// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type PodSpecGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&PodSpecGetSuite{})

var podSpecGetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"extra"}, `unrecognized args: \["extra"\]`},
}

func (s *PodSpecGetSuite) TestPodSpecGetInit(c *gc.C) {
	for i, t := range podSpecGetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "pod-spec-get")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *PodSpecGetSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "pod-spec-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	expectedHelp := `
Usage: pod-spec-get

Summary:
get pod spec information

Details:
Gets configuration data used for a pod.
`[1:]

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *PodSpecGetSuite) TestPodSpecSet(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	hctx.info.PodSpec = "podspec"
	com, err := jujuc.NewCommand(hctx, "pod-spec-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "podspec")
}
