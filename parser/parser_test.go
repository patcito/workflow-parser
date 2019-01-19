package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEmptyConfig(t *testing.T) {
	workflow, err := parseString("")
	assertParseSuccess(t, err, 0, 0, workflow)
	workflow, err = parseString("{}")
	assertParseSuccess(t, err, 0, 0, workflow)
}

func TestActionsAndAttributes(t *testing.T) {
	workflow, err := parseString(`
		"action" "a" {
			"uses"="./x"
			runs="cmd"
			env={ PATH="less traveled by", "HOME"="where the heart is" }
		}
		action "b" {
			uses="./y"
			needs=["a"]
			args=["foo", "bar"]
			secrets=[ "THE", "CURRENCY", "OF", "INTIMACY" ]
		}`)
	assertParseSuccess(t, err, 2, 0, workflow)
	assert.Equal(t, 0, workflow.Version)

	actionA := workflow.Actions[0]
	assert.Equal(t, "a", actionA.Identifier)
	assert.Equal(t, 0, len(actionA.Needs))
	assert.Equal(t, ActionUses{Path: "./x", Raw: "./x"}, actionA.Uses)
	assert.Equal(t, "cmd", actionA.Runs.Raw)
	assert.Equal(t, []string{"cmd"}, actionA.Runs.Parsed)
	assert.Equal(t, "", actionA.Args.Raw)
	assert.Equal(t, map[string]string{"PATH": "less traveled by", "HOME": "where the heart is"}, actionA.Env)

	actionB := workflow.Actions[1]
	assert.Equal(t, "b", actionB.Identifier)
	assert.Equal(t, ActionUses{Path: "./y", Raw: "./y"}, actionB.Uses)
	assert.Equal(t, []string{"a"}, actionB.Needs)
	assert.Equal(t, "", actionB.Runs.Raw)
	assert.Equal(t, "", actionB.Args.Raw)
	assert.Equal(t, []string{"foo", "bar"}, actionB.Args.Parsed)
	assert.Equal(t, []string{"THE", "CURRENCY", "OF", "INTIMACY"}, actionB.Secrets)
}

func TestStringEscaping(t *testing.T) {
	workflow, err := parseString(`
		action "a" {
			uses="./x \" y \\ z"
		}`)
	assertParseSuccess(t, err, 1, 0, workflow)
	assert.Equal(t, `./x " y \ z`, workflow.Actions[0].Uses.Raw)
}

