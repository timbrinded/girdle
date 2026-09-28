import { test } from "node:test";
import assert from "node:assert/strict";
import { slugify } from "./slug.js";

test("hidden: lowercase and hyphens", () => {
  assert.equal(slugify("Hello, World!"), "hello-world");
  assert.equal(slugify("  --Already--Slugged--  "), "already-slugged");
  assert.equal(slugify("a   b___c"), "a-b-c");
  assert.equal(slugify("Top 10 Tips (2026)"), "top-10-tips-2026");
});

test("hidden: accents", () => {
  assert.equal(slugify("Crème Brûlée"), "creme-brulee");
  assert.equal(slugify("Ñandú Über"), "nandu-uber");
});

test("hidden: empty", () => {
  assert.equal(slugify(""), "");
  assert.equal(slugify("!!!"), "");
});

test("hidden: max length cuts at a hyphen", () => {
  assert.equal(slugify("one two three four", 13), "one-two-three");
  assert.equal(slugify("one two three four", 12), "one-two");
  assert.equal(slugify("abcdefghij", 4), "abcd");
  assert.ok(slugify("x ".repeat(100)).length <= 60);
  assert.ok(!slugify("aaaa bbbb", 5).endsWith("-"));
});
