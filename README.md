# verdict

**Revision:** 1
**Last modified:** 2026-09-01T09:30:00Z

A three-valued result type for Go.

```
0  ok            the check ran and found nothing wrong
1  problem       the check ran and found a real problem
2  undetermined  the check could not reach a conclusion
```

The numeric value **is** the process exit code, so `os.Exit(v.Code())` is
always correct.

## What this is for

The third state is the one that gets lost. A missing dependency, an
unreachable backend, a crashed helper or an unreadable input is not a pass and
it is not a failure — it is an *absence of evidence*, and reporting it as
either is a lie about what was measured.

Two failure modes this type exists to make impossible, both of which have
shipped in real gate runners:

- **`rc != 0 => failure`.** A gate whose helper was missing (`rc=127`)
  reported the tree as violating a rule nobody had actually checked. That is a
  false accusation dressed as a finding.
- **`undetermined counted as pass`.** A suite folded its ENV/SKIP bucket into
  the pass count and reported green over checks that never ran.

Carrying the distinction in the type system beats carrying it in everyone's
memory.

## What is deliberately absent

There is no `Bool()`, no `IsFail()` that also covers `Undetermined`, and no
`Error()` on the verdict. Each of those invites a two-valued read of a
three-valued fact. The predicates are `IsPass()`, `IsProblem()` and
`Determined()` — exactly one of the first two holds for a determined verdict,
and neither holds for `Undetermined`.

`FromCode` rejects anything outside `0..2` rather than coercing it. A caller
holding an arbitrary process exit code wants `FromProcessExitCode`, which maps
`127` (command not found), `126` (not executable), signals and negative values
to `Undetermined` — none of those is evidence about the thing being checked.

## This module is project-not-aware

It knows nothing about any application, and it has **zero dependencies** — not
one `require` line. A gate runner, a one-file CLI, an HTTP handler and a health
probe can all import it without any of them inheriting a `go.sum` entry or a
supply-chain surface.

That is enforced, not promised: `TestNoDependencies` fails if a `require` line
appears, and `TestNoConsumerShapedDependencies` fails if a consumer-shaped
symbol is introduced.

## Install

```bash
go get github.com/vasic-digital/verdict
```

As a submodule, mounted at the consuming project's **root** (nested submodules
are forbidden — HelixConstitution §11.4.28):

```bash
git submodule add git@github.com:vasic-digital/verdict.git submodules/verdict
```

then in the consumer's `go.mod`:

```
require github.com/vasic-digital/verdict v0.0.0
replace github.com/vasic-digital/verdict => ./submodules/verdict
```

## Use

```go
import "github.com/vasic-digital/verdict/pkg/verdict"

func check() verdict.Result {
    out, err := exec.Command("probe").Output()
    if err != nil {
        // NOT a failure of the thing being probed.
        return verdict.Cannot("probe", "probe binary unavailable: "+err.Error())
    }
    if bytes.Contains(out, []byte("BAD")) {
        return verdict.Fail("probe", "probe reported BAD")
    }
    return verdict.Pass("probe")
}

func main() { os.Exit(check().Verdict.Code()) }
```

A `Tally` aggregates results, and its verdict is `Undetermined` if **any**
contributing check was undetermined — an aggregate cannot be more certain than
its least certain input.

## Test

```bash
go test ./...
go test -race ./...
```

The suite includes paired mutations: each gate is accompanied by a test that
breaks the guarded condition and asserts the gate then fails, so a gate that
cannot fail is caught rather than trusted.

## License

MIT — see [LICENSE](LICENSE).
