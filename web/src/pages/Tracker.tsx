import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router";
import { api, type TrackerItem } from "../api";
import { Loading } from "../components";
import { useSession } from "../session";

const STATUSES: Array<[string, string]> = [
  ["saved", "Saved"],
  ["applied", "Applied"],
  ["interviewing", "Interviewing"],
  ["offer", "Offer"],
  ["rejected", "Not this time"],
];

export function SaveButton({ jobKey }: { jobKey: string }) {
  const [state, setState] = useState<"idle" | "saving" | "saved" | "failed">("idle");
  return (
    <button
      type="button"
      disabled={state === "saving" || state === "saved"}
      aria-live="polite"
      onClick={() => {
        setState("saving");
        api("/api/tracker", { method: "POST", body: { key: jobKey } }).then(
          () => { setState("saved"); },
          () => { setState("failed"); },
        );
      }}
    >
      {state === "saved" ? "Saved to tracker" : state === "failed" ? "Couldn't save, retry" : "Save"}
    </button>
  );
}

export function Tracker() {
  const { me, loading } = useSession();
  const [items, setItems] = useState<TrackerItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (!me) return;
    api<{ items: TrackerItem[] }>("/api/tracker").then(
      (r) => { setItems(r.items); },
      () => { setError("We couldn't load your tracker."); },
    );
  }, [me]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;

  const setStatus = (item: TrackerItem, status: string) => {
    setItems((xs) => xs?.map((x) => (x.id === item.id ? { ...x, status } : x)) ?? null);
    api(`/api/tracker/${item.id}`, { method: "PATCH", body: { status } }).catch(() => { setError("That change didn't save. Please try again."); });
  };
  const remove = (item: TrackerItem) => {
    api(`/api/tracker/${item.id}`, { method: "DELETE" }).then(
      () => { setItems((xs) => xs?.filter((x) => x.id !== item.id) ?? null); },
      () => { setError("We couldn't remove that. Please try again."); },
    );
  };

  return (
    <>
      <h1>Jobs you&apos;re following</h1>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {!items && !error ? <Loading what="your tracker" /> : null}
      {items?.length === 0 ? (
        <p className="notice">
          Nothing saved yet. <Link to="/matches">Find a match</Link> and press Save.
        </p>
      ) : null}
      {items && items.length > 0 ? (
        <table className="tracker">
          <thead>
            <tr>
              <th scope="col">Job</th>
              <th scope="col">Status</th>
              <th scope="col">
                <span className="skip">Remove</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>
                  <Link to={`/jobs/${item.key}`}>{item.title}</Link>
                  <div className="fine">{item.company}</div>
                </td>
                <td>
                  <select aria-label={`Status of ${item.title}`} value={item.status} onChange={(e) => { setStatus(item, e.target.value); }}>
                    {STATUSES.map(([v, label]) => (
                      <option key={v} value={v}>
                        {label}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <button type="button" onClick={() => { remove(item); }} aria-label={`Remove ${item.title}`}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </>
  );
}
