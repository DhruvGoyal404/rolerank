import { useEffect, useState } from "react";
import { Link, NavLink, Route, Routes } from "react-router";
import { api } from "./api";
import { AboutBot } from "./pages/AboutBot";
import { Account } from "./pages/Account";
import { BringJD } from "./pages/BringJD";
import { Home } from "./pages/Home";
import { Job } from "./pages/Job";
import { Matches } from "./pages/Matches";
import { Privacy } from "./pages/Privacy";
import { Resume } from "./pages/Resume";
import { SignIn } from "./pages/SignIn";
import { Sources } from "./pages/Sources";
import { Tracker } from "./pages/Tracker";
import { useSession } from "./session";
import { currentTheme, setTheme, type Theme } from "./theme";

/** Names the action, not the state: "Dark theme" switches to dark. Follows the system until picked. */
function ThemeToggle() {
  const [theme, set] = useState<Theme>(currentTheme);
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const follow = () => {
      set(currentTheme());
    };
    media.addEventListener("change", follow);
    return () => {
      media.removeEventListener("change", follow);
    };
  }, []);
  const next: Theme = theme === "dark" ? "light" : "dark";
  return (
    <button
      type="button"
      className="linklike"
      aria-label={`Switch to ${next} theme`}
      onClick={() => {
        setTheme(next);
        set(next);
      }}
    >
      {next === "dark" ? "☾ Dark theme" : "☀ Light theme"}
    </button>
  );
}

export function App() {
  const { me, clear } = useSession();
  const signOut = async () => {
    await api("/api/auth/logout", { method: "POST" }).catch(() => undefined);
    // A full load of home: no page's redirect can race it, and nothing from the session stays in memory.
    clear();
    window.location.assign("/");
  };
  return (
    <>
      <a className="skip" href="#main">
        Skip to content
      </a>
      <header className="topbar">
        <div className="topbar-inner">
          <Link className="brand" to="/">
            <img src="/icons/favicon-48.png" alt="" width="30" height="30" />
            RoleRank
          </Link>
          <nav className="nav" aria-label="Main">
            {me ? (
              <>
                <NavLink to="/matches">Matches</NavLink>
                <NavLink to="/jd">Bring a JD</NavLink>
                <NavLink to="/tracker">Tracker</NavLink>
                <NavLink to="/resume">Resume</NavLink>
                <NavLink to="/account">Account</NavLink>
                <button type="button" className="linklike" onClick={() => void signOut()}>
                  Sign out
                </button>
              </>
            ) : (
              <NavLink to="/signin">Sign in</NavLink>
            )}
            <ThemeToggle />
          </nav>
        </div>
      </header>
      <main id="main">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/signin" element={<SignIn />} />
          <Route path="/resume" element={<Resume />} />
          <Route path="/matches" element={<Matches />} />
          <Route path="/jobs/:key" element={<Job />} />
          <Route path="/jd" element={<BringJD />} />
          <Route path="/tracker" element={<Tracker />} />
          <Route path="/account" element={<Account />} />
          <Route path="/about/bot" element={<AboutBot />} />
          <Route path="/sources" element={<Sources />} />
          <Route path="/privacy" element={<Privacy />} />
          <Route path="*" element={<h1>Page not found</h1>} />
        </Routes>
      </main>
      <footer className="site">
        <div className="inner">
          <Link to="/sources">Sources and opt-out</Link>
          <Link to="/about/bot">About our bot</Link>
          <Link to="/privacy">Privacy</Link>
        </div>
      </footer>
    </>
  );
}