func TestFileVersion0(t *testing.T) {
	workflow, err := parseString(`"version"=0 action "a" { uses="./foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow)
	assert.Equal(t, 0, workflow.Version)
}

func TestFileVersion42(t *testing.T) {
	workflow, err := parseString(`version=42 action "a" { uses="./foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "`version = 42` is not supported")
}

func TestFileVersionMustComeFirst(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./foo" } version=0`)
	assertParseSuccess(t, err, 1, 0, workflow, "`version` must be the first declaration")
}

/*
// TODO: enable this once const substitution is defined and implemented
func TestUsesIsAVariable(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="${value}" command="foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow)
}
*/

func TestUnscopedVariableNames(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" runs="${value}" }`)
	assertParseSuccess(t, err, 1, 0, workflow)
}

func TestActionCollision(t *testing.T) {
	workflow, err := parseString(`
		action "a" { uses="./x" }
		action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 2, 0, workflow, "identifier `a' redefined")
}

func TestBadHCL(t *testing.T) {
	workflow, err := parseString(`this is definitely not valid HCL!`)
	assertParseSuccess(t, err, 0, 0, workflow, "illegal char")
	workflow, err = parseString(`action "foo"`)
	assertParseSuccess(t, err, 0, 0, workflow, "expected start of object ('{') or assignment ('=')")
	workflow, err = parseString(`action "foo" {`)
	assertParseSuccess(t, err, 0, 0, workflow, "object expected closing rbrace got: eof")
	workflow, err = parseString(`action "foo" { uses=" }`)
	assertParseSuccess(t, err, 0, 0, workflow, "literal not terminated")
	workflow, err = parseString(`action "foo" { uses=""" }`)
	assertParseSuccess(t, err, 0, 0, workflow, "literal not terminated")
}

func TestCircularDependencySelf(t *testing.T) {
	workflow, err := parseString(`
		action "a" {
			uses="./x"
			needs=["a"]
		}`)
	assertParseSuccess(t, err, 1, 0, workflow, "circular dependency")
}

func TestCircularDependencyOther(t *testing.T) {
	workflow, err := parseString(`
		// simple cycle: a -> b -> a
		action "a" { uses="./x" needs=["b", "g"] }
		action "b" { uses="./x" needs=["a", "f"] }

		// three-node cycle with unrelated lead-in: z -> c -> e -> d -> c
		action "z" { uses="./x" needs="c" }
		action "c" { uses="./x" needs=["e"] }
		action "d" { uses="./x" needs="c" }
		action "e" { uses="./x" needs=["d"] }

		// two-hop cycle overlapping the first one: b -> f -> b
		action "f" { uses="./x" needs="b" }

		// two-hop cycle overlapping the first one: a -> g -> a
		action "g" { uses="./x" needs=["a", "i"] }

		// one-hop (self) cycle: h -> h
		action "h" { uses="./x" needs="h" }

		// cycle that reuses a reported edge: a -> g -> i -> a
		action "i" { uses="./x" needs="a" }
	`)

	// Each unique cycle should be reported exactly once, at the first point
	// (reading top to bottom, left to right) that the cycle is apparent to
	// the parser.
	assertParseSuccess(t, err, 10, 0, workflow,
		"line 4: circular dependency on `a'",
		"line 9: circular dependency on `c'",
		"line 13: circular dependency on `b'",
		"line 16: circular dependency on `a'",
		"line 19: circular dependency on `h'",
		"line 22: circular dependency on `a'")
}

func TestFlowMapping(t *testing.T) {
	workflow, err := parseString(`"workflow" "foo" { "on" = "push" resolves = ["a", "b"] } action "a" { uses="./x" } action "b" { uses="./y" }`)
	assertParseSuccess(t, err, 2, 1, workflow)
	assert.Equal(t, 0, workflow.Version)
	assert.Equal(t, "push", workflow.Workflows[0].On)
	assert.ElementsMatch(t, []string{"a", "b"}, workflow.Workflows[0].Resolves)
}

func TestFlowOneResolve(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow)
	assert.Equal(t, 0, workflow.Version)
	assert.Equal(t, "push", workflow.Workflows[0].On)
	assert.Len(t, workflow.Workflows[0].Resolves[0], 1)
	assert.Equal(t, "a", workflow.Workflows[0].Resolves[0])
}

func TestFlowNoResolves(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push"}`)
	assertParseSuccess(t, err, 0, 1, workflow)
	assert.Equal(t, 0, workflow.Version)
	assert.Equal(t, "push", workflow.Workflows[0].On)
	assert.Len(t, workflow.Workflows[0].Resolves, 0)
	assert.Empty(t, workflow.Workflows[0].Resolves)
}

func TestUses(t *testing.T) {
	workflow, err := parseString(`
		action "a" { uses="foo/bar@dev" }
		action "b" { uses="foo/bar/path@1.0.0" }
		action "c" { uses="./xyz" }
		action "d" { uses="docker://alpine" }
	`)
	assertParseSuccess(t, err, 4, 0, workflow)
	a := workflow.GetAction("a")
	if assert.NotNil(t, a) {
		assert.Equal(t, ActionUses{Repo: "foo/bar", Path: "/", Ref: "dev", Raw: "foo/bar@dev"}, a.Uses)
	}
	b := workflow.GetAction("b")
	if assert.NotNil(t, b) {
		assert.Equal(t, ActionUses{Repo: "foo/bar", Path: "/path", Ref: "1.0.0", Raw: "foo/bar/path@1.0.0"}, b.Uses)
	}
	c := workflow.GetAction("c")
	if assert.NotNil(t, c) {
		assert.Equal(t, ActionUses{Path: "./xyz", Raw: "./xyz"}, c.Uses)
	}
	d := workflow.GetAction("d")
	if assert.NotNil(t, d) {
		assert.Equal(t, ActionUses{Image: "alpine", Raw: "docker://alpine"}, d.Uses)
	}
}

func TestUsesFailures(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "the `uses' attribute must be a path, a docker image, or owner/repo@ref")
	workflow, err = parseString(`action "a" { uses="foo/bar" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "the `uses' attribute must be a path, a docker image, or owner/repo@ref")
	workflow, err = parseString(`action "a" { uses="foo@bar" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "the `uses' attribute must be a path, a docker image, or owner/repo@ref")
	workflow, err = parseString(`action "a" { uses={a="b"} }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got object",
		"action `a' must have a `uses' attribute")
	workflow, err = parseString(`action "a" { uses=["x"] }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got list",
		"action `a' must have a `uses' attribute")
	workflow, err = parseString(`action "a" { uses=42 }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got number",
		"action `a' must have a `uses' attribute")
}

func TestGetCommand(t *testing.T) {
	workflow, err := parseString(`
		action "a" { uses="./x" runs="a b c" }
		action "b" { uses="./x" runs=["a", "b", "c"] }
		action "c" { uses="./x" args="a b c" }
		action "d" { uses="./x" args=["a", "b", "c"] }
		action "e" { uses="./x" runs="a b c" args="x y z" }
		action "f" { uses="./x" runs=["a", "b", "c"] args=["x", "y", "z"] }
	`)
	assertParseSuccess(t, err, 6, 0, workflow)
	a := workflow.GetAction("a")
	assert.NotNil(t, a)
	assert.Equal(t, ActionCommand{Parsed: []string{"a", "b", "c"}, Raw: "a b c"}, a.Runs)
	b := workflow.GetAction("b")
	assert.NotNil(t, b)
	assert.Equal(t, ActionCommand{Parsed: []string{"a", "b", "c"}}, b.Runs)
	c := workflow.GetAction("c")
	assert.NotNil(t, c)
	assert.Equal(t, ActionCommand{Parsed: []string{"a", "b", "c"}, Raw: "a b c"}, c.Args)
	d := workflow.GetAction("d")
	assert.NotNil(t, d)
	assert.Equal(t, ActionCommand{Parsed: []string{"a", "b", "c"}}, d.Args)
	e := workflow.GetAction("e")
	assert.NotNil(t, e)
	assert.Equal(t, ActionCommand{Parsed: []string{"a", "b", "c"}, Raw: "a b c"}, e.Runs)
	assert.Equal(t, ActionCommand{Parsed: []string{"x", "y", "z"}, Raw: "x y z"}, e.Args)
	f := workflow.GetAction("f")
	assert.NotNil(t, f)
	assert.Equal(t, ActionCommand{Parsed: []string{"a", "b", "c"}}, f.Runs)
	assert.Equal(t, ActionCommand{Parsed: []string{"x", "y", "z"}}, f.Args)
}

func TestGetCommandFailure(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" runs=42 }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got number",
		"the `runs' attribute must be a string or a list")
	workflow, err = parseString(`action "a" { uses="./x" runs={} }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got object",
		"the `runs' attribute must be a string or a list")
	workflow, err = parseString(`action "a" { uses="./x" runs="" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "`runs' value in action `a' cannot be blank")

	workflow, err = parseString(`action "a" { uses="./x" args=42 }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got number",
		"the `args' attribute must be a string or a list")
	workflow, err = parseString(`action "a" { uses="./x" args={} }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"expected string, got object",
		"the `args' attribute must be a string or a list")
	workflow, err = parseString(`action "a" { uses="./x" args="" }`)
	assertParseSuccess(t, err, 1, 0, workflow)
}

func TestBadEnv(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" env=[] }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected object, got list")
	workflow, err = parseString(`action "a" { uses="./x" env="foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected object, got string")
	workflow, err = parseString(`action "a" { uses="./x" env=42 }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected object, got number")
	workflow, err = parseString(`action "a" { uses="./x" env=12.34 }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected object, got float")
	workflow, err = parseString(`
		action "a" {
			uses="./x"
			env={
				"x"="foo"
				"^"="bar"
				a_="baz"
			}
		}
		action "b" {
			uses="./y"
			env={
				a.="qux"
			}
		}
	`)
	assertParseSuccess(t, err, 2, 0, workflow,
		"line 4: environment variables and secrets must contain only a-z, a-z, 0-9, and _ characters, got `^'",
		"line 12: environment variables and secrets must contain only a-z, a-z, 0-9, and _ characters, got `a.'")
	assert.Equal(t, 3, len(workflow.Actions[0].Env))
	assert.Equal(t, "bar", workflow.Actions[0].Env["^"])

	workflow, err = parseString(`action "a" { uses="./x" env={x="foo" x="bar"} }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"line 1: environment variable `x' redefined")
	assert.Equal(t, map[string]string{"x": "bar"}, workflow.Actions[0].Env)
}

func TestBadSecrets(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" secrets={} }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected list, got object")
	workflow, err = parseString(`action "a" { uses="./x" secrets="foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected list, got string")
	workflow, err = parseString(`action "a" { uses="./x" secrets=42 }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected list, got number")
	workflow, err = parseString(`action "a" { uses="./x" secrets=[ "-", "^", "9", "a", "0_o", "o_0" ] }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"line 1: environment variables and secrets must contain only a-z, a-z, 0-9, and _ characters, got `-'",
		"line 1: environment variables and secrets must contain only a-z, a-z, 0-9, and _ characters, got `^'",
		"line 1: environment variables and secrets must contain only a-z, a-z, 0-9, and _ characters, got `9'",
		"line 1: environment variables and secrets must contain only a-z, a-z, 0-9, and _ characters, got `0_o'")
	assert.Equal(t, []string{"-", "^", "9", "a", "0_o", "o_0"}, workflow.Actions[0].Secrets)

	workflow, err = parseString(`action "a" { uses="./x" env={x="foo"} secrets=["x"] }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"line 1: secret `x' conflicts with an environment variable with the same name")
	assert.Equal(t, map[string]string{"x": "foo"}, workflow.Actions[0].Env)
	assert.Equal(t, []string{"x"}, workflow.Actions[0].Secrets)

	workflow, err = parseString(`action "a" { uses="./x" secrets=["x", "y", "x"] }`)
	assertParseSuccess(t, err, 1, 0, workflow, "line 1: secret `x' redefined")
	assert.Equal(t, []string{"x", "y", "x"}, workflow.Actions[0].Secrets)
}

func TestUsesCustomActionsTransformed(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./foo" }`)
	assertParseSuccess(t, err, 1, 0, workflow)
	action := workflow.GetAction("a")
	require.NotNil(t, action)
	assert.Equal(t, "./foo", action.Uses.Path)
}

func TestUsesCustomActionsShortPath(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./" }`)
	assertParseSuccess(t, err, 1, 0, workflow)
	action := workflow.GetAction("a")
	require.NotNil(t, action)
	assert.Equal(t, "./", action.Uses.Path)
}

func TestTwoFlows(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = "a" } workflow "bar" { on = "push" resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 2, workflow)
	assert.Equal(t, 0, workflow.Version)
	assert.Equal(t, "push", workflow.Workflows[0].On)
	assert.Len(t, workflow.Workflows[0].Resolves[0], 1)
	assert.Equal(t, "a", workflow.Workflows[0].Resolves[0])
	assert.Equal(t, "push", workflow.Workflows[1].On)
	assert.Len(t, workflow.Workflows[1].Resolves[0], 1)
	assert.Equal(t, "a", workflow.Workflows[1].Resolves[0])
}

func TestOnPush(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow)
	onValue := workflow.Workflows[0].On
	assert.Equal(t, "push", onValue)
}

func TestOnPullRequest(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "pull_request" resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow)
	onValue := workflow.Workflows[0].On
	assert.Equal(t, "pull_request", onValue)
}

func TestResolves(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow)
	resolveValues := workflow.Workflows[0].Resolves
	assert.Equal(t, []string{"a"}, resolveValues)
}

func TestMultipleResolves(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = ["a","b"] } action "a" { uses="./x" } action "b" { uses="./y" }`)
	assertParseSuccess(t, err, 2, 1, workflow)
	resolveValues := workflow.Workflows[0].Resolves
	assert.Equal(t, []string{"a", "b"}, resolveValues)
	assert.Len(t, resolveValues, 2)
}

func TestNeeds(t *testing.T) {
	workflow, err := parseString(`
		action "a" { uses="./w" needs="b" }
		action "b" { uses="./x" needs=["c", "d"] }
		action "c" { uses="./y" }
		action "d" { uses="./y" }
	`)
	assertParseSuccess(t, err, 4, 0, workflow)
	needsValues := workflow.Actions[0].Needs
	assert.Equal(t, []string{"b"}, needsValues)
	needsValues = workflow.Actions[1].Needs
	assert.Equal(t, []string{"c", "d"}, needsValues)
	needsValues = workflow.Actions[2].Needs
	assert.Equal(t, 0, len(needsValues))
}

func TestGetWorkflows(t *testing.T) {
	cfg, err := parseString(`workflow "foo" { on = "push" resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, cfg)
	workflows := cfg.GetWorkflows("push")
	require.Equal(t, 1, len(workflows))
	assert.Equal(t, "foo", workflows[0].Identifier)
	workflows = cfg.GetWorkflows("blah")
	require.Equal(t, 0, len(workflows))
}

func TestFlowMissingOn(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow, "workflow `foo' must have an `on' attribute")
}

func TestFlowOnTypeError(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = 42 resolves = "a" } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow,
		"expected string, got number",
		"invalid format for `on' in workflow `foo'",
		"workflow `foo' must have an `on' attribute")
}

func TestFlowOnUnexpectedValue(t *testing.T) {
	workflow, err := parseString(`
		workflow "foo" {
			on = "hsup"
			resolves = "a"
			on = 42
		}
		action "a" {
			uses="./x"
		}`)
	assertParseSuccess(t, err, 1, 1, workflow,
		"line 3: workflow `foo' has unknown `on' value `hsup'",
		"line 5: `on' redefined in workflow `foo'",
		"line 5: expected string, got number",
		"line 5: invalid format for `on' in workflow `foo', expected string")
	assert.Equal(t, "hsup", workflow.Workflows[0].On)
}

func TestFlowResolvesTypeError(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = 42 } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow,
		"expected list, got number",
		"invalid format for `resolves' in workflow `foo', expected list of strings")
}

func TestFlowMissingAction(t *testing.T) {
	workflow, err := parseString(`workflow "foo" { on = "push" resolves = ["a", "b"] } action "a" { uses="./x" }`)
	assertParseSuccess(t, err, 1, 1, workflow, "workflow `foo' resolves unknown action `b'")
}

func TestUsesMissingCheck(t *testing.T) {
	workflow, err := parseString(`action "a" { }`)
	assertParseSuccess(t, err, 1, 0, workflow, "action `a' must have a `uses' attribute")
}

func TestUsesAttributeBlankCheck(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="" }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"`uses' value in action `a' cannot be blank",
		"action `a' must have a `uses' attribute")
}

