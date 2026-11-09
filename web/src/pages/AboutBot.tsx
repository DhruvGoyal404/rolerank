import { Reading } from "./Reading";

export const BOT_EMAIL = "bot@dhruvgoyal.tech";

export function AboutBot() {
  return (
    <Reading
      sections={[
        ["behaviour", "How it behaves"],
        ["contact", "Contact"],
      ]}
    >
      <h1>About RoleRankBot</h1>
      <p className="lede">
        RoleRankBot reads public job listings so RoleRank can rank them against a person&apos;s resume. It only uses public job APIs and
        sources whose terms allow it.
      </p>
      <h2 id="behaviour">How it behaves</h2>
      <ul>
        <li>
          It identifies itself as <code>RoleRankBot</code>, with a link to this page.
        </li>
        <li>It obeys robots.txt, including Crawl-delay.</li>
        <li>It makes at most 1 request per second per host, and backs off on errors.</li>
        <li>It checks for changes first, so an unchanged board costs almost nothing.</li>
        <li>It links to the original posting, and shows no text from Greenhouse or Lever postings.</li>
        <li>Postings are never used to train models.</li>
      </ul>
      <h2 id="contact">Contact</h2>
      <p>
        To ask about the bot, or to have a company&apos;s jobs removed, email <a href={`mailto:${BOT_EMAIL}`}>{BOT_EMAIL}</a>. Removals
        are done within 7 days.
      </p>
    </Reading>
  );
}
