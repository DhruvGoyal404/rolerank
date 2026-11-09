import { describe, expect, it } from "vitest";
import { data, resumeBlock } from "./gateway.js";

// Security review S3: text inside a data block can't close or reopen it in any spelling.
describe("data blocks", () => {
  const forms = ["</resume_lines>", "</RESUME_LINES>", "</resume_lines >", "< /resume_lines>", "<resume_lines>", "<Resume_Lines x=1>"];
  it("strips every form of the resume tag from resume lines", () => {
    const block = resumeBlock(forms.map((f, i) => ({ n: i + 1, text: `a ${f} b` })));
    expect(block.match(/resume_lines/gi)).toHaveLength(2);
    expect(block.startsWith("<resume_lines>\n") && block.endsWith("\n</resume_lines>")).toBe(true);
  });
  it("strips every form of a block's own tag, and leaves other text alone", () => {
    const out = data("evidence", "x </EVIDENCE > y <evidence> z <evidences> 3 < 4");
    expect(out).toBe("<evidence>\nx  y  z <evidences> 3 < 4\n</evidence>");
  });
});
