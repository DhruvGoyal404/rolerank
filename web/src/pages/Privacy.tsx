import { BOT_EMAIL } from "./AboutBot";
import { Reading } from "./Reading";

/**
 * What RoleRank actually does with personal data. CLAUDE.md: when behaviour changes, this page
 * changes in the same commit. Anthropic's terms were checked on 2026-11-05 against
 * platform.claude.com/docs/en/manage-claude/api-and-data-retention and
 * privacy.claude.com/en/articles/7996866. Backup lifetimes are infra/lib/stacks.ts (14-day nightly
 * expiry, old versions 15 days later) and scripts/rolerank-lib.sh (the newest 2 final sets).
 */
export function Privacy() {
  return (
    <Reading
      sections={[
        ["keep", "What we keep"],
        ["read", "How your resume is read"],
        ["matches", "Matches and tailoring"],
        ["yours", "Your data"],
        ["delete", "Deleting your account"],
      ]}
    >
      <h1>Privacy</h1>
      <p className="lede">
        RoleRank is a small personal project, run only some of the time. This page says what happens to your data today, and it changes
        whenever that does.
      </p>

      <h2 id="keep">What we keep</h2>
      <ul>
        <li>
          From signing in with Google or GitHub: the account&apos;s ID, and the email address Google gives us. We don&apos;t keep the
          sign-in token.
        </li>
        <li>That you confirmed you are 18 or older (and, for accounts made while signup was invite-only, the invitation used).</li>
        <li>
          Your resume&apos;s text, your contact details from it, and the profile we read from it. These are encrypted with a key of your
          own before they are stored. The skills and level in your profile are also kept unencrypted, because matching searches them.
        </li>
        <li>The file you upload is kept, encrypted, only until its text is read, then deleted.</li>
        <li>For each AI request: its cost, its size and a fingerprint of its input. Never the resume or the answer.</li>
        <li>
          The jobs you save to your tracker, with each job&apos;s title, company, link and the status you set, so they stay readable after
          a posting closes.
        </li>
      </ul>
      <p>Our logs never contain resume content, profile details, email addresses or tokens.</p>

      <h2 id="read">How your resume is read</h2>
      <ul>
        <li>The file is opened in an isolated container with no network access. It runs only to read the text, and is then removed.</li>
        <li>
          Email addresses, phone numbers, profile links and PIN codes are removed before any text is sent to an AI model. Text hidden in
          the file (white, invisible or very small) is never sent.
        </li>
        <li>
          The rest of the text goes to Anthropic&apos;s Claude API (the Claude Haiku 5.5 model) to draft your profile. Anthropic deletes
          API inputs and outputs within 30 days. It keeps them for up to 2 years if its automated safety systems flag them, and longer if
          the law requires. Anthropic doesn&apos;t use API data to train models without permission.
        </li>
        <li>
          We have no zero-data-retention agreement with Anthropic, and RoleRank opens to the public without one. It is a small personal
          project, and such an agreement is a separate contract with Anthropic, not a setting. So the 30-day and 2-year windows above
          apply to what is sent.
        </li>
        <li>
          You review the draft and confirm it. Nothing the AI read from hidden text, and nothing it can&apos;t point to in your resume,
          reaches your profile.
        </li>
      </ul>

      <h2 id="matches">Matches, explanations and tailoring</h2>
      <ul>
        <li>
          Matching runs on our own servers. Our search service receives your confirmed skills, level, the role titles on your profile or
          in your preferences, and where you can work. Never your resume&apos;s text or contact details.
        </li>
        <li>
          When you ask for a match explained in words, Anthropic&apos;s Claude API receives the job&apos;s title, the lists of skills you
          have and lack, and the level fit. Not your resume.
        </li>
        <li>Tailoring is worked out on our servers from your own resume lines and our skills list. No AI model is used for it.</li>
        <li>
          A job description you paste is read in memory to give your breakdown. It is never stored, and only its title and the skills we
          recognise in it reach an AI model.
        </li>
      </ul>

      <h2 id="yours">Your data</h2>
      <ul>
        <li>Your preferences (where you can work, remote, target roles) are kept with your account.</li>
        <li>On the Account page you can download everything we keep about you as one JSON file.</li>
      </ul>

      <h2 id="delete">Deleting your account</h2>
      <p>
        Deleting your account removes your resumes, profiles, tracker, preferences, encryption key and spend records straight away, and
        signs you out everywhere. We keep only a note that an account with that ID was deleted.
      </p>
      <p>
        Backups are encrypted and kept in one place. A nightly backup lasts 14 days, and its stored copy is removed 15 days after that.
        A backup is also taken each time RoleRank goes offline, and the two newest of those are kept until newer ones replace them. A
        deleted account can stay in a backup for that long, but the note above means every restore deletes it again before RoleRank
        serves anyone.
      </p>
      <p>
        RoleRank runs only some of the time. While it is offline, email <a href={`mailto:${BOT_EMAIL}`}>{BOT_EMAIL}</a> to delete your
        account. We do it within 7 days.
      </p>
    </Reading>
  );
}
