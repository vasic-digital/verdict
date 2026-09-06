package verdict

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The three states exist, are distinct, and carry the mandated exit codes.
// ---------------------------------------------------------------------------

func TestThreeStatesAndTheirCodes(t *testing.T) {
	cases := []struct {
		v    Verdict
		code int
		name string
	}{
		{OK, 0, "ok"},
		{Problem, 1, "problem"},
		{Undetermined, 2, "undetermined"},
	}
	for _, c := range cases {
		if got := c.v.Code(); got != c.code {
			t.Errorf("%s: Code() = %d, want %d", c.name, got, c.code)
		}
		if got := c.v.String(); got != c.name {
			t.Errorf("Code %d: String() = %q, want %q", c.code, got, c.name)
		}
		if !c.v.Valid() {
			t.Errorf("%s: Valid() = false, want true", c.name)
		}
	}
	if OK == Problem || Problem == Undetermined || OK == Undetermined {
		t.Fatal("the three states are not distinct")
	}
}

// The defect this type exists to prevent: reading `undetermined` as either
// pass or fail. Found and fixed seven times in this project, twice in its own
// gate runner.
func TestUndeterminedIsNeitherPassNorFail(t *testing.T) {
	if Undetermined.IsPass() {
		t.Error("Undetermined.IsPass() = true — could-not-determine reported as PASS")
	}
	if Undetermined.IsProblem() {
		t.Error("Undetermined.IsProblem() = true — could-not-determine reported as FAIL")
	}
	if Undetermined.Determined() {
		t.Error("Undetermined.Determined() = true")
	}
	if !OK.Determined() || !Problem.Determined() {
		t.Error("OK and Problem must both be Determined()")
	}
	// Exactly one of the three predicates holds for each state — there is no
	// value for which two hold, and none for which zero hold.
	for _, v := range []Verdict{OK, Problem, Undetermined} {
		n := 0
		for _, b := range []bool{v.IsPass(), v.IsProblem(), v == Undetermined} {
			if b {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s: %d predicates hold, want exactly 1", v, n)
		}
	}
}

func TestZeroValueIsOK(t *testing.T) {
	var v Verdict
	if v != OK {
		t.Fatalf("zero value = %v, want OK — a Result built by literal must default to the safe state only when explicitly checked", v)
	}
}

func TestInvalidCodeIsRejectedNotCoerced(t *testing.T) {
	for _, bad := range []int{-1, 3, 127, 255} {
		if _, err := FromCode(bad); err == nil {
			t.Errorf("FromCode(%d) accepted an out-of-range code", bad)
		}
	}
	v := Verdict(9)
	if v.Valid() {
		t.Error("Verdict(9).Valid() = true")
	}
	if !strings.Contains(v.String(), "9") {
		t.Errorf("Verdict(9).String() = %q — an invalid verdict must render visibly, never as a valid name", v.String())
	}
}

// ---------------------------------------------------------------------------
// Process exit codes. A helper that crashed did not determine anything.
// ---------------------------------------------------------------------------

func TestFromProcessExitCode(t *testing.T) {
	cases := map[int]Verdict{
		0:   OK,
		1:   Problem,
		2:   Undetermined,
		127: Undetermined, // command not found — a missing dependency
		126: Undetermined, // not executable
		139: Undetermined, // SIGSEGV — a crashed helper
		255: Undetermined,
		-1:  Undetermined, // killed before it produced a code
	}
	for rc, want := range cases {
		if got := FromProcessExitCode(rc); got != want {
			t.Errorf("FromProcessExitCode(%d) = %v, want %v", rc, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Result: a non-OK verdict without a reason is not reportable.
// ---------------------------------------------------------------------------

func TestNonOKResultRequiresAReason(t *testing.T) {
	if err := (Result{Check: "c", Verdict: Problem}).Validate(); err == nil {
		t.Error("a Problem with no Reason validated — a bare failure is not evidence")
	}
	if err := (Result{Check: "c", Verdict: Undetermined}).Validate(); err == nil {
		t.Error("an Undetermined with no Reason validated — 'could not determine' must say why")
	}
	if err := (Result{Check: "c", Verdict: OK}).Validate(); err != nil {
		t.Errorf("a plain OK failed validation: %v", err)
	}
	if err := (Result{Verdict: OK}).Validate(); err == nil {
		t.Error("a Result with no Check name validated")
	}
	if err := Fail("c", "boom", "detail").Validate(); err != nil {
		t.Errorf("Fail() produced an invalid Result: %v", err)
	}
	if err := Cannot("c", "backend unreachable").Validate(); err != nil {
		t.Errorf("Cannot() produced an invalid Result: %v", err)
	}
}

func TestConstructorsCarryTheRightVerdict(t *testing.T) {
	if Pass("c").Verdict != OK {
		t.Error("Pass() is not OK")
	}
	if Fail("c", "r").Verdict != Problem {
		t.Error("Fail() is not Problem")
	}
	if Cannot("c", "r").Verdict != Undetermined {
		t.Error("Cannot() is not Undetermined")
	}
}

// ---------------------------------------------------------------------------
// Serialisation. The wire form is the name, never the bare integer, so a
// reader of a log line cannot mistake 2 for "two problems".
// ---------------------------------------------------------------------------

func TestJSONRoundTripUsesTheName(t *testing.T) {
	for _, v := range []Verdict{OK, Problem, Undetermined} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %v: %v", v, err)
		}
		if string(b) != `"`+v.String()+`"` {
			t.Errorf("Marshal(%v) = %s, want %q", v, b, v.String())
		}
		var back Verdict
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal %s: %v", b, err)
		}
		if back != v {
			t.Errorf("round trip: %v -> %s -> %v", v, b, back)
		}
	}
}

func TestUnmarshalRejectsUnknownAndNumericForms(t *testing.T) {
	for _, bad := range []string{`"pass"`, `"fail"`, `"error"`, `"true"`, `2`, `"2"`, `""`} {
		var v Verdict
		if err := json.Unmarshal([]byte(bad), &v); err == nil {
			t.Errorf("Unmarshal(%s) accepted a non-canonical form as %v", bad, v)
		}
	}
}

func TestParseIsCaseInsensitiveButNameOnly(t *testing.T) {
	for _, s := range []string{"ok", "OK", "Ok", " ok "} {
		got, err := Parse(s)
		if err != nil || got != OK {
			t.Errorf("Parse(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := Parse("passed"); err == nil {
		t.Error("Parse accepted an alias — aliases are how conflation gets in")
	}
}

// ---------------------------------------------------------------------------
// Tally: aggregation precedence, and the counts that keep it honest.
// ---------------------------------------------------------------------------

func TestTallyPrecedenceProblemBeatsUndetermined(t *testing.T) {
	var tl Tally
	tl.Add(Pass("a"))
	tl.Add(Cannot("b", "helper crashed"))
	tl.Add(Fail("c", "violation"))
	if got := tl.Verdict(); got != Problem {
		t.Errorf("Verdict() = %v, want Problem — a confirmed finding must not be downgraded to 'we do not know'", got)
	}
	// ...but the incomplete coverage must remain visible, not be swallowed.
	if tl.Undetermined != 1 {
		t.Errorf("Undetermined count = %d, want 1", tl.Undetermined)
	}
	if !strings.Contains(tl.String(), "1 undetermined") {
		t.Errorf("Tally.String() = %q — it must surface the undetermined count", tl.String())
	}
}

func TestTallyUndeterminedBeatsOK(t *testing.T) {
	var tl Tally
	tl.Add(Pass("a"))
	tl.Add(Pass("b"))
	tl.Add(Cannot("c", "no such tool"))
	if got := tl.Verdict(); got != Undetermined {
		t.Errorf("Verdict() = %v, want Undetermined — a check that could not run is never a pass", got)
	}
}

func TestEmptyTallyIsUndetermined(t *testing.T) {
	var tl Tally
	if got := tl.Verdict(); got != Undetermined {
		t.Errorf("empty Tally.Verdict() = %v, want Undetermined — zero checks run proves nothing", got)
	}
}

func TestTallyAllPassIsOK(t *testing.T) {
	var tl Tally
	tl.Add(Pass("a"))
	tl.Add(Pass("b"))
	if got := tl.Verdict(); got != OK {
		t.Errorf("Verdict() = %v, want OK", got)
	}
}

func TestTallyRejectsAnUnreportableResult(t *testing.T) {
	var tl Tally
	if err := tl.AddChecked(Result{Check: "c", Verdict: Problem}); err == nil {
		t.Error("AddChecked accepted a Problem with no Reason")
	}
	if tl.Total() != 0 {
		t.Errorf("a rejected Result was still counted (total=%d)", tl.Total())
	}
}

// ---------------------------------------------------------------------------
// PAIRED MUTATION PROOFS (FR-032, SC-012, §1.1).
//
// Each mutation reproduces a conflation this project has actually shipped, and
// asserts that the guard above goes RED. A proof that runs zero mutations
// proves nothing — three gates in this repository shipped exactly that.
// ---------------------------------------------------------------------------

// mutantCollapse is the historical defect verbatim: `rc != 0 => failure`.
// It is the shape the constitution's own step-1 caller had before 2026-08-27.
func mutantCollapseNonZeroToProblem(rc int) Verdict {
	if rc == 0 {
		return OK
	}
	return Problem
}

func TestPairedMutation_CollapsingNonZeroToProblemIsCaught(t *testing.T) {
	// The real guard: 127 (command not found) is a missing dependency.
	if got := FromProcessExitCode(127); got != Undetermined {
		t.Fatalf("CONTROL FAILED: real FromProcessExitCode(127) = %v, want Undetermined; "+
			"the mutation below would prove nothing", got)
	}
	// The mutation: collapse everything non-zero onto Problem.
	if got := mutantCollapseNonZeroToProblem(127); got == Undetermined {
		t.Fatal("MUTATION SURVIVED: the collapsing implementation still reported Undetermined")
	} else {
		t.Logf("mutation caught: collapsed rc=127 to %v instead of undetermined", got)
	}
}

// mutantTallyUndeterminedAsPass is the second historical defect: an ENV/SKIP
// bucket folded into the pass count so the suite reports green.
func mutantTallyUndeterminedAsPass(tl Tally) Verdict {
	if tl.Problem > 0 {
		return Problem
	}
	return OK // <-- swallows tl.Undetermined
}

func TestPairedMutation_UndeterminedCountedAsPassIsCaught(t *testing.T) {
	var tl Tally
	tl.Add(Pass("a"))
	tl.Add(Cannot("b", "backend unreachable"))

	if got := tl.Verdict(); got != Undetermined {
		t.Fatalf("CONTROL FAILED: real Tally.Verdict() = %v, want Undetermined", got)
	}
	if got := mutantTallyUndeterminedAsPass(tl); got == Undetermined {
		t.Fatal("MUTATION SURVIVED: the swallowing implementation still reported Undetermined")
	} else {
		t.Logf("mutation caught: swallowing aggregation reported %v for a run containing %d undetermined checks",
			got, tl.Undetermined)
	}
}

// mutantTallyUndeterminedWins downgrades a confirmed violation to
// could-not-determine — the opposite conflation, equally wrong.
func mutantTallyUndeterminedWins(tl Tally) Verdict {
	if tl.Undetermined > 0 {
		return Undetermined
	}
	if tl.Problem > 0 {
		return Problem
	}
	return OK
}

func TestPairedMutation_ConfirmedProblemDowngradedIsCaught(t *testing.T) {
	var tl Tally
	tl.Add(Fail("a", "real violation"))
	tl.Add(Cannot("b", "helper crashed"))

	if got := tl.Verdict(); got != Problem {
		t.Fatalf("CONTROL FAILED: real Tally.Verdict() = %v, want Problem", got)
	}
	if got := mutantTallyUndeterminedWins(tl); got == Problem {
		t.Fatal("MUTATION SURVIVED: the downgrading implementation still reported Problem")
	} else {
		t.Logf("mutation caught: a confirmed violation was reported as %v", got)
	}
}

// ---------------------------------------------------------------------------
// The reusability claim, asserted rather than asserted-in-prose.
// ---------------------------------------------------------------------------

// depHit is one dependency-introducing line found in a go.mod.
type depHit struct {
	Line int
	Text string
}

// scanModuleDependencies reports every line of a go.mod that introduces a
// dependency. It is the single implementation behind both TestNoDependencies
// and its paired mutation, so the mutation drives the code the real gate runs
// rather than a copy of it that could agree while the original rots.
func scanModuleDependencies(src []byte) []depHit {
	var hits []depHit
	for i, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "require") || strings.HasPrefix(trimmed, "replace") {
			hits = append(hits, depHit{i + 1, trimmed})
		}
	}
	return hits
}

// internalSegments reports every `internal` path segment in a slash-separated
// relative path. Shared with the paired mutation for the same reason as above.
func internalSegments(rel string) []string {
	var out []string
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == "internal" {
			out = append(out, seg)
		}
	}
	return out
}

// TestNoDependencies fails the moment a `require` line appears in go.mod.
// It reads the REAL go.mod of the REAL module, located by walking up from this
// file's own package directory — no frozen host path, no committed copy.
func TestNoDependencies(t *testing.T) {
	root := moduleRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		// An unreadable go.mod is an ABSENCE OF EVIDENCE about this module's
		// dependencies, not a finding that it has none. Failing here is the
		// only honest option a Go test has: t.Skip would be counted as a pass.
		t.Fatalf("could not read the module's go.mod: %v — this is undetermined, "+
			"NOT a clean result", err)
	}
	// Belt-and-braces against the empty-subject-set failure: a scan over no
	// bytes reports no hits and looks identical to a clean result.
	//
	// HONEST BOUNDARY (§11.4.6): this branch is DEFENSIVE, not a live control,
	// and it was measured rather than assumed. Emptying go.mod and running the
	// suite does not reach it — the go tool refuses first, with "error reading
	// go.mod: missing module declaration", so no test binary is ever built.
	// It is kept because it costs nothing and the day this file is read by
	// something other than `go test` it becomes the difference between a
	// blind pass and a stop. Do not cite it as a proven control.
	if len(b) == 0 {
		t.Fatal("the module's go.mod is empty — the scan would pass by looking at " +
			"nothing, which is the blind-instrument failure this family exists to prevent")
	}
	for _, h := range scanModuleDependencies(b) {
		t.Errorf("go.mod:%d introduces a dependency (%q). This module is stdlib-only by "+
			"contract: every consumer inherits whatever it requires.", h.Line, h.Text)
	}
}

