export const BOT_EMAIL = "bot@dhruvgoyal.tech";

export function AboutBot() {
  return (
    <>
      <h1>About RoleRankBot</h1>
      <p>
        RoleRankBot reads public job listings so RoleRank can rank them against a person&apos;s
        resume. It only uses public job APIs and sources whose terms allow it.
      </p>
      <h2>How it behaves</h2>
      <ul>
        <li>
          It identifies itself as <code>RoleRankBot</code>, with a link to this page.
        </li>
        <li>It obeys robots.txt, including Crawl-delay.</li>
        <li>It makes at most 1 request per second per host, and backs off on errors.</li>
        <li>It checks for changes first, so an unchanged board costs almost nothing.</li>
        <li>It shows a short excerpt and links to the original posting.</li>
        <li>Postings are never used to train models.</li>
      </ul>
      <h2>Contact</h2>
      <p>
        To ask about the bot, or to have a company&apos;s jobs removed, email{" "}
        <a href={`mailto:${BOT_EMAIL}`}>{BOT_EMAIL}</a>. Removals are done within 7 days.
      </p>
    </>
  );
}
