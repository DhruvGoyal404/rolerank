import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router";
import { api, type TrackerItem } from "../api";
import { Loading } from "../components";
import { useSession } from "../session";
import { useToast } from "../toast";

const STATUSES: Array<[string, string]> = [
  ["saved", "Saved"],
  ["applied", "Applied"],
  ["interviewing", "Interviewing"],
  ["offer", "Offer"],
  ["rejected", "Not this time"],
];

const label = (status: string) => STATUSES.find(([v]) => v === status)?.[1] ?? status;

export function SaveButton({ jobKey, compact = true }: { jobKey: string; compact?: boolean }) {
  const [state, setState] = useState<"idle" | "saving" | "saved" | "failed">("idle");
  const toast = useToast();
  return (
    <button
      type="button"
      className={compact ? "compact" : undefined}
      disabled={state === "saving" || state === "saved"}
      aria-busy={state === "saving"}
      onClick={() => {
        setState("saving");
        api("/api/tracker", { method: "POST", body: { key: jobKey } }).then(
          () => {
            setState("saved");
            toast("Saved to your tracker.");
          },
          () => {
            setState("failed");
          },
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
  const toast = useToast();
  useEffect(() => {
    if (!me) return;
    api<{ items: TrackerItem[] }>("/api/tracker").then(
      (r) => { setItems(r.items); },
      () => { setError("We couldn't load your tracker."); },
    );
  }, [me]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;

  // Optimistic: the change shows at once and rolls back if the server refuses it.
  const setStatus = (item: TrackerItem, status: string, announce = true) => {
    const before = item.status;
    setItems((xs) => xs?.map((x) => (x.id === item.id ? { ...x, status } : x)) ?? null);
    api(`/api/tracker/${item.id}`, { method: "PATCH", body: { status } }).then(
      () => {
        if (announce)
          toast(`Marked ${label(status).toLowerCase()}.`, {
            undo: () => {
              setStatus({ ...item, status }, before, false);
            },
          });
      },
      () => {
        setItems((xs) => xs?.map((x) => (x.id === item.id ? { ...x, status: before } : x)) ?? null);
        setError("That change didn't save. Please try again.");
      },
    );
  };
  const remove = (item: TrackerItem) => {
    api(`/api/tracker/${item.id}`, { method: "DELETE" }).then(
      () => {
        setItems((xs) => xs?.filter((x) => x.id !== item.id) ?? null);
        toast(`Removed ${item.title}.`, {
          undo: () => {
            void (async () => {
              try {
                const back = await api<TrackerItem>("/api/tracker", { method: "POST", body: { key: item.key } });
                if (back.status !== item.status) await api(`/api/tracker/${back.id}`, { method: "PATCH", body: { status: item.status } });
                setItems((await api<{ items: TrackerItem[] }>("/api/tracker")).items);
              } catch {
                setError("We couldn't put that back. Save it again from your matches.");
              }
            })();
          },
        });
      },
      () => { setError("We couldn't remove that. Please try again."); },
    );
  };

  return (
    <>
      <header className="page-head">
        <h1>Jobs you&apos;re following</h1>
        <p className="lede">Everything you&apos;ve saved, and where each application stands.</p>
      </header>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {!items && !error ? <Loading what="your tracker" /> : null}
      {items?.length === 0 ? (
        <div className="state">
          <h2>Nothing saved yet.</h2>
          <p>Save a role from your matches and it appears here, ready to track from applied to offer.</p>
          <Link className="button primary" to="/matches">
            Find a match
          </Link>
        </div>
      ) : null}
      {items && items.length > 0 ? (
        <ul className="status-summary" aria-label="Summary">
          {STATUSES.map(([v, name]) => (
            <li key={v}>
              <strong>{items.filter((x) => x.status === v).length}</strong>
              {name}
            </li>
          ))}
        </ul>
      ) : null}
      {items && items.length > 0 ? (
        <table className="tracker">
          <thead>
            <tr>
              <th scope="col">Job</th>
              <th scope="col">Status</th>
              <th scope="col">
                <span className="sr-only">Remove</span>
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
                  <button type="button" className="quiet compact" onClick={() => { remove(item); }} aria-label={`Remove ${item.title}`}>
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
