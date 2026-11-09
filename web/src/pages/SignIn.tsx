import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Journey } from "../components";
import { GitHubMark, GoogleMark } from "../icons";

const ERRORS: Record<string, string> = {
  state: "That sign-in link expired. Please try again.",
  provider: "We couldn't complete sign-in with that provider. Please try again.",
  age_required: "New accounts must confirm they are 18 or over. Tick the box, then choose your provider again.",
  signup_limited: "Too many new accounts were made from this network in the last hour. Try again later, or sign in to an account you already have.",
};

export function startUrl(provider: "google" | "github", age18: boolean): string {
  return `/api/auth/${provider}/start${age18 ? "?age18=1" : ""}`;
}

export function SignIn() {
  const [params] = useSearchParams();
  const [age18, setAge18] = useState(false);
  const error = ERRORS[params.get("error") ?? ""];

  return (
    <div className="signin">
      <section className="panel" aria-labelledby="signin-title">
        <Journey at={0} />
        <h1 id="signin-title">Sign in to RoleRank</h1>
        {error ? (
          <p className="error" role="alert">
            {error}
          </p>
        ) : null}
        <p className="fine">Use the Google or GitHub account you already have. We never see your password.</p>
        <div className="age">
          <label className="check">
            <input
              type="checkbox"
              checked={age18}
              aria-describedby="age-help"
              onChange={(event) => {
                setAge18(event.target.checked);
              }}
            />
            I am 18 or over
          </label>
          <p id="age-help" className="fine">
            Needed once, when you create an account.
          </p>
        </div>
        <div className="providers">
          <a className="button" href={startUrl("google", age18)}>
            <GoogleMark />
            Continue with Google
          </a>
          <a className="button" href={startUrl("github", age18)}>
            <GitHubMark />
            Continue with GitHub
          </a>
        </div>
      </section>
      <section className="kept" aria-labelledby="kept-title">
        <h2 id="kept-title">What we keep when you sign in</h2>
        <dl>
          <div>
            <dt>Your account ID, not your password</dt>
            <dd>Google or GitHub tells us who you are. From Google we also get your email address; from GitHub, nothing more.</dd>
          </div>
          <div>
            <dt>Your resume, encrypted</dt>
            <dd>Read in a sandbox with no network, contact details removed before any AI model sees it, and stored with a key of your own.</dd>
          </div>
          <div>
            <dt>Nothing after you delete it</dt>
            <dd>
              One button on your account page removes everything. <Link to="/privacy">The privacy page</Link> says exactly how.
            </dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
