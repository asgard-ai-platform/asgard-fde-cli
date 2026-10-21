package extension

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// CheckRequires holds an Extension's `requires` against this repository and
// this asgard-cli. Every mismatch is a warning: the Extension is copied
// regardless, and the FDE Agent decides what to do about it.
//
// `platform` is only named, never checked: the platform has no route that says
// which capabilities a Stack has, so the FDE Agent judges it from the
// asgard-platform material.
//
// cliVersion is this binary's version. A development build ("v0.1.8-main") is
// compared as the release it is named after; one with no version at all is not
// compared.
func CheckRequires(req Requires, skillsRoot string, lock *Lock, cliVersion string) []string {
	var out []string
	if want := strings.TrimSpace(req.AsgardCLI); want != "" {
		core, _, _ := strings.Cut(strings.TrimPrefix(cliVersion, "v"), "-")
		if _, ok := parseSemver(core); !ok {
			out = append(out, fmt.Sprintf("it requires asgard-cli %s; this build has no release version to compare (%q)", want, cliVersion))
		} else if ok, understood := satisfies(core, want); !understood {
			out = append(out, fmt.Sprintf("it requires asgard-cli %q, which is not a version range this asgard-cli can read", want))
		} else if !ok {
			out = append(out, fmt.Sprintf("it requires asgard-cli %s and this is %s; `asgard-cli update` replaces it", want, cliVersion))
		}
	}
	for _, s := range req.Skills {
		if _, err := os.Stat(filepath.Join(skillsRoot, s, "SKILL.md")); err != nil {
			out = append(out, fmt.Sprintf("it relies on the skill %s, which this repository does not have; `asgard-cli init` writes the shipped skills and `asgard-cli skill update` the platform's", s))
		}
	}
	for _, e := range req.Extensions {
		if _, ok := lock.Extensions[e]; ok {
			continue
		}
		if _, ok := lock.Find(path.Base(e)); ok {
			continue
		}
		out = append(out, fmt.Sprintf("it relies on the Extension %s, which this repository does not hold; `asgard-cli extension get %s`", e, e))
	}
	if len(req.Platform) > 0 {
		out = append(out, fmt.Sprintf("it relies on the platform's %s; whether this Stack has it is not something asgard-cli can ask", strings.Join(req.Platform, ", ")))
	}
	return out
}
