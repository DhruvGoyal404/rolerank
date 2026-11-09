import { StrictMode } from "react";
import { createRoot, hydrateRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import { SessionProvider } from "./session";
import { applyStoredTheme } from "./theme";
import { ToastProvider } from "./toast";
import "./styles.css";

applyStoredTheme();
const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

const tree = (
  <StrictMode>
    <BrowserRouter>
      <SessionProvider>
        <ToastProvider>
          <App />
        </ToastProvider>
      </SessionProvider>
    </BrowserRouter>
  </StrictMode>
);

// A prerendered public page (scripts/prerender.mjs) hydrates. Any other route renders fresh; a
// server that answered it with another page's HTML (a dev or preview fallback) has that HTML
// cleared first.
if (root.dataset.prerendered === window.location.pathname) hydrateRoot(root, tree);
else {
  root.replaceChildren();
  delete root.dataset.prerendered;
  createRoot(root).render(tree);
}
