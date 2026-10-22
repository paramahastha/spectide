package adapters_test

import (
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/adapters/claude"
	"github.com/spectide/spectide/internal/adapters/codex"
	"github.com/spectide/spectide/internal/adapters/opencode"
	"strings"
	"testing"
)

func TestBuildersDoNotContainBypassFlags(t *testing.T) {
	cases := []adapters.Adapter{claude.Adapter{}, codex.Adapter{}, opencode.Adapter{}}
	for _, a := range cases {
		in, e := a.Build(adapters.Request{ProjectRoot: "/tmp/work", Prompt: "test", Profile: adapters.Profile{Executable: string(a.Name())}})
		if e != nil && a.Name() == adapters.Claude {
			continue
		}
		for _, arg := range in.Args {
			if arg == "--dangerously-bypass-approvals-and-sandbox" {
				t.Fatal("unsafe flag")
			}
		}
	}
}

func TestProviderNormalizersAcceptJSONLAndDiscardRawPayload(t *testing.T) {
	cases := []adapters.Adapter{claude.Adapter{}, codex.Adapter{}, opencode.Adapter{}}
	for _, a := range cases {
		events, err := a.Normalize([]byte("{\"type\":\"system\",\"message\":\"private body\"}\n{\"type\":\"assistant\"}\n"))
		if err != nil {
			t.Fatalf("%s: %v", a.Name(), err)
		}
		if len(events) != 2 {
			t.Fatalf("%s events=%d", a.Name(), len(events))
		}
		if events[0].Raw != nil || strings.Contains(events[0].Summary, "private body") {
			t.Fatalf("%s retained provider content", a.Name())
		}
	}
}

func TestProviderNormalizerRejectsMalformedAndOversizedLines(t *testing.T) {
	if _, err := (codex.Adapter{}).Normalize([]byte("{broken}\n")); err == nil {
		t.Fatal("malformed event accepted")
	}
	line := strings.Repeat("x", (1<<20)+1)
	if _, err := (opencode.Adapter{}).Normalize([]byte(line)); err == nil {
		t.Fatal("oversized event accepted")
	}
}

func TestReadOnlyAndEffortPoliciesFailClosed(t *testing.T) {
	profile := adapters.Profile{Executable: "tool"}
	claudeInvocation, err := (claude.Adapter{}).Build(adapters.Request{Profile: profile, ProjectRoot: "/tmp/work", Prompt: "inspect", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	foundPlan := false
	for i, arg := range claudeInvocation.Args {
		if arg == "--permission-mode" && i+1 < len(claudeInvocation.Args) && claudeInvocation.Args[i+1] == "plan" {
			foundPlan = true
		}
	}
	if !foundPlan {
		t.Fatal("Claude analyze route lacks plan permission mode")
	}
	if _, err = (claude.Adapter{}).Build(adapters.Request{Profile: profile, ProjectRoot: "/tmp/work", Prompt: "inspect", Effort: "high"}); err == nil {
		t.Fatal("unverified Claude effort option was accepted")
	}
	if _, err = (opencode.Adapter{}).Build(adapters.Request{Profile: profile, ProjectRoot: "/tmp/work", Prompt: "inspect", ReadOnly: true}); err == nil {
		t.Fatal("OpenCode read-only policy was accepted without a verified control")
	}
	in, err := (codex.Adapter{}).Build(adapters.Request{Profile: profile, ProjectRoot: "/tmp/work", Prompt: "inspect", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for i, arg := range in.Args {
		if arg == "--sandbox" && i+1 < len(in.Args) && in.Args[i+1] == "read-only" {
			ok = true
		}
	}
	if !ok {
		t.Fatal("Codex read-only sandbox was not selected")
	}
}

func TestProbeUsesVersionCommandWithoutCheckingCredentials(t *testing.T) {
	c, err := adapters.ProbeBinary(adapters.Profile{Provider: adapters.Codex, Executable: "/bin/echo"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.ExecutableFound || c.Version != "--version" || c.Authenticated != "NOT_PROBED" {
		t.Fatalf("capability=%+v", c)
	}
}
