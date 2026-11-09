/**
 * Light and dark themes. The system setting decides until the user picks one; the pick is a
 * per-browser convenience, kept in localStorage, and the page works without it.
 */
export type Theme = "light" | "dark";
const KEY = "rolerank.theme";

function stored(): Theme | null {
  try {
    const v = localStorage.getItem(KEY);
    return v === "light" || v === "dark" ? v : null;
  } catch {
    return null;
  }
}

export function currentTheme(): Theme {
  return stored() ?? (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
}

export function setTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme;
  for (const meta of document.querySelectorAll('meta[name="theme-color"]')) meta.setAttribute("content", theme === "dark" ? "#0b1022" : "#f6f7fb");
  try {
    localStorage.setItem(KEY, theme);
  } catch {
    // private windows may refuse storage; the theme still applies for this visit
  }
}

/** Applies a stored pick at start-up; without one, CSS follows the system. */
export function applyStoredTheme(): void {
  const t = stored();
  if (t) document.documentElement.dataset.theme = t;
}
