import { Link, Navigate } from "react-router";
import { Band, SkillSummary } from "../components";
import { useSession } from "../session";

export function Home() {
  const { me, loading } = useSession();
  if (!loading && me) return <Navigate to={me.profile?.confirmed ? "/matches" : "/resume"} replace />;
  return (
    <>
      <section className="hero">
        <div>
          <h1>Real jobs, ranked against your resume, with every match explained</h1>
          <p className="lede">
            RoleRank reads your resume, finds openings you can actually apply to, and shows exactly which of your skills fit and which
            are missing. It never invents experience for you.
          </p>
          <div className="row">
            <Link className="button primary" to="/signin">
              Get started
            </Link>
            <Link className="button" to="/sources">
              Where the jobs come from
            </Link>
          </div>
          <p className="fine">Open by invite while it&apos;s in early development.</p>
        </div>
        <figure className="hero-example">
          <figcaption>An example of what you see for each role</figcaption>
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
            <p className="fine">Your resume shows Python in &ldquo;Built a fee-reminder service in Python and Postgres&rdquo;.</p>
          </div>
        </figure>
      </section>
      <ol className="steps">
        <li>
          <h3>Upload your resume</h3>
          <p className="fine">PDF or Word. It&apos;s read in a locked-down sandbox and stored encrypted.</p>
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
    </>
  );
}
