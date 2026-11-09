import { useEffect, useState } from "react";
import { Link, NavLink, Route, Routes, useLocation } from "react-router";
import { api } from "./api";
import { Logo, Moon, Sun } from "./icons";
import { AboutBot } from "./pages/AboutBot";
import { Account } from "./pages/Account";
import { BringJD } from "./pages/BringJD";
import { Home } from "./pages/Home";
import { Job } from "./pages/Job";
import { Matches } from "./pages/Matches";
import { NotFound } from "./pages/NotFound";
import { Privacy } from "./pages/Privacy";
import { Resume } from "./pages/Resume";
import { SignIn } from "./pages/SignIn";
import { Sources } from "./pages/Sources";
import { Tracker } from "./pages/Tracker";
import { useSession } from "./session";
import { currentTheme, setTheme, type Theme } from "./theme";

/** Names the action, not the state: "Switch to dark theme". Follows the system until picked. */
function ThemeToggle() {
  const [theme, set] = useState<Theme | null>(null);
  useEffect(() => {
    set(currentTheme());
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
      className="icon-button"
      aria-label={`Switch to ${next} theme`}
      title={`Switch to ${next} theme`}
      onClick={() => {
        setTheme(next);
        set(next);
      }}
    >
      {next === "dark" ? <Moon /> : <Sun />}
    </button>
  );
}

export function App() {
  const { me, clear } = useSession();
  const { pathname } = useLocation();
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
            <Logo />
            RoleRank
          </Link>
          {me ? (
            <nav className="nav" aria-label="Main">
              <NavLink to="/matches">Matches</NavLink>
              <NavLink to="/jd">Bring a JD</NavLink>
              <NavLink to="/tracker">Tracker</NavLink>
              <NavLink to="/resume">Resume</NavLink>
              <button type="button" className="linklike compact nav-signout" onClick={() => void signOut()}>
                Sign out
              </button>
            </nav>
          ) : null}
          <div className="nav-end">
            {me ? (
              <>
                <NavLink to="/account">Account</NavLink>
                <button type="button" className="linklike compact bar-signout" onClick={() => void signOut()}>
                  Sign out
                </button>
              </>
            ) : (
              <NavLink to="/signin">Sign in</NavLink>
            )}
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main id="main" tabIndex={-1}>
        <div className="route" key={pathname}>
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
            <Route path="*" element={<NotFound />} />
          </Routes>
        </div>
      </main>
      <footer className="site">
        <div className="inner">
          <div>
            <Link className="brand" to="/">
              <Logo />
              RoleRank
            </Link>
            <p>Real jobs from public, legal sources, ranked against your resume. Every match explained, nothing invented.</p>
          </div>
          <nav aria-label="About RoleRank">
            <ul>
              <li>
                <Link to="/sources">Sources and opt-out</Link>
              </li>
              <li>
                <Link to="/about/bot">About our bot</Link>
              </li>
              <li>
                <Link to="/privacy">Privacy</Link>
              </li>
            </ul>
          </nav>
        </div>
      </footer>
    </>
  );
}
