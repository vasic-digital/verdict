// Package verdict is a three-valued result type.
//
//	0  ok            fine
//	1  problem       a real problem was found
//	2  undetermined  could not determine
//
// # Why this is a type and not a convention
//
// The third state is the one that gets lost. A missing dependency, an
// unreachable backend or a crashed helper is not a pass and it is not a
// failure — it is an absence of evidence, and reporting it as either is a lie
// about what was measured. This project has found and fixed that exact
// conflation seven times, twice inside its own gate runner, which is why the
// distinction is carried by the type system instead of by everyone remembering.
//
// The two failure modes, both shipped here before:
//
//   - `rc != 0 => failure`. A gate whose helper was missing (rc=127) reported
//     the tree as violating a rule nobody had actually checked.
//   - `undetermined counted as pass`. A suite folded its ENV/SKIP bucket into
//     the pass count and reported green over checks that never ran.
//
// # What is deliberately absent
//
// There is no `Bool()`, no `IsFail()` that also covers Undetermined, and no
// `Error()` on the verdict. Every one of those invites a two-valued read of a
// three-valued fact. The predicates are [Verdict.IsPass], [Verdict.IsProblem]
// and [Verdict.Determined], and exactly one of the first two holds for a
// determined verdict while neither holds for Undetermined.
//
// # Scope
//
// This package knows nothing about any application. It has no dependencies —
// not even inside its own repository — so it can be imported by a gate runner,
// a one-file CLI, an HTTP handler or a health probe without any of them
// inheriting a `go.sum` entry. That is enforced by TestNoDependencies, not
// promised in prose.
package verdict

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Verdict is one of exactly three outcomes. Its numeric value is the process
// exit code a command should return, so `os.Exit(v.Code())` is always correct.
type Verdict uint8

const (
	// OK means the check ran and found nothing wrong.
	OK Verdict = 0

	// Problem means the check ran and found a real problem. It is a
	// determined, actionable finding — never "something went wrong while
	// checking".
	Problem Verdict = 1

	// Undetermined means the check could not reach a conclusion: a missing
	// dependency, an unreachable backend, a crashed helper, an unreadable
	// input. It is never a pass and never a failure.
	Undetermined Verdict = 2
)

// Code returns the process exit code for this verdict: 0, 1 or 2.
func (v Verdict) Code() int { return int(v) }

// Valid reports whether v is one of the three defined states.
func (v Verdict) Valid() bool { return v <= Undetermined }

// String returns the canonical wire name. An out-of-range value renders
// visibly as invalid rather than borrowing a valid name.
func (v Verdict) String() string {
	switch v {
	case OK:
		return "ok"
	case Problem:
		return "problem"
	case Undetermined:
		return "undetermined"
	default:
		return fmt.Sprintf("invalid(%d)", uint8(v))
	}
}

// IsPass reports whether the check ran and passed. False for Undetermined.
func (v Verdict) IsPass() bool { return v == OK }

// IsProblem reports whether a real problem was found. False for Undetermined.
func (v Verdict) IsProblem() bool { return v == Problem }

// Determined reports whether a conclusion was actually reached.
func (v Verdict) Determined() bool { return v == OK || v == Problem }

// ErrInvalidVerdict is returned when a value is not one of the three states.
var ErrInvalidVerdict = errors.New("verdict: not one of ok/problem/undetermined")

// FromCode converts an exit code the caller asserts is already three-valued.
// It REJECTS anything outside 0..2 rather than coercing it — a caller holding
// an arbitrary process exit code wants [FromProcessExitCode] and should have to
// say so.
func FromCode(code int) (Verdict, error) {
	if code < 0 || code > int(Undetermined) {
		return Undetermined, fmt.Errorf("%w: code %d", ErrInvalidVerdict, code)
	}
	return Verdict(code), nil
}

// FromProcessExitCode maps the exit code of an arbitrary subprocess.
//
// 0, 1 and 2 map to the three states. EVERYTHING ELSE maps to Undetermined:
// 127 is "command not found", 126 is "not executable", 130+ is a signal, and a
// negative value means the process was killed before it produced a code. None
// of those is evidence about the thing being checked — they are evidence that
// the check did not run.
func FromProcessExitCode(rc int) Verdict {
	switch rc {
	case 0:
		return OK
	case 1:
		return Problem
	default:
		return Undetermined
	}
}

// Parse converts a canonical name, case-insensitively, with surrounding
// whitespace trimmed. Aliases ("pass", "fail", "error", "skip") are rejected
// on purpose: an alias set is how a fourth informal state, and then a
// conflation, gets in.
func Parse(s string) (Verdict, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ok":
		return OK, nil
	case "problem":
		return Problem, nil
	case "undetermined":
		return Undetermined, nil
	default:
		return Undetermined, fmt.Errorf("%w: %q", ErrInvalidVerdict, s)
	}
}

// MarshalText renders the canonical name. Implementing TextMarshaler covers
// JSON, YAML and map keys in one place.
func (v Verdict) MarshalText() ([]byte, error) {
	if !v.Valid() {
		return nil, fmt.Errorf("%w: %d", ErrInvalidVerdict, uint8(v))
	}
	return []byte(v.String()), nil
}