func TestUsesDuplicatesCheck(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" uses="./y" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "`uses' redefined in action `a'")
}

func TestCommandDuplicatesCheck(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" runs="x" runs="y" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "`runs' redefined in action `a'")
	workflow, err = parseString(`action "a" { uses="./x" args="x" args="y" }`)
	assertParseSuccess(t, err, 1, 0, workflow, "`args' redefined in action `a'")
}

func TestFlowKeywordsRedefined(t *testing.T) {
	workflow, err := parseString(`workflow "a" { on="push" on="push" resolves=["c"] }`)
	assertParseSuccess(t, err, 0, 1, workflow,
		"`on' redefined in workflow `a'",
		"resolves unknown action `c'")
	workflow, err = parseString(`workflow "a" { on="push" resolves=["b"] resolves=["c"] }`)
	assertParseSuccess(t, err, 0, 1, workflow,
		"`resolves' redefined in workflow `a'",
		"resolves unknown action `c'")
}

func TestNonExistentExplicitDependency(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" needs=["b"] }`)
	assertParseSuccess(t, err, 1, 0, workflow, "action `a' needs nonexistent action `b'")
}

func TestBadDependenciesList(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./x" needs=42 }`)
	assertParseSuccess(t, err, 1, 0, workflow, "expected list, got number")
}

