import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, ApiError, type Me } from "./api";

/** Who is signed in, and where they are: no resume yet, a draft profile, or a confirmed one. */
interface Session {
  me: Me | null; // null: signed out
  loading: boolean;
  refresh: () => Promise<void>;
  /** Forget the session at once, for after sign-out or deletion. */
  clear: () => void;
}

const SessionContext = createContext<Session>({ me: null, loading: true, refresh: () => Promise.resolve(), clear: () => undefined });

export function SessionProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const refresh = useCallback(async () => {
    try {
      setMe(await api<Me>("/api/me"));
    } catch (error) {
      if (!(error instanceof ApiError)) throw error;
      setMe(null);
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void refresh();
  }, [refresh]);
  const clear = useCallback(() => {
    setMe(null);
  }, []);
  return <SessionContext value={{ me, loading, refresh, clear }}>{children}</SessionContext>;
}

export const useSession = (): Session => useContext(SessionContext);
