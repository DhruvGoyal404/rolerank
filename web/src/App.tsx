import { Link, Route, Routes } from "react-router";
import { AboutBot } from "./pages/AboutBot";
import { Home } from "./pages/Home";
import { SignIn } from "./pages/SignIn";

export function App() {
  return (
    <>
      <header>
        <nav aria-label="Main">
          <Link to="/">RoleRank</Link>
          <Link to="/signin">Sign in</Link>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/signin" element={<SignIn />} />
          <Route path="/about/bot" element={<AboutBot />} />
          <Route path="*" element={<h1>Page not found</h1>} />
        </Routes>
      </main>
      <footer>
        <Link to="/about/bot">About our bot</Link>
      </footer>
    </>
  );
}
