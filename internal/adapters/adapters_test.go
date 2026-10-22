package adapters_test

import (
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/adapters/claude"
	"github.com/spectide/spectide/internal/adapters/codex"
	"github.com/spectide/spectide/internal/adapters/opencode"
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
