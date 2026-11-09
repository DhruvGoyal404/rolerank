import type { ReactNode } from "react";

/** A reading page: a contents rail beside a 42rem column (DESIGN.md, Pages). */
export function Reading({ sections, children }: { sections: Array<[id: string, title: string]>; children: ReactNode }) {
  return (
    <div className="reading">
      <nav className="toc" aria-labelledby="toc-title">
        <h2 id="toc-title">On this page</h2>
        <ul>
          {sections.map(([id, title]) => (
            <li key={id}>
              <a href={`#${id}`}>{title}</a>
            </li>
          ))}
        </ul>
      </nav>
      <article>{children}</article>
    </div>
  );
}
