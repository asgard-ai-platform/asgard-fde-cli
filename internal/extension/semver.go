package extension

import (
	"strconv"
	"strings"
)

// semver is a parsed version: vMAJOR.MINOR.PATCH with an optional
// pre-release. Build metadata is dropped, as semver ranks without it.
type semver struct {
	core [3]int
	pre  []string
}

// parseSemver reads "v1.2.3", "1.2.3" or "v1.3.0-rc.1".
func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	s, _, _ = strings.Cut(s, "+")
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var v semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return semver{}, false
		}
		v.core[i] = n
	}
	if hasPre {
		if pre == "" {
			return semver{}, false
		}
		v.pre = strings.Split(pre, ".")
	}
	return v, true
}

// compare orders two versions the way semver does: a pre-release ranks below
// the release it precedes.
func (a semver) compare(b semver) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			if a.core[i] < b.core[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePreIdent(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a.pre) < len(b.pre):
		return -1
	case len(a.pre) > len(b.pre):
		return 1
	}
	return 0
}

func comparePreIdent(a, b string) int {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		if na < nb {
			return -1
		} else if na > nb {
			return 1
		}
		return 0
	case errA == nil:
		// A numeric identifier ranks below an alphanumeric one.
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}

// Newer reports whether candidate is a higher version than current. Either one
// not being a version is never newer.
func Newer(candidate, current string) bool {
	c, ok1 := parseSemver(candidate)
	h, ok2 := parseSemver(current)
	return ok1 && ok2 && c.compare(h) > 0
}

// satisfies reports whether version meets constraint, a list of comparisons
// that must all hold (">=0.3.0", ">=0.3.0 <1.0.0", "0.3.0"). understood is
// false for a constraint it cannot read.
func satisfies(version, constraint string) (ok, understood bool) {
	v, parsed := parseSemver(version)
	if !parsed {
		return false, false
	}
	fields := strings.Fields(strings.ReplaceAll(constraint, ",", " "))
	if len(fields) == 0 {
		return false, false
	}
	for _, f := range fields {
		op := strings.TrimRight(f, "v0123456789.-+abcdefghijklmnopqrstuvwxyz")
		want, parsed := parseSemver(strings.TrimPrefix(f, op))
		if !parsed {
			return false, false
		}
		c := v.compare(want)
		var holds bool
		switch op {
		case ">=":
			holds = c >= 0
		case ">":
			holds = c > 0
		case "<=":
			holds = c <= 0
		case "<":
			holds = c < 0
		case "=", "":
			holds = c == 0
		default:
			return false, false
		}
		if !holds {
			return false, true
		}
	}
	return true, true
}
