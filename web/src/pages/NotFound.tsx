import { Link } from "react-router";
import { useSession } from "../session";

export function NotFound() {
  const { me } = useSession();
  return (
    <div className="notfound">
      <h1>Page not found</h1>
      <p className="lede">This address doesn&apos;t lead anywhere on RoleRank. It may have been mistyped, or the page has moved.</p>
      <Link className="button primary" to={me ? "/matches" : "/"}>
        {me ? "Go to your matches" : "Go to the home page"}
      </Link>
    </div>
  );
}
