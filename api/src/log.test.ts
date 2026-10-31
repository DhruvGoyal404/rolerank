import { Writable } from "node:stream";
import { expect, it } from "vitest";
import { createLog, pick } from "./log.js";

it("drops every field not on the allowlist", () => {
  expect(
    pick({ event: "x", status: 200, email: "person@example.com", token: "t", resume: "r" }),
  ).toEqual({ event: "x", status: 200 });
});

it("drops allowed fields holding objects", () => {
  expect(pick({ event: { nested: "person@example.com" } })).toEqual({});
});

it("writes nothing it wasn't allowed to", async () => {
  const lines: string[] = [];
  const sink = new Writable({
    write(chunk: Buffer, _encoding, done) {
      lines.push(chunk.toString());
      done();
    },
  });
  const log = createLog(sink);
  log.info({ event: "ok", ...({ email: "person@example.com" } as object) });
  await new Promise((resolve) => setImmediate(resolve));
  expect(lines.join("")).toContain('"event":"ok"');
  expect(lines.join("")).not.toContain("person@example.com");
});
