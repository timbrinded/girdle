import { test } from "node:test";
import assert from "node:assert/strict";
import { slugify } from "./slug.js";

test("basic title", () => {
  assert.equal(slugify("Hello World"), "hello-world");
});
