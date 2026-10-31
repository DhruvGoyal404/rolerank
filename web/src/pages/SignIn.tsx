import { useState } from "react";
import { useSearchParams } from "react-router";

const ERRORS: Record<string, string> = {
  state: "That sign-in link expired. Please try again.",
  provider: "We couldn't complete sign-in with that provider. Please try again.",
  invite_required: "New accounts need a valid invite code.",
  age_required: "New accounts must confirm they are 18 or over.",
};

export function startUrl(provider: "google" | "github", invite: string, age18: boolean): string {
  const query = new URLSearchParams();
  if (invite.trim() !== "") query.set("invite", invite.trim());
  if (age18) query.set("age18", "1");
  const suffix = query.toString();
  return `/api/auth/${provider}/start${suffix === "" ? "" : `?${suffix}`}`;
}

export function SignIn() {
  const [params] = useSearchParams();
  const [invite, setInvite] = useState("");
  const [age18, setAge18] = useState(false);
  const error = ERRORS[params.get("error") ?? ""];

  return (
    <>
      <h1>Sign in</h1>
      {error ? <p role="alert">{error}</p> : null}
      <p>Returning? Just choose your provider. New here? You&apos;ll need an invite code.</p>
      <label htmlFor="invite">Invite code</label>
      <input
        id="invite"
        value={invite}
        autoComplete="off"
        onChange={(event) => {
          setInvite(event.target.value);
        }}
      />
      <label>
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
        <a href={startUrl("google", invite, age18)}>Continue with Google</a>
      </p>
      <p>
        <a href={startUrl("github", invite, age18)}>Continue with GitHub</a>
      </p>
    </>
  );
}
