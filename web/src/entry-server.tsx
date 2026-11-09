import { StrictMode } from "react";
import { renderToString } from "react-dom/server";
import { StaticRouter } from "react-router";
import { App } from "./App";
import { SessionProvider } from "./session";
import { ToastProvider } from "./toast";

/**
 * Renders a public page to HTML at build time (scripts/prerender.mjs), so crawlers read the real
 * page and its first paint needs no JavaScript. The tree is the client's, signed out and still
 * loading the session, which is exactly what the browser renders first, so it hydrates cleanly.
 */
export function render(url: string): string {
  return renderToString(
    <StrictMode>
      <StaticRouter location={url}>
        <SessionProvider>
          <ToastProvider>
            <App />
          </ToastProvider>
        </SessionProvider>
      </StaticRouter>
    </StrictMode>,
  );
}
