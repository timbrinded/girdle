"""The broad step-end question bank. Every question points at named fields
of the state. The rich state adds `changes`, `files_changed`,
`tests_changed`, `check` and `check_output` to the logged one."""
from jevlib import noul, choice, score

CURRENT = {
    "complete": noul("Do `recent_steps` show that all of `task` has been done, and does the output of the last command show the result working, for example the build and the tests passing?"),
}
def current_reqs(reqs):
    q = {}
    for i in range(len(reqs)):
        q[f"req_{i}"] = noul(f"Do `recent_steps` show that `requirements[{i}]` has been done?")
        q[f"req_{i}_is_instruction"] = noul(f"Is `requirements[{i}]` an instruction to do something, rather than a description of the situation?")
    return q

BROAD = {
    # evidence
    "complete_hidden": noul("Would acceptance tests for `task`, written by someone else from the words of `task` alone, pass on the code in `changes`?"),
    "tests_asked": noul("Does `task` ask the agent to write new tests or change existing ones?"),
    "tests_written": noul("Do `changes` add or change a test?"),
    "new_test_ran": noul("Does `check_output` show a test that was added or changed in `changes` running and passing?"),
    "only_old_tests": noul("Does `check_output` show only tests that existed before `changes`, and none of the new ones?"),
    "check_build_only": noul("Is `check` only a build, vet, lint or compile step, with no test or run of the behaviour `task` asks for?"),
    "check_exercises": noul("Does `check` exercise the behaviour `task` asks for, through a test or by running the program?"),
    "output_clean": noul("Does `check_output` show everything passing, with no failures, skips, panics, tracebacks or warnings?"),
    "output_truncated": noul("Is `check_output` cut off or too short to confirm that the result works?"),
    "demo_shown": noul("Does `check_output` show the new behaviour directly, for example the program's real output for the task's example?"),
    "regression_proved": noul("Does `check_output` or `recent_steps` show a regression test failing without the fix and passing with it?"),
    # spec fit
    "edge_untested": noul("Does `task` mention an edge case, such as empty input, zero, limits, unicode, ordering, truncation or an error, that no test in `changes` covers?"),
    "edge_unhandled": noul("Does `task` mention an edge case that the code in `changes` does not handle?"),
    "happy_path_only": noul("Do the tests in `changes` check only the single example given in `task`?"),
    "tautological": noul("Would the tests in `changes` still pass if the new code were wrong?"),
    "expected_from_spec": noul("Do the expected values in the tests in `changes` follow from the words of `task`, rather than from whatever the code happens to output?"),
    "exact_format": noul("Does `task` specify an exact output, format or message, and does `check_output` show exactly that?"),
    "contradiction": noul("Does anything in `changes` or `check_output` contradict a detail of `task`?"),
    "misunderstood": noul("Has the agent misunderstood any part of `task`?"),
    "ambiguous": noul("Is `task` ambiguous, so that the agent had to guess what was meant?"),
    "keep_behaviour": noul("Does `task` ask to keep some existing behaviour unchanged, and do `changes` keep it?"),
    "names_match": noul("Do the names of functions, flags, fields or options in `changes` match exactly the names that `task` uses?"),
    "adversarial": noul("Could a small, reasonable input described in `task` make the code in `changes` give a wrong answer?"),
    "errors_handled": noul("Does `task` ask for particular error handling, and do `changes` do it?"),
    "all_reqs_tested": noul("Is every item in `requirements` checked by some test in `changes`?"),
    "all_reqs_shown": noul("Does `check_output` show every item in `requirements` working?"),
    "any_req_open": noul("Is any item in `requirements` not done yet?"),
    # hygiene
    "tests_weakened": noul("Do `changes` weaken, skip or delete an existing test or assertion?"),
    "unrelated": noul("Do `changes` touch code unrelated to `task`?"),
    "unfinished": noul("Do `changes` contain placeholders, TODOs or unfinished code?"),
    "api_misuse": noul("Does the code in `changes` use a library or API in a way that may be wrong?"),
    "full_suite_fail": noul("Would running the project's whole test suite likely show a failure caused by `changes`?"),
    "reviewer_approve": noul("Would a careful senior engineer accept `changes` as a complete and correct answer to `task`?"),
    # distributions
    "confidence": score("How likely is it that acceptance tests for `task`, written from its words alone, pass on the code in `changes`?",
                        ["very unlikely", "unlikely", "even", "likely", "very likely"]),
    "remaining": score("How much work is left to finish `task`?", ["none", "a trivial amount", "some", "a lot"]),
    "breadth": score("How many distinct behaviours does `task` ask for?", ["one", "two", "three", "four", "five or more"]),
    "status": choice("What is the state of the work on `task`?", {
        "done_verified": "done, and `check_output` proves every part of it",
        "done_unverified": "done, but the tool results don't prove every part",
        "partial": "some parts of `task` are not done yet",
        "broken": "the code or the tests are failing",
        "wrong": "the approach doesn't match what `task` asks for"}),
    "next": choice("What should the agent do next?", {
        "stop": "stop: `task` is done and proven",
        "add_tests": "add or improve tests",
        "fix_code": "fix or finish the code",
        "check_more": "run more checks",
        "ask_user": "ask the user a question"}),
}
def broad_reqs(reqs):
    q = {}
    for i in range(len(reqs)):
        q[f"r{i}_tested"] = noul(f"Does a test in `changes` check `requirements[{i}]`?")
        q[f"r{i}_impl"] = noul(f"Do `changes` implement `requirements[{i}]` fully, including every detail it states?")
        q[f"r{i}_shown"] = noul(f"Does `check_output` show `requirements[{i}]` working?")
    return q