// TestPairedMutation_TheDependencyScannerCatchesASeededRequire drives the same
// scanner the gate uses, with go.mod content as DATA. Without this, "zero
// dependencies is enforced" rested on a scanner nothing had ever seen fire.
func TestPairedMutation_TheDependencyScannerCatchesASeededRequire(t *testing.T) {
	// CONTROL: this module's real, dependency-free go.mod — comment prose that
	// merely mentions the word "require" included — produces no hits, so a hit
	// below means the seeded line and not a scanner that fires on anything.
	clean, err := os.ReadFile(filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("could not read the module's go.mod: %v", err)
	}
	if hits := scanModuleDependencies(clean); len(hits) != 0 {
		t.Fatalf("CONTROL FAILED: the scanner reported %v on the real go.mod", hits)
	}
	// The mutation, in both forms a dependency can actually take.
	for _, seeded := range []string{
		string(clean) + "\nrequire example.com/anything v1.2.3\n",
		string(clean) + "\nrequire (\n\texample.com/anything v1.2.3\n)\n",
		string(clean) + "\nreplace example.com/anything => ../elsewhere\n",
	} {
		hits := scanModuleDependencies([]byte(seeded))
		if len(hits) == 0 {
			t.Errorf("MUTATION SURVIVED: the scanner did not report a seeded dependency. "+
				"The gate is inoperative — it would not catch the exact defect it exists "+
				"for. Seeded tail: %q", seeded[len(string(clean)):])
			continue
		}
		t.Logf("mutation caught at go.mod:%d — %q", hits[0].Line, hits[0].Text)
	}
}