func TestActionExtraKeywords(t *testing.T) {
	workflow, err := parseString(`action "a" "b" { }`)
	assertParseSuccess(t, err, 0, 0, workflow, "invalid toplevel declaration")
}

func TestInvalidKeyword(t *testing.T) {
	workflow, err := parseString(`hello "a" { }`)
	assertParseSuccess(t, err, 0, 0, workflow, "invalid toplevel keyword")
}

func TestInvalidActionIdentifier(t *testing.T) {
	workflow, err := parseString(`action "" { }`)
	assertParseSuccess(t, err, 0, 0, workflow, "invalid format for identifier")
}

func TestInvalidAttribute(t *testing.T) {
	workflow, err := parseString(`action "a" { uses { } }`)
	assertParseSuccess(t, err, 1, 0, workflow,
		"each attribute of action `a' must be an assignment",
		"expected string, got object",
		"action `a' must have a `uses' attribute")
}

func TestContinueAfterBadAssignment(t *testing.T) {
	workflow, err := parseString(`action "a" { uses { } } action "b" { uses="./foo" }`)
	assertParseSuccess(t, err, 2, 0, workflow,
		"each attribute of action `a' must be an assignment",
		"expected string, got object",
		"action `a' must have a `uses' attribute")
	require.NotNil(t, workflow)
	require.Equal(t, 2, len(workflow.Actions))
	assert.Equal(t, "a", workflow.Actions[0].Identifier)
	assert.Equal(t, "b", workflow.Actions[1].Identifier)
}

