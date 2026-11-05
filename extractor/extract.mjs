// Runs inside the sandbox (ARCHITECTURE.md, "Resume pipeline"): no network, read-only root,
// 512 MB, 1 CPU, 20 s. Reads one PDF or DOCX on stdin and writes JSON to stdout:
//   {"kind":"pdf"|"docx","pages":n,"lines":[{"n":1,"text":"...","hidden":false,"tiny":false}],"error":""}
// Lines are numbered here; everything downstream cites L<n>. The Go side has already checked
// size, magic bytes and the zip structure; this file trusts nothing anyway.
import { getDocument, OPS } from "pdfjs-dist/legacy/build/pdf.mjs";
import mammoth from "mammoth";
import JSZip from "jszip";

const MAX_PAGES = 30;
const MAX_LINES = 2000;
const TINY_PT = 4;

async function readStdin() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  return Buffer.concat(chunks);
}

function finish(out) {
  out.lines = out.lines.slice(0, MAX_LINES).map((l, i) => ({ n: i + 1, ...l }));
  process.stdout.write(JSON.stringify(out));
}

async function pdf(bytes) {
  const task = getDocument({
    data: new Uint8Array(bytes),
    isEvalSupported: false,
    enableXfa: false,
    disableFontFace: true,
    useSystemFonts: false,
    stopAtErrors: true,
  });
  const doc = await task.promise;
  if (doc.numPages > MAX_PAGES) throw new Error(`too many pages: ${doc.numPages}`);
  const lines = [];
  for (let p = 1; p <= doc.numPages; p++) {
    const page = await doc.getPage(p);
    // Text drawn white or in render mode 3 (invisible) is hidden from a reader but not from a
    // parser: collect it from the operator list so its lines can be flagged.
    const hidden = new Set();
    const ops = await page.getOperatorList();
    let fillWhite = false;
    let invisible = false;
    for (let i = 0; i < ops.fnArray.length; i++) {
      const fn = ops.fnArray[i];
      const args = ops.argsArray[i];
      if (fn === OPS.setFillRGBColor) {
        const [r, g, b] = typeof args[0] === "string" ? hexRGB(args[0]) : args;
        fillWhite = r > 0.95 * (r > 1 ? 255 : 1) && g > 0.95 * (g > 1 ? 255 : 1) && b > 0.95 * (b > 1 ? 255 : 1);
      } else if (fn === OPS.setFillGray) {
        fillWhite = args[0] > 0.95;
      } else if (fn === OPS.setTextRenderingMode) {
        invisible = args[0] === 3;
      } else if ((fn === OPS.showText || fn === OPS.showSpacedText) && (fillWhite || invisible)) {
        const glyphs = Array.isArray(args[0]) ? args[0] : [];
        const s = glyphs.map((g) => (g && typeof g === "object" ? g.unicode || "" : "")).join("").trim();
        if (s) hidden.add(s);
      }
    }
    const content = await page.getTextContent();
    const rows = new Map();
    for (const it of content.items) {
      if (!("str" in it) || !it.str.trim()) continue;
      const y = Math.round(it.transform[5]);
      const size = Math.hypot(it.transform[2], it.transform[3]);
      if (!rows.has(y)) rows.set(y, []);
      rows.get(y).push({ x: it.transform[4], str: it.str, size });
    }
    const ys = [...rows.keys()].sort((a, b) => b - a);
    for (const y of ys) {
      const parts = rows.get(y).sort((a, b) => a.x - b.x);
      const text = parts.map((q) => q.str).join(" ").replace(/\s+/g, " ").trim();
      if (!text) continue;
      const tiny = parts.every((q) => q.size < TINY_PT);
      // Either way round: pdf.js clips text that runs off the page, so a line can be a
      // fragment of a longer hidden string.
      const isHidden = [...hidden].some((h) => h.length >= 3 && (text.includes(h) || (text.length >= 8 && h.includes(text))));
      lines.push({ text, hidden: isHidden, tiny });
    }
    page.cleanup();
  }
  await task.destroy();
  return { kind: "pdf", pages: doc.numPages, lines };
}

function hexRGB(hex) {
  const v = parseInt(hex.replace("#", ""), 16);
  return [(v >> 16) & 255, (v >> 8) & 255, v & 255];
}

async function docx(bytes) {
  const { value } = await mammoth.extractRawText({ buffer: bytes });
  // mammoth keeps hidden runs (w:vanish) as ordinary text, so find them in the XML directly.
  const zip = await JSZip.loadAsync(bytes);
  const xml = await zip.file("word/document.xml").async("string");
  const hidden = new Set();
  for (const run of xml.match(/<w:r[ >][\s\S]*?<\/w:r>/g) || []) {
    const props = run.match(/<w:rPr>[\s\S]*?<\/w:rPr>/)?.[0] || "";
    const size = Number(props.match(/<w:sz w:val="(\d+)"/)?.[1] || 24) / 2;
    const white = /<w:color w:val="FFFFFF"/i.test(props);
    if (/<w:vanish\/>|<w:vanish w:val="(true|1)"/.test(props) || white || size < TINY_PT) {
      const t = (run.match(/<w:t[^>]*>([^<]*)<\/w:t>/g) || []).map((x) => x.replace(/<[^>]+>/g, "")).join("").trim();
      if (t.length >= 3) hidden.add(decode(t));
    }
  }
  const lines = value
    .split(/\r?\n/)
    .map((t) => t.replace(/\s+/g, " ").trim())
    .filter(Boolean)
    .map((text) => ({
      text,
      hidden: [...hidden].some((h) => text.includes(h) || (text.length >= 8 && h.includes(text))),
      tiny: false,
    }));
  return { kind: "docx", pages: 0, lines };
}

function decode(s) {
  return s.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"').replace(/&apos;/g, "'").replace(/&amp;/g, "&");
}

const out = { kind: "", pages: 0, lines: [], error: "" };
try {
  const bytes = await readStdin();
  if (bytes.subarray(0, 5).toString("latin1") === "%PDF-") Object.assign(out, await pdf(bytes));
  else if (bytes[0] === 0x50 && bytes[1] === 0x4b) Object.assign(out, await docx(bytes));
  else throw new Error("not a PDF or DOCX");
} catch (e) {
  out.error = String(e && e.name === "PasswordException" ? "password-protected" : e?.message || e).slice(0, 200);
}
finish(out);
