import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { AuthConfig } from "../app/execution";

// Capture once before React mounts, then remove the one-use secret from history.
let activation = new URLSearchParams(location.hash.slice(1)).get("activate") || "";
if (activation) history.replaceState(null, "", location.pathname + location.search);
export function SignIn({ config, connected }: { config: AuthConfig; connected: () => void }) {
  const [activate, setActivate] = useState(activation);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault(); setBusy(true); setMessage("");
    try {
      await api(activate ? "/auth/activate" : "/auth/password", { method: "POST", body: activate ? { token: activate, password } : { username, password } });
      setPassword("");
      if (activate) { activation = ""; setActivate(""); setMessage("Password saved. Sign in with your instance username."); }
      else {
        const next = new URLSearchParams(location.search).get("return");
        if (next && /^\/connect\/[a-f0-9]{64}$/.test(next)) location.assign(next);
        else connected();
      }
    } catch (e) { setMessage((e as Error).message); }
    finally { setBusy(false); }
  }
  return <div className="launch-instructions">
    {message && <p role="status">{message}</p>}
    {config.methods.some(m => m.id === "password") && <form className="onboarding-form" onSubmit={submit}><fieldset disabled={busy}>
      {activate ? <p>Choose a password for your account.</p> : <label>Username<input required autoComplete="username" maxLength={64} value={username} onChange={e => setUsername(e.target.value)} /></label>}
      <label>{activate ? "New password" : "Password"}<input required type="password" minLength={activate ? 12 : undefined} maxLength={1024} autoComplete={activate ? "new-password" : "current-password"} value={password} onChange={e => setPassword(e.target.value)} /></label>
      <button className="button primary" type="submit">{busy ? "Please wait…" : activate ? "Set password" : "Sign in"}</button>
    </fieldset></form>}
    {!activate && config.methods.filter(m => m.id === "oidc" && m.url?.startsWith("/auth/")).map(m => <a key={m.id} className="button primary" href={m.url}>{m.label}</a>)}
    {config.access === "personal" && <><span>Run in your terminal</span><code>ding ui</code><p>The command opens a one-use link. Credentials stay on your machine.</p></>}
  </div>;
}
