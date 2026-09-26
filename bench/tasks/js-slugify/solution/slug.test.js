import { test } from "node:test";
import assert from "node:assert/strict";
import { slugify } from "./slug.js";

test("basic title", () => {
  assert.equal(slugify("Hello World"), "hello-world");
});

test("accents and length", () => {
  assert.equal(slugify("Crème Brûlée"), "creme-brulee");
  assert.equal(slugify("one two three four", 12), "one-two");
});
