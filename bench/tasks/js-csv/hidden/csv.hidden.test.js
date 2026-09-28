import { test } from "node:test";
import assert from "node:assert/strict";
import { parse } from "./csv.js";

test("hidden: basics", () => {
  assert.deepEqual(parse(""), []);
  assert.deepEqual(parse("a,b\n1,2\n"), [["a", "b"], ["1", "2"]]);
  assert.deepEqual(parse("a,b\r\n1,2\r\n"), [["a", "b"], ["1", "2"]]);
  assert.deepEqual(parse("a,,b"), [["a", "", "b"]]);
  assert.deepEqual(parse(",\n,"), [["", ""], ["", ""]]);
  assert.deepEqual(parse(" a , b "), [[" a ", " b "]]);
});

test("hidden: quotes", () => {
  assert.deepEqual(parse('"a,b",c'), [["a,b", "c"]]);
  assert.deepEqual(parse('"say ""hi""",x'), [['say "hi"', "x"]]);
  assert.deepEqual(parse('"line1\nline2",z'), [["line1\nline2", "z"]]);
  assert.deepEqual(parse('"crlf\r\ninside"'), [["crlf\r\ninside"]]);
  assert.deepEqual(parse('""'), [[""]]);
  assert.deepEqual(parse('a\n""\nb'), [["a"], [""], ["b"]]);
});

test("hidden: errors", () => {
  assert.throws(() => parse('"unterminated'));
  assert.throws(() => parse('"a"b,c'));
});

test("hidden: blank middle line is a record with one empty field", () => {
  assert.deepEqual(parse("a\n\nb"), [["a"], [""], ["b"]]);
});
