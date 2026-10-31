// Creates one invite code: APP_DATABASE_URL=... npm run invite -- <uses> <days>
// Only its hash is stored, so this is the one time the code is shown.
import pg from "pg";
import { randomToken, sha256 } from "../crypto.js";

const url = process.env.APP_DATABASE_URL;
if (!url) {
  console.error("APP_DATABASE_URL is required");
  process.exit(1);
}
const uses = Number(process.argv[2] ?? 1);
const days = Number(process.argv[3] ?? 30);
const code = randomToken(16);
const client = new pg.Client({ connectionString: url });
await client.connect();
await client.query(
  `INSERT INTO invites (code_sha256, uses_left, expires_at)
   VALUES ($1, $2, now() + make_interval(days => $3))`,
  [sha256(code), uses, days],
);
await client.end();
console.log(code);