// UnmarshalText parses the canonical name.
func (v *Verdict) UnmarshalText(b []byte) error {
	parsed, err := Parse(string(b))
	if err != nil {
		return err
	}
	*v = parsed
	return nil
}

// MarshalJSON emits the NAME, never the bare integer, so a reader of a log
// line cannot mistake `2` for "two problems".
func (v Verdict) MarshalJSON() ([]byte, error) {
	t, err := v.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(t))
}

// UnmarshalJSON accepts only the canonical quoted name. A bare number is
// rejected: on the wire, `2` is ambiguous between a verdict and a count.
func (v *Verdict) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%w: expected a quoted name, got %s", ErrInvalidVerdict, b)
	}
	return v.UnmarshalText([]byte(s))
}

// Result is one check's outcome, with the evidence that makes it reportable.
//
// A non-OK Result MUST carry a Reason. "No bluffing: every PASS carries
// positive evidence" has a mirror — every non-pass carries a stated cause, so
// a report can never contain a bare failure with nothing behind it.
type Result struct {
	// Check names what was checked. Required.
	Check string `json:"check"`
	// Verdict is the three-valued outcome.
	Verdict Verdict `json:"verdict"`
	// Reason is a short, stable, machine-greppable cause. Required unless the
	// verdict is OK.
	Reason string `json:"reason,omitempty"`
	// Detail is free-form human context. Always optional.
	Detail string `json:"detail,omitempty"`
}

// Pass builds an OK Result.
func Pass(check string) Result { return Result{Check: check, Verdict: OK} }

// Fail builds a Problem Result. A real problem was found.
func Fail(check, reason string, detail ...string) Result {
	return Result{Check: check, Verdict: Problem, Reason: reason, Detail: joinDetail(detail)}
}

// Cannot builds an Undetermined Result. The check could not run or could not
// reach a conclusion. This is the constructor to reach for when a dependency
// is missing, a backend is unreachable or a helper crashed.
func Cannot(check, reason string, detail ...string) Result {
	return Result{Check: check, Verdict: Undetermined, Reason: reason, Detail: joinDetail(detail)}
}

func joinDetail(d []string) string { return strings.Join(d, " ") }

// Validate reports why a Result is not reportable, or nil if it is.
func (r Result) Validate() error {
	if strings.TrimSpace(r.Check) == "" {
		return errors.New("verdict: Result has no Check name")
	}
	if !r.Verdict.Valid() {
		return fmt.Errorf("verdict: Result %q carries %s", r.Check, r.Verdict)
	}
	if r.Verdict != OK && strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("verdict: Result %q is %s with no Reason", r.Check, r.Verdict)
	}
	return nil
}

// Tally aggregates many Results into one verdict while keeping the counts that
// make the aggregate honest.
type Tally struct {
	OK           int
	Problem      int
	Undetermined int
	Results      []Result
}

// Add records a Result. Use [Tally.AddChecked] where an unreportable Result
// should be refused rather than counted.
func (t *Tally) Add(r Result) {
	t.Results = append(t.Results, r)
	switch r.Verdict {
	case OK:
		t.OK++
	case Problem:
		t.Problem++
	default:
		t.Undetermined++
	}
}

// AddChecked records a Result only if it is reportable.
func (t *Tally) AddChecked(r Result) error {
	if err := r.Validate(); err != nil {
		return err
	}
	t.Add(r)
	return nil
}

// Total is the number of Results recorded.
func (t *Tally) Total() int { return len(t.Results) }

// Verdict aggregates. The precedence is deliberate and is the one place a
// judgement call was made, so it is stated rather than left to be inferred:
//
//		Problem      > Undetermined > OK       (and an EMPTY tally is Undetermined)
//
//	  - Problem wins over Undetermined because a confirmed violation is
//	    determined evidence. Downgrading it to "we could not determine" would
//	    hide a finding that was, in fact, made.
//	  - Undetermined wins over OK because a check that did not run is not a
//	    check that passed. This is the direction the conflation usually goes.
//	  - An empty tally is Undetermined because zero checks run proves nothing.
//	    A suite that silently discovers no checks must not report green.
//
// The aggregate is never the whole story, which is why [Tally.Counts] and
// [Tally.String] always surface the Undetermined count alongside it. A caller
// that reports only the aggregate is discarding the coverage fact.
func (t *Tally) Verdict() Verdict {
	switch {
	case t.Problem > 0:
		return Problem
	case t.Undetermined > 0, t.Total() == 0:
		return Undetermined
	default:
		return OK
	}
}

// Counts returns the per-state counts.
func (t *Tally) Counts() (ok, problem, undetermined int) {
	return t.OK, t.Problem, t.Undetermined
}

// String renders the aggregate with its counts. The undetermined count is
// always present, including when it is zero, so its absence from a report is
// never mistaken for an absence of the problem.
func (t *Tally) String() string {
	return fmt.Sprintf("%s: %d ok, %d problem, %d undetermined (of %d)",
		t.Verdict(), t.OK, t.Problem, t.Undetermined, t.Total())
}

// Unresolved returns the Results that could not be determined, so a report can
// name them individually instead of hiding them behind a count.
func (t *Tally) Unresolved() []Result {
	var out []Result
	for _, r := range t.Results {
		if r.Verdict == Undetermined {
			out = append(out, r)
		}
	}
	return out
}
