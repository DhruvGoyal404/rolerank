import { BOT_EMAIL } from "./AboutBot";

/**
 * What RoleRank actually does with personal data. CLAUDE.md: when behaviour changes, this page
 * changes in the same commit. Anthropic's terms were checked on 2026-11-05 against
 * platform.claude.com/docs/en/manage-claude/api-and-data-retention and
 * privacy.claude.com/en/articles/7996866.
 */
export function Privacy() {
  return (
    <>
      <h1>Privacy</h1>
      <p>
        RoleRank is a student project, open by invitation only. This page says what happens to your
        data today. It changes whenever that does.
      </p>

      <h2>What we keep</h2>
      <ul>
        <li>
          From signing in with Google or GitHub: the account&apos;s ID, and the email address Google
          gives us. We don&apos;t keep the sign-in token.
        </li>
        <li>That you confirmed you are 18 or older, and the invitation you used.</li>
        <li>
          Your resume&apos;s text, your contact details from it, and the profile we read from it.
          These are encrypted with a key of your own before they are stored. The skills and level in
          your profile are also kept unencrypted, because matching searches them.
        </li>
        <li>
          The file you upload is kept, encrypted, only until its text is read, then deleted.
        </li>
        <li>
          For each AI request: its cost, its size and a fingerprint of its input. Never the resume or
          the answer.
        </li>
        <li>
          The jobs you save to your tracker, with each job&apos;s title, company, link and the status
          you set, so they stay readable after a posting closes.
        </li>
      </ul>
      <p>Our logs never contain resume content, profile details, email addresses or tokens.</p>

      <h2>How your resume is read</h2>
      <ul>
        <li>
          The file is opened in an isolated container with no network access. It runs only to read
          the text, and is then removed.
        </li>
        <li>
          Email addresses, phone numbers, profile links and PIN codes are removed before any text is
          sent to an AI model. Text hidden in the file (white, invisible or very small) is never sent.
        </li>
        <li>
          The rest of the text goes to Anthropic&apos;s Claude API (the Claude Haiku 5.5 model) to
          draft your profile. Anthropic deletes API inputs and outputs within 30 days. It keeps them
          for up to 2 years if its automated safety systems flag them, and longer if the law
          requires. Anthropic doesn&apos;t use API data to train models without permission.
        </li>
        <li>
          We have no zero-data-retention agreement with Anthropic. We will look at one again before
          RoleRank opens to the public.
        </li>
        <li>
          You review the draft and confirm it. Nothing the AI read from hidden text, and nothing it
          can&apos;t point to in your resume, reaches your profile.
        </li>
      </ul>

      <h2>Deleting your account</h2>
      <p>
        Deleting your account removes your resumes, profiles, encryption key and spend records
        straight away. We keep only a note that an account with that ID was deleted. That note is
        how a deletion survives a restore from a backup.
      </p>
      <p>
        Backups aren&apos;t running yet. When they start, this page will say how long a deleted
        account can stay in a backup.
      </p>
      <p>
        RoleRank runs only some of the time. While it is offline, email{" "}
        <a href={`mailto:${BOT_EMAIL}`}>{BOT_EMAIL}</a> to delete your account. We do it within 7
        days.
      </p>
    </>
  );
}