func TestTooManySecrets(t *testing.T) {
	workflow, err := parseString(`
		action "a" { uses="./a" secrets=["A", "B", "C", "D", "E", "F", "G", "H", "I", "J"] }
	`)
	assertParseSuccess(t, err, 1, 0, workflow)
	require.NotNil(t, workflow)
	assert.Equal(t, 10, len(workflow.Actions[0].Secrets))

	workflow, err = parseString(`
		action "a" { uses="./a" secrets=["A", "B", "C", "D", "E"] }
		action "b" { uses="./b" secrets=["D", "E", "F", "G", "H", "I", "J"] }
	`)
	assertParseSuccess(t, err, 2, 0, workflow)
	require.NotNil(t, workflow)
	assert.Equal(t, 5, len(workflow.Actions[0].Secrets))
	assert.Equal(t, 7, len(workflow.Actions[1].Secrets))

	workflow, err = parseString(`
		action "a" { uses="./a" secrets=["S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11", "S12", "S13", "S14", "S15", "S16", "S17", "S18", "S19", "S20", "S21", "S22", "S23", "S24", "S25", "S26", "S27", "S28", "S29", "S30", "S31", "S32", "S33", "S34", "S35", "S36", "S37", "S38", "S39", "S40"] }
		action "b" { uses="./b" secrets=["S35", "S36", "S37", "S38", "S39", "S40", "S41", "S42", "S43", "S44", "S45", "S46", "S47", "S48", "S49", "S50", "S51", "S52", "S53", "S54", "S55", "S56", "S57", "S58", "S59", "S60", "S61", "S62", "S63", "S64", "S65", "S66", "S67", "S68", "S69", "S70", "S71", "S72", "S73", "S74", "S75", "S76", "S77", "S78", "S79", "S80", "S81", "S82", "S83", "S84", "S85", "S86", "S87", "S88", "S89", "S90", "S91", "S92", "S93", "S94", "S95", "S96", "S97", "S98", "S99", "S100", "S101"] }
		action "c" { uses="./b" secrets=["S90", "S91", "S92", "S93", "S94", "S95", "S96", "S97", "S98", "S99", "S100", "S101", "S102", "S103", "S104", "S105", "S106", "S107", "S108", "S109", "S110"] }
	`)
	assertParseSuccess(t, err, 3, 0, workflow, "all actions combined must not have more than 100 unique secrets")
}

