package adapters_test

import (
	"errors"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/adapters/claude"
	"github.com/spectide/spectide/internal/adapters/codex"
	"github.com/spectide/spectide/internal/adapters/opencode"
	"strings"
	"testing"
	"time"
)

func TestBuildersDoNotContainBypassFlags(t *testing.T) {
	cases := []adapters.Adapter{claude.Adapter{}, codex.Adapter{}, opencode.Adapter{}}
	for _, a := range cases {
		configuredTimeout := 17 * time.Minute
		in, e := a.Build(adapters.Request{ProjectRoot: "/tmp/work", Prompt: "test", Timeout: configuredTimeout, Profile: adapters.Profile{Executable: string(a.Name())}})
		if e != nil && a.Name() == adapters.Claude {
			continue
		}
		if in.Timeout != configuredTimeout {
			t.Fatalf("%s adapter dropped configured timeout: got %s want %s", a.Name(), in.Timeout, configuredTimeout)
		}
		for _, arg := range in.Args {
			if arg == "--dangerously-bypass-approvals-and-sandbox" {
				t.Fatal("unsafe flag")
			}
		}
	}
}

func TestProviderNormalizersAcceptJSONLAndDiscardRawPayload(t *testing.T) {
	cases := []struct {
		adapter adapters.Adapter
		output  string
		count   int
	}{
		{claude.Adapter{}, `{"type":"system","message":"private body"}` + "\n" + `{"type":"assistant"}` + "\n" + `{"type":"result","result":"done"}` + "\n", 3},
		{codex.Adapter{}, `{"type":"item.completed","item":{"type":"agent_message","text":"part"}}` + "\n" + `{"type":"turn.completed","last_agent_message":"done"}` + "\n", 2},
		{opencode.Adapter{}, `{"type":"text","text":"done"}` + "\n" + `{"type":"step_finish"}` + "\n", 2},
	}
	for _, tc := range cases {
		a := tc.adapter
		events, err := a.Normalize([]byte(tc.output))
		if err != nil {
			t.Fatalf("%s: %v", a.Name(), err)
		}
		if len(events) != tc.count {
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
	claudeEffort, err := (claude.Adapter{}).Build(adapters.Request{Profile: profile, ProjectRoot: "/tmp/work", Prompt: "inspect", Effort: "high"})
	if err != nil || !hasArgPair(claudeEffort.Args, "--effort", "high") {
		t.Fatalf("verified Claude effort flag missing: invocation=%+v err=%v", claudeEffort, err)
	}
	if _, err = (opencode.Adapter{}).Build(adapters.Request{Profile: profile, ProjectRoot: "/tmp/work", Prompt: "inspect", ReadOnly: true}); err == nil {
		t.Fatal("OpenCode read-only policy was accepted without a verified control")
	} else {
		assertProviderErrorCode(t, err, "PERMISSION_DENIED")
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

func TestAdapterErrorsExposeStableCodesAndUnderlyingCause(t *testing.T) {
	cause := errors.New("probe detail")
	err := adapters.WrapProviderError("PROBE_FAILED", "provider probe failed", cause)
	assertProviderErrorCode(t, err, "PROBE_FAILED")
	if !errors.Is(err, cause) {
		t.Fatal("provider error did not preserve its underlying cause")
	}
	if _, err := adapters.NormalizeJSONL(adapters.Codex, []byte("{bad json}\n")); err == nil {
		t.Fatal("malformed event was accepted")
	} else {
		assertProviderErrorCode(t, err, "MALFORMED_PROVIDER_EVENT")
	}
}

func assertProviderErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var providerErr *adapters.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error %T does not expose ProviderError", err)
	}
	if providerErr.Code != want {
		t.Fatalf("error code = %q, want %q", providerErr.Code, want)
	}
}

func hasArgPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestProbeUsesVersionCommandWithoutCheckingCredentials(t *testing.T) {
	c, err := adapters.ProbeBinary(adapters.Profile{Provider: adapters.Codex, Executable: "/bin/echo"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.ExecutableFound || c.Version != "--version" || c.Authenticated != "NOT_PROBED" || c.ApprovedExecutablePath == "" || len(c.ExecutableSHA256) != 64 {
		t.Fatalf("capability=%+v", c)
	}
}
