# 0013 Jev fan-out: ask far more questions, keep the ones that pay

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

Until now each checkpoint asked Jev one to three questions, plus one or two per requirement. Jev's own guidance is the opposite: ask everything that might matter in one request, since questions branch off one encoding of the state. This record covers a broad re-test of that idea, first offline on logged runs, then live.

## Fan-out is free

On a 3k-token state, adding questions barely moves latency. Each question adds about 18 input tokens.

| Questions in one request | Median latency | Input tokens |
|---|---|---|
| 1 | 322 ms | 3,269 |
| 10 | 262 ms | 3,422 |
| 40 | 281 ms | 3,962 |
| 100 | 303 ms | 5,042 |
| 200 | 333 ms | 6,942 |

## Offline replays

The replay data is every scored benchmark run with a log: 2,764 runs, 164 of them failures. The scripts are in `research/experiments/fanout/`.

### Step end: is the work done? (kept, as `-stepfan`)

- **Data.** Every logged step-end decision, 1,410 in all, labelled safe when the run passed and no later apply changed a file.
- **Method.** Each decision was asked again two ways. The first used the logged state with today's questions. The second used a richer state carrying the actual changes, the check command and its output, with 59 questions in one request.
- **Result.** The richer state with a status question separated safe from unsafe stops at an AUC of 0.92, against 0.87 for today's questions on the logged state. Thresholds were chosen on the other tasks, leave-one-task-out:

  | Policy | Safe stops | Unsafe stops |
  |---|---|---|
  | Today's questions, logged state | 1,088 | 34 |
  | Today's questions, richer state | 1,125 | 35 |
  | P(status is done_verified) ≥ 0.34, richer state | 1,161 | 35 |
  | The same, and "does the check exercise the task?" ≥ 0.7 | 1,081 | 26 |

- **Status alone failed live.** On gm-strike-tag the agent used an apply whose check was only `head -12` of its new test file. That check exits 0, and status read the work as done and verified at 0.51. The test file didn't compile, so the hidden tests failed. The fan-out had the answer: "does the check exercise the task?" scored 0.39. Status alone was never tested against a check like that offline, because today's question asks for passing tests in the output.
- **The policy now needs both answers.** Stop when P(done_verified) ≥ 0.34 and the check exercises the task at 0.7 or more. Offline that makes 8 fewer false "done"s than today for 7 fewer early stops. A missed early stop costs one more LLM step: a median 2.4 s and a mean 11.7 s. A false "done" is the failure the North Star rules out, so accuracy wins that trade.

### False "done"s: what fan-out can't catch

- **The 34 unsafe stops.** 21 came from configurations since dropped: the fast model and minimal effort. The rest are subtle code bugs, such as truncating a slug at the wrong hyphen, or a function consuming an iterator twice.
- **One question per spec rule made it worse.** For the four tasks whose spec is a bulleted comment, Jev was asked, rule by rule, whether the code follows the rule and whether a test checks it. Among stops, "implemented" scored an AUC of 0.24: Jev rated the broken code as following the rules better than the working code.
- **Why.** Jev judges meaning. It can't run code in its head. Bugs like these need a test that runs, which is what the cross-check does.
- **Requirement splitting adds noise.** "Run the tests with `npm test`" becomes a requirement, and it is the one Jev most often flags as not implemented.

### After a failed check: tests or code? (not built)

- **Data.** 583 failed applies, labelled by where the agent's next fix went.
- **Result.** Jev predicted it at an AUC of 0.97. At a test-at-fault score of 0.8 or more it was right 137 times out of 139.
- **Why not built.** The agent already fixes the right side. Only 2 of the 583 cases show Jev and the agent disagreeing in a run that then failed.

### Which files will the agent need? (kept, as `-prefetch`)

- **The cost it targets.** Every run on the real-repository suite looks things up, 3.1 lookup steps per run. Those steps take about 11 s of a 42 s run. Across all recent fast runs, LLM time is 85% of the total, and Jev's checkpoints are about 1%.
- **Method.** Every file in goldmark and more-itertools was judged against each scale task, with one question per file and its own request. Jev saw the path, the first 25 lines and the top-level definitions.
- **Result.** Jev's top 10 held 14 of the 21 files that agents looked up in at least a quarter of runs. Its top 5 held 10. A search for the task's code names found 8 and 6.
- **Isolation matters.** Batching 15 files into one request found 7 in the top 5, against 10 when each file had its own request. So prefetch sends one request per file, 32 at a time, within 4 s.
- **What goes into the snapshot.** Up to six files scoring 0.7 or more, and only files of 16 KB or less, shown whole. Round 2 below explains both limits.
- **The earlier failure.** Judged from paths alone, Jev ranked a docs index first for a code task. It needs the content.

### After a lookup: which files next? (not built)

- **Method.** 81 logged lookups that were followed by lookups of new files, mostly on gm-strike-tag. Jev was told what had been looked up so far and asked, file by file, whether the agent would still need each one.
- **Result.** Jev's top 5 held 41 of the 300 files looked up later, about three times chance. Once the first lookups are done, the agent's exploration is too scattered to predict.

