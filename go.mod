module github.com/vasic-digital/verdict

go 1.26.2

// DELIBERATELY EMPTY REQUIRE SET — load-bearing, not incidental.
//
// 1. ZERO DEPENDENCY WEIGHT IS THE WHOLE POINT.
//    This type is meant to be imported by gate runners, one-file CLIs, health
//    probes and HTTP handlers alike. If it dragged in a database driver or a
//    web framework, every consumer would inherit those lines in its own
//    `go.sum` and its own supply-chain surface, and the cheap thing would stop
//    being cheap. Stdlib only, forever.
//
// 2. IT IS A SEPARATE MODULE SO "GENERIC" IS COMPILER-ENFORCED.
//    A `pkg/` directory inside a consuming application would still be able to
//    import that application's own domain types. A separate module cannot:
//    it does not require the consumer, so no consumer-shaped symbol can ever
//    appear here. The module boundary makes the reusability claim a fact
//    rather than a convention a reviewer has to police.
//    `TestNoDependencies` asserts this and fails if a require line appears.
//
// 3. IT IS NOT `internal/`.
//    Go's `internal/` is importable only from within its own module, by
//    language rule. That is the least reusable placement available, and this
//    is one of the two types in the project most worth reusing.
