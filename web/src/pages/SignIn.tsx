import { useState } from "react";
import { useSearchParams } from "react-router";

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
    <>
      <h1>Sign in</h1>
      {error ? <p role="alert">{error}</p> : null}
      <p>Use your Google or GitHub account. New here? Confirm your age first.</p>
      <label className="check">
        <input
          type="checkbox"
          checked={age18}
          onChange={(event) => {
            setAge18(event.target.checked);
          }}
        />
        I am 18 or over
      </label>
      <p>
        <a href={startUrl("google", age18)}>Continue with Google</a>
      </p>
      <p>
        <a href={startUrl("github", age18)}>Continue with GitHub</a>
      </p>
    </>
  );
}
