package checkpoint

import (
	"fmt"
	"maps"

	"github.com/timbrinded/girdle/internal/jev"
)

// The step-end fan-out asks Jev a broad set of questions about the work
// itself: the request's changes and the latest check's output. Fan-out is
// free: 200 questions take about as long as one, about 0.3 s. One question
// decides; the rest are logged so later tuning can use them.
//
// Replayed over 1,410 logged step-end decisions (decision 0013), the status
// question on this state separated safe stops from unsafe ones at an AUC of
// 0.92, against 0.87 for "complete" on the logged summary. Status alone
// stopped once live on a check that only printed a file, so the policy also
// needs Jev to read the check as exercising the task. Together they made
// 1,081 safe stops and 26 unsafe ones, against 1,088 and 34 today.

// statusQuestion and checkExercises are the ones the fan-out policy acts on.
var checkExercises = jev.Noul("Does `check` exercise the behaviour `task` asks for, through a test or by running the program?")

var statusQuestion = jev.Choice("What is the state of the work on `task`?", map[string]string{
	"done_verified":   "done, and `check_output` proves every part of it",
	"done_unverified": "done, but the tool results don't prove every part",
	"partial":         "some parts of `task` are not done yet",
	"broken":          "the code or the tests are failing",
	"wrong":           "the approach doesn't match what `task` asks for",
})

// stepFanBase holds the questions asked about every request. Only status
// and check_exercises act; each of the others separated safe from unsafe stops in the replay,
// in one direction or the other, and is kept for tuning.
var stepFanBase = map[string]jev.Question{
	"status":           statusQuestion,
	"check_exercises":  checkExercises,
	"complete_hidden":  jev.Noul("Would acceptance tests for `task`, written by someone else from the words of `task` alone, pass on the code in `changes`?"),
	"new_test_ran":     jev.Noul("Does `check_output` show a test that was added or changed in `changes` running and passing?"),
	"check_build_only": jev.Noul("Is `check` only a build, vet, lint or compile step, with no test or run of the behaviour `task` asks for?"),
	"output_clean":     jev.Noul("Does `check_output` show everything passing, with no failures, skips, panics, tracebacks or warnings?"),
	"output_truncated": jev.Noul("Is `check_output` cut off or too short to confirm that the result works?"),
	"edge_unhandled":   jev.Noul("Does `task` mention an edge case, such as empty input, zero, limits, unicode, ordering, truncation or an error, that the code in `changes` does not handle?"),
	"contradiction":    jev.Noul("Does anything in `changes` or `check_output` contradict a detail of `task`?"),
	"misunderstood":    jev.Noul("Has the agent misunderstood any part of `task`?"),
	"ambiguous":        jev.Noul("Is `task` ambiguous, so that the agent had to guess what was meant?"),
	"any_req_open":     jev.Noul("Is any item in `requirements` not done yet?"),
	"tests_weakened":   jev.Noul("Do `changes` weaken, skip or delete an existing test or assertion?"),
	"unfinished":       jev.Noul("Do `changes` contain placeholders, TODOs or unfinished code?"),
	"api_misuse":       jev.Noul("Does the code in `changes` use a library or API in a way that may be wrong?"),
	"full_suite_fail":  jev.Noul("Would running the project's whole test suite likely show a failure caused by `changes`?"),
	"confidence": jev.Score("How likely is it that acceptance tests for `task`, written from its words alone, pass on the code in `changes`?",
		"very unlikely", "unlikely", "even", "likely", "very likely"),
	"remaining": jev.Score("How much work is left to finish `task`?", "none", "a trivial amount", "some", "a lot"),
	"next": jev.Choice("What should the agent do next?", map[string]string{
		"stop":       "stop: `task` is done and proven",
		"add_tests":  "add or improve tests",
		"fix_code":   "fix or finish the code",
		"check_more": "run more checks",
		"ask_user":   "ask the user a question",
	}),
}

// StepFanQuestions returns the fan-out question set: the base set plus
// three questions per requirement.
func StepFanQuestions(requirements []string) map[string]jev.Question {
	qs := maps.Clone(stepFanBase)
	for i := range requirements {
		qs[fmt.Sprintf("r%d_impl", i)] = jev.Noul(fmt.Sprintf("Do `changes` implement `requirements[%d]` fully, including every detail it states?", i))
		qs[fmt.Sprintf("r%d_tested", i)] = jev.Noul(fmt.Sprintf("Does a test in `changes` check `requirements[%d]`?", i))
		qs[fmt.Sprintf("r%d_shown", i)] = jev.Noul(fmt.Sprintf("Does `check_output` show `requirements[%d]` working?", i))
	}
	return qs
}

// decideFan stops when Jev reads the work as done and verified by a check
// that exercises the task.
func (p StepPolicy) decideFan(a map[string]jev.Answer) (Action, string) {
	switch {
	case a["status"].Probabilities["done_verified"] < p.Verified:
		return Continue, "not_verified"
	case a["check_exercises"].Noul < p.Exercises:
		return Continue, "check_not_exercising"
	}
	return Stop, "done_verified"
}
