import { Link } from "react-router";
import { BOT_EMAIL } from "./AboutBot";
import { Reading } from "./Reading";

export function Sources() {
  return (
    <Reading
      sections={[
        ["sources", "Sources"],
        ["optout", "Employers: remove your jobs"],
      ]}
    >
      <h1>Where our jobs come from</h1>
      <p className="lede">
        RoleRank reads public job listings and links to the original posting, where you apply. We never use postings to train models.
      </p>
      <h2 id="sources">Sources</h2>
      <ul>
        <li>
          <strong>Greenhouse and Lever:</strong> we show the title, company, location, our own skill tags and fit, and a link to the
          original posting. We show no posting text. Description text is used only on our servers, to work out skills and match.
        </li>
        <li>
          <strong>Himalayas and RemoteOK:</strong> we show a short excerpt and a link back to the source.
        </li>
      </ul>
      <h2 id="optout">Employers: remove your jobs</h2>
      <p>
        Your postings belong to you. To opt out, email <a href={`mailto:${BOT_EMAIL}`}>{BOT_EMAIL}</a> with your company name and
        careers page. We put your board on our denylist and delete what we hold from it within 7 days, and we don&apos;t collect it again.
      </p>
      <p>
        Read more <Link to="/about/bot">about our bot</Link>.
      </p>
    </Reading>
  );
}
