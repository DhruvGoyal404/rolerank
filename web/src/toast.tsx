import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";

/**
 * One toast at a time, announced politely (DESIGN.md, Toasts). Used to confirm optimistic actions,
 * with Undo where undoing is safe. Errors stay until dismissed; everything else leaves after 4 s.
 */
interface Toast {
  id: number;
  text: string;
  undo?: (() => void) | undefined;
  error?: boolean | undefined;
}

type Show = (text: string, options?: { undo?: () => void; error?: boolean }) => void;

const ToastContext = createContext<Show>(() => undefined);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<Toast | null>(null);
  const next = useRef(0);
  const show = useCallback<Show>((text, options) => {
    next.current += 1;
    setToast({ id: next.current, text, undo: options?.undo, error: options?.error });
  }, []);
  useEffect(() => {
    if (!toast || toast.error) return;
    const timer = setTimeout(() => {
      setToast((t) => (t?.id === toast.id ? null : t));
    }, 4000);
    return () => {
      clearTimeout(timer);
    };
  }, [toast]);
  return (
    <ToastContext.Provider value={show}>
      {children}
      <div className="toast-region" role="status" aria-live="polite">
        {toast ? (
          <div className="toast" key={toast.id}>
            <p>{toast.text}</p>
            {toast.undo ? (
              <button
                type="button"
                onClick={() => {
                  toast.undo?.();
                  setToast(null);
                }}
              >
                Undo
              </button>
            ) : null}
            <button
              type="button"
              onClick={() => {
                setToast(null);
              }}
            >
              Dismiss
            </button>
          </div>
        ) : null}
      </div>
    </ToastContext.Provider>
  );
}

export const useToast = (): Show => useContext(ToastContext);
