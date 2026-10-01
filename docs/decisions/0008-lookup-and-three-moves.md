# 0008 One lookup, three moves, and a regression test with every fix

**Date:** 2026-09-26 · **Branch:** `feat/scale-bench`

**Context:** After 0007 the fast flow passed the scale suite, but it was only about 12% faster than the default flow, and slower on gm-strike-tag. The traces showed two sources of extra steps:

- **Exploration one item per step.** The model read or searched one thing, waited about 2 to 4 s, and read the next.
- **Verification after a correct fix.** After fixing a symptom-only bug, the check ran the existing suite. That didn't show the symptom gone, so Jev's step-end check said "not complete". The model then wrote throwaway programs to show it, averaging 4.8 extra bash calls on gm-heading-close.

**Decision:**

- **One lookup tool.** In the fast flow, `lookup` replaces `read`, `search` and `definition`. It takes lists of files or line ranges (`path:START-END`), definitions and searches, and returns them all in one call, capped at 120 KB.
- **Three moves.** The fast prompt frames the work as gather, change and fix: one lookup with everything needed, one apply with every change and a check that proves the whole task, and a further apply only if the check fails.
- **A regression test with every bug fix.** When the task reports a bug, the fix and a test that reproduces the report go in the same apply. The check's output then shows the fix, and Jev can stop the run at once.
- **Whole named files.** The snapshot shows a file holding named code whole when it is under 16 KB, rather than only the definition.
- **Less noise.** The snapshot skips usage searches for plain lowercase words the repository doesn't define, such as `del`. Definitions carry at most 30 comment lines from above.

**Why:** Each change removes LLM steps. On this model every step costs 2 to 4 s regardless of how little it does. So a step that fetches five things, or a fix whose check already proves the symptom gone, saves whole steps.

**Result (scale suite, 3 runs per task, side by side):**

| | Fast flow | Default flow |
|---|---|---|
| Before (0007 final) | 17/18, 48.8 s mean, 32.5 s median | 18/18, 55.7 s mean, 43.5 s median |
| After | 18/18, 36.8 s mean, 23.5 s median | 18/18, 53.9 s mean, 36.5 s median |

- **gm-heading-close:** 25 s with 2 tool calls, against 85 s for the default flow.
- **gm-strike-tag:** still the long pole, and now roughly tied (93 s against 95 s). The implementation passes within about 15 s. The rest is the model learning goldmark v2's test API to write tests.
