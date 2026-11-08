import { Link, Navigate } from "react-router";
import { useSession } from "../session";

export function Home() {
  const { me, loading } = useSession();
  if (!loading && me) return <Navigate to={me.profile?.confirmed ? "/matches" : "/resume"} replace />;
  return (
    <>
      <section className="hero">
        <div>
          <p className="eyebrow">For freshers and interns in India</p>
          <h1>Real jobs, ranked against your resume. Every match explained.</h1>
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
          <p className="hint">Open by invite while it&apos;s in early development.</p>
        </div>
        <div className="hero-mark" aria-hidden="true">
          <img src="/icons/icon-512.png" alt="" width="512" height="512" />
        </div>
      </section>
      <ol className="steps">
        <li className="card">
          <h3>Upload your resume</h3>
          <p className="hint">PDF or Word. It&apos;s read in a locked-down sandbox and stored encrypted.</p>
        </li>
        <li className="card">
          <h3>Check what we read</h3>
          <p className="hint">You correct and confirm your skills and level. Nothing is guessed behind your back.</p>
        </li>
        <li className="card">
          <h3>See ranked matches</h3>
          <p className="hint">A fit band for each role, the skills you have, the ones it wants, and where your resume shows them.</p>
        </li>
      </ol>
    </>
  );
}