### Other fan-out uses measured

- **Classifying failures.** Of the 583 failed applies, Jev read 235 as build errors, 184 as a wrong check command, and 141 as failed assertions.
- **Tampering.** Questions about weakened tests, unrelated changes and unfinished code separated safe from unsafe stops only weakly, at AUCs from 0.63 to 0.71. They are asked and logged under `-stepfan`, but nothing acts on them.

## Live results

### Round 1: prefetch, and the first step-end policy

Warm, 3 runs per task, side by side: the six real-repository tasks plus the three small tasks where the most early stops were missed.

| Real-repository suite | Fast flow | With `-prefetch` |
|---|---|---|
| Passed | 18/18 | 18/18 |
| Mean time | 50.9 s | 35.9 s |
| Median time | 38.0 s | 23.5 s |
| Lookups per run | 5.2 | 2.1 |
| LLM steps per run | 8.0 | 4.5 |
| Cost per run | $0.0099 | $0.0101 |

- **The biggest change was gm-heading-close.** It went from 3.7 lookups and 48 s to no lookups and 17 s, because the file the fix belongs in was already in the snapshot.
- **The prefetch takes about 1 s before the first call.** That covers 125 requests for goldmark's files.
- **The cost doesn't change.** The Jev tokens are paid for by fewer LLM steps.
- **The first step-end policy made the one false "done" described above.** It was replaced before round 2.

### Round 2: the corrected step-end policy, and prefetch again

The same nine tasks, 3 runs each, with four variants side by side. All 108 runs passed.

| Mean time | Fast flow | `-prefetch` | `-stepfan` | Both |
|---|---|---|---|---|
| Real-repository suite | 38.1 s | 35.6 s | 39.2 s | 36.3 s |
| Three small tasks | 23.0 s | 23.7 s | 25.8 s | 27.7 s |

- **Prefetch helped less than in round 1.** The median fell from 22.0 s to 16.0 s and the mean only from 38.1 s to 35.6 s.
- **It helped the same task again.** gm-heading-close went from 46 s to 13 s.
- **It hurt the same task again.** mi-argsort went from 47 s to 75 s, after 59 s to 73 s in round 1.
  - Its files are 28 to 255 KB, so they went in only as outlines, and the agent still looked up the ranges it needed.
  - Files scoring from 0.5 to 0.7, such as the changelog, went in too.
  - Every prefetch run of it made two failed applies before passing.
  - So prefetch now adds only whole files, and only those scoring 0.7 or more.
- **The step-end fan-out was about neutral on time, and made no false "done"s.** It stopped early 17 times on the real-repository suite against 13. Its check-exercise rule refused 5 stops, all on go-ctx-migration, at 0.55 to 0.61 for checks that ran `go test ./...`. Each refusal cost an extra step.

### Round 3: whole-file prefetch, and the step-end fan-out on every small task

| Suite, 3 runs per task | Fast flow | `-prefetch` | `-prefetch -stepfan` | `-stepfan` |
|---|---|---|---|---|
| Real repositories, mean | 36.9 s, 18/18 | 34.3 s, 18/18 | 36.4 s, 18/18 | not run |
| Small fixtures, mean | 20.2 s, 39/39 | not run | not run | 20.2 s, 38/39 |

- **Prefetch now leaves more-itertools alone.** None of its small files scores 0.7, and mi-argsort ran at its usual speed. On goldmark it still picks the file the heading fix needs.
- **The step-end fan-out's one failure was a blind spot shared by every check.** On js-slugify the agent's tests passed, and so did all 8 cross-check tests. Every question read the work as done, at 0.82 to 0.98. All of them agreed with code that truncates slugs wrongly. Asked on the same state, today's questions scored "complete" at 0.95 and would have stopped too.

## Decision

- **`-prefetch` joins `-fast`.** It was faster on the real-repository suite in all three rounds, with no failures:

  | Round | Fast flow | With prefetch |
  |---|---|---|
  | 1 | 50.9 s | 35.9 s |
  | 2 | 38.1 s | 35.6 s |
  | 3 | 36.9 s | 34.3 s |

  The gain comes where the needed file is small and outside the code the task names. It costs about $0.004 a run in Jev tokens, and it does nothing for repositories small enough to snapshot whole.
- **`-stepfan` joins `-fast`.**
  - Live, across 111 runs, it was neutral on time. Its only failure was one today's policy would also have made.
  - Offline, across 1,410 decisions, it made 26 false "done"s against 34.
  - Every step end now logs 59 answers, so later thresholds can be tuned from real runs rather than replays.
- **Not built.** Classifying failed checks, one question per spec rule, and predicting the files needed after a lookup.
- **One request per checkpoint has an exception.** For per-item judgements, such as one question per file, give each item its own request and send them in parallel. Isolation beat batching, 10 files to 7 in each task's top five.
