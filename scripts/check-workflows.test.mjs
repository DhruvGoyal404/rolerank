import assert from "node:assert/strict";
import { test } from "node:test";
import { hasPipe, lint } from "./check-workflows.mjs";

const sha = "0123456789abcdef0123456789abcdef01234567";

test("accepts a SHA-pinned action and a safe piped step", () => {
  const source = `steps:
  - uses: actions/checkout@${sha} # v7
  - run: |
      set -o pipefail
      npm test | tee out.log
`;
  assert.deepEqual(lint(source), []);
});

test("rejects a tag or branch pin", () => {
  assert.match(lint("- uses: actions/checkout@v7\n")[0], /not pinned/);
  assert.match(lint("      uses: actions/checkout@main\n")[0], /not pinned/);
});

test("allows local actions", () => {
  assert.deepEqual(lint("- uses: ./.github/actions/setup\n"), []);
});

test("rejects a pipe without pipefail, in both run forms", () => {
  assert.match(lint("- run: npm test | tee out.log\n")[0], /pipefail/);
  const block = lint(`- run: |
    npm ci
    npm test | tee out.log
- run: echo next
`);
  assert.equal(block.length, 1);
  assert.match(block[0], /pipefail/);
});

test("doesn't read || or a quoted | as a pipe", () => {
  assert.equal(hasPipe("a || b"), false);
  assert.equal(hasPipe('echo "a | b"'), false);
  assert.equal(hasPipe("a | b"), true);
});

test("rejects docker run on a tag without a digest", () => {
  assert.match(lint("- run: docker run --rm foo/bar:1.2.3 scan\n")[0], /digest/);
  assert.deepEqual(lint("- run: docker run --rm foo/bar:1.2.3@sha256:abc scan\n"), []);
});

test("bans pull_request_target", () => {
  assert.match(lint("on:\n  pull_request_target:\n")[0], /banned/);
});
