import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { afterAll, beforeAll, expect, it } from "vitest";
import { httpExtractor } from "./pipeline.js";

// The extractor service's answers, and the fixed code each becomes for the user.
let server: Server;
let answer: { status: number; body: unknown } = { status: 200, body: {} };
beforeAll(async () => {
  server = createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      res.statusCode = answer.status;
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify(answer.body));
    });
  });
  await new Promise<void>((ok) => server.listen(0, "127.0.0.1", ok));
});
afterAll(() => server.close());

it.each([
  [502, { error: "failed", reason: "startup" }, "extract_startup"],
  [502, { error: "failed", reason: "timeout" }, "extract_timeout"],
  [502, { error: "failed", reason: "output" }, "extract_too_much_text"],
  [502, { error: "failed", reason: "sandbox" }, "extract_failed"],
  [502, {}, "extract_failed"],
  [422, { error: "rejected" }, "rejected"],
  [200, { kind: "pdf", error: "file is password protected" }, "password_protected"],
])("an extractor answer %i %j becomes %s", async (status, body, code) => {
  answer = { status, body };
  const ex = httpExtractor(`http://127.0.0.1:${String((server.address() as AddressInfo).port)}`, "t");
  expect(await ex.extract(Buffer.from("%PDF-1.4"))).toEqual({ status: "failed", code });
});