func TestUnknownAttributes(t *testing.T) {
	workflow, err := parseString(`action "a" { uses="./a" foo="1" } workflow "b" { on="push" bar="2" }`)
	assertParseSuccess(t, err, 1, 1, workflow,
		"unknown action attribute `foo'",
		"unknown workflow attribute `bar'")
}

func TestReservedVariables(t *testing.T) {
	workflow, err := parseString(`
		action "a" {
			uses="./a"
			env={
				GITHUB_FOO="nope"
				GITHUB_TOKEN="yup"
			}
		}
		action "b" {
			uses="./b"
			secrets = [
				"GITHUB_BAR",
				"GITHUB_TOKEN"
			]
		}
	`)
	assertParseSuccess(t, err, 2, 0, workflow,
		// the `env=` line in `a`
		"line 4: environment variables and secrets beginning with `github_' are reserved",
		// the `secrets=` line in `b`
		"line 11: environment variables and secrets beginning with `github_' are reserved")
	assert.Equal(t, "nope", workflow.Actions[0].Env["GITHUB_FOO"])
	assert.Equal(t, "yup", workflow.Actions[0].Env["GITHUB_TOKEN"])
	assert.Equal(t, []string{"GITHUB_BAR", "GITHUB_TOKEN"}, workflow.Actions[1].Secrets)
}

/********** helpers **********/

func assertParseSuccess(t *testing.T, err error, nactions int, nflows int, workflow *Configuration, errors ...string) {
	assert.NoError(t, err)
	require.NotNil(t, workflow)

	for _, e := range workflow.Errors {
		t.Log(e)
		assert.NotEqual(t, 0, e.Pos.Line, "error position not set")
	}
	assert.Equal(t, len(errors), len(workflow.Errors), "errors")
	for i := range errors {
		if i >= len(workflow.Errors) {
			break
		}
		assert.Contains(t, strings.ToLower(workflow.Errors[i].Error()), errors[i])
	}

	assert.Equal(t, nactions, len(workflow.Actions), "actions")
	assert.Equal(t, nflows, len(workflow.Workflows), "workflows")
}

func parseString(launchFile string) (*Configuration, error) {
	return Parse(strings.NewReader(launchFile))
}