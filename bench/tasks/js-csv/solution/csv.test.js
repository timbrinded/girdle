import { test } from "node:test";
import assert from "node:assert/strict";
import { parse } from "./csv.js";

test("simple", () => {
  assert.deepEqual(parse("a,b\n1,2"), [["a", "b"], ["1", "2"]]);
});

test("quotes", () => {
  assert.deepEqual(parse('"a,b",c'), [["a,b", "c"]]);
});
