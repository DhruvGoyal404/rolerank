import { expect, it } from "vitest";
import { safeNext } from "./app.js";

it.each(["/", "/matches", "/matches?page=2"])("keeps the same-origin path %s", (path) => {
  expect(safeNext(path)).toBe(path);
});

it.each([
  "https://evil.example",
  "//evil.example",
  "/\\evil.example",
  "\\\\evil.example",
  "javascript:alert(1)",
  "",
  undefined,
  ["/a", "/b"],
])("sends %j home", (value) => {
  expect(safeNext(value)).toBe("/");
});