// TestNotInInternal asserts the package is importable from outside this module.
// `internal/` anywhere in the import path would make that impossible by
// language rule, which is the placement the operator constraint forbids.
func TestNotInInternal(t *testing.T) {
	root := moduleRoot(t)
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	rel, err := filepath.Rel(root, here)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	if segs := internalSegments(rel); len(segs) != 0 {
		t.Fatalf("package sits under an `internal/` segment (%s) — nothing outside "+
			"module %q could ever import it", rel, "github.com/vasic-digital/verdict")
	}
}

// TestPairedMutation_TheInternalScannerCatchesASeededSegment drives the same
// predicate the gate uses, with the path as DATA.
func TestPairedMutation_TheInternalScannerCatchesASeededSegment(t *testing.T) {
	// CONTROL: this package's real location produces no hits.
	for _, ok := range []string{"pkg/verdict", ".", "a/b/c", "internalise/x", "x/myinternal"} {
		if segs := internalSegments(ok); len(segs) != 0 {
			t.Fatalf("CONTROL FAILED: %q reported %d `internal` segment(s)", ok, len(segs))
		}
	}
	for _, seeded := range []string{"internal/verdict", "pkg/internal/verdict", "a/b/internal"} {
		if segs := internalSegments(seeded); len(segs) == 0 {
			t.Errorf("MUTATION SURVIVED: %q was not reported as internal. The gate is "+
				"inoperative — the package could be moved somewhere no consumer could "+
				"import it and this would stay green.", seeded)
			continue
		}
		t.Logf("mutation caught: %q", seeded)
	}
}

// moduleRoot walks up from the working directory to the nearest go.mod.
// Derived, never literal — §11.4 "no frozen host assumptions".
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("walked to the filesystem root without finding a go.mod")
		}
		dir = parent
	}
}
