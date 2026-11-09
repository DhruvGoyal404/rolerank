import { Link, Navigate } from "react-router";
import { Band, SkillSummary } from "../components";
import { useSession } from "../session";

export function Home() {
  const { me, loading } = useSession();
  if (!loading && me) return <Navigate to={me.profile?.confirmed ? "/matches" : "/resume"} replace />;
  return (
    <>
      <section className="hero" aria-labelledby="hero-title">
        <div>
          <h1 id="hero-title">Real jobs, ranked against your resume</h1>
          <p className="lede">
            RoleRank finds openings you can actually apply to, in India or remote from India, and shows exactly which of your skills fit,
            which are missing, and the line of your resume that proves each one. It never invents experience for you.
          </p>
          <div className="actions">
            <Link className="button primary" to="/signin">
              Get started
            </Link>
            <Link className="button quiet" to="/sources">
              Where the jobs come from
            </Link>
          </div>
          <p className="fine">Sign in with Google or GitHub. Free, and you can delete everything at any time.</p>
        </div>
        <figure className="hero-example">
          <figcaption>What you see for each role (an example)</figcaption>
          <div className="match">
            <div className="match-head">
              <h2 className="job-title">Backend Engineer Intern</h2>
              <Band band="Strong" />
            </div>
            <ul className="facts">
              <li>A product company</li>
              <li>Bengaluru</li>
              <li>posted 2 days ago</li>
            </ul>
            <SkillSummary
              compact
              e={{ matched: ["Python", "SQL", "REST APIs"], missing: ["Kubernetes"], niceMatched: [], level: { job: "intern", profile: "intern", fit: "fits" } }}
            />
            <p className="evidence-note">
              <span className="cite">L14</span>
              <span>
                Your resume shows Python in &ldquo;Built a fee-reminder service in Python and Postgres&rdquo;.
              </span>
            </p>
          </div>
        </figure>
      </section>

      <section className="how" aria-labelledby="how-title">
        <div>
          <h2 id="how-title">From resume to ranked roles in a few minutes</h2>
          <p className="lede">Four steps, and you check every one of them.</p>
        </div>
        <ol className="steps">
          <li>
            <h3>Sign in</h3>
            <p className="fine">With Google or GitHub. No password, no email list.</p>
          </li>
          <li>
            <h3>Upload your resume</h3>
            <p className="fine">PDF or Word. It&apos;s read in a locked-down sandbox and stored encrypted with a key of your own.</p>
          </li>
          <li>
            <h3>Check what we read</h3>
            <p className="fine">You correct and confirm your skills and level. Nothing is guessed behind your back.</p>
          </li>
          <li>
            <h3>See ranked matches</h3>
            <p className="fine">A fit band for each role, the skills you have, the ones it wants, and where your resume shows them.</p>
          </li>
        </ol>
      </section>

      <section className="promise-band" aria-labelledby="promise-title">
        <div>
          <h2 id="promise-title">What RoleRank will never do</h2>
          <p>Matching you to a job is only useful if you can trust it, so these are rules in the code, not promises in a brochure.</p>
        </div>
        <ul>
          <li>
            <strong>Invent experience.</strong> Every suggestion cites a line of your own resume.
          </li>
          <li>
            <strong>Score you as a percentage.</strong> You get a fit band and the reasons behind it.
          </li>
          <li>
            <strong>Scrape sites that forbid it.</strong> Jobs come from public sources that allow it. <Link to="/sources">See which</Link>.
          </li>
          <li>
            <strong>Keep your data after you delete it.</strong> One button removes everything. <Link to="/privacy">Read how</Link>.
          </li>
        </ul>
      </section>
    </>
  );
}
