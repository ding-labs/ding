import { useEffect, useRef, useState } from "react";
import bell from "../../../../design/assets/mark.svg";
import {
  NavLink,
  Navigate,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  ArrowRight,
  LogOut,
  Monitor,
  Radio,
  Send,
  SlidersHorizontal,
  SquareTerminal,
} from "lucide-react";
import { useConnection } from "../api/connection";
import { api, connect, logout, selectWorkspace, selectIdentity, forgetSession } from "../api/client";
import type { ControlInfo, ControlBrowserSession } from "../api/contracts";

import { preference, savePreference, setPreferenceWorkspace } from "./preferences";
import { ExecutionContext, discoverAuth, type AuthConfig, type Execution } from "./execution";
import { SignIn } from "../components/SignIn";
import { CommandMenu } from "../components/CommandMenu";
import { System } from "../pages/System";
import { Workbench } from "../pages/Workbench";
import { FirstWatch } from "../pages/FirstWatch";
import { CloudFirstWatch } from "../pages/CloudFirstWatch";
import { resetDraft } from "./draft";
import { useNavigationContext } from "./scroll";
import { Watches, WatchDetail } from "../pages/Watches";
import { Events, EventDetail } from "../pages/Events";
import { Deliveries, DeliveryDetail } from "../pages/Deliveries";
import { TimeContext } from "../components/common";

const nav = [
  { path: "/watches", label: "Watches", icon: Radio },
  { path: "/events", label: "Events", icon: Activity },
  { path: "/deliveries", label: "Deliveries", icon: Send },
  { path: "/workbench", label: "Workbench", icon: SquareTerminal },
  { path: "/system", label: "System", icon: SlidersHorizontal },
];
// Module-level promise makes the single-use exchange safe under StrictMode.
let sessionAttempt: ReturnType<typeof connect> | undefined;
export function App() {
  useNavigationContext();
  const connected = useConnection();
  const [execution, setExecution] = useState<Execution>({ mode: "local" });
  const workspace = useRef("");
  const accessMode = useRef<Execution["mode"]>("local");
  const [auth, setAuth] = useState<AuthConfig>();
  const [memberships, setMemberships] = useState<ControlBrowserSession["memberships"]>([]);
  const [density, setDensity] = useState(
    preference("ding.density", "comfortable"),
  );
  const [session, setSession] = useState<"loading" | "ready" | "locked">(
    "loading",
  );
  const [error, setError] = useState("");
  const [theme, setTheme] = useState(() => preference("ding.theme", "system"));
  const [timeMode, setTimeMode] = useState(preference("ding.time", "local"));
  const client = useQueryClient();
  const location = useLocation();
  const establish = () => {
    sessionAttempt ??= discoverAuth().then(config => { setAuth(config); accessMode.current = config.execution; setExecution(current => ({ ...current, mode: config.execution })); return connect(); });
    sessionAttempt
      .then((details) => {
        const back = new URLSearchParams(window.location.search).get("return");
        if (back && /^\/connect\/[a-f0-9]{64}$/.test(back)) { window.location.assign(back); return; }
        const next = `${window.location.origin}:${details.user?.id || "personal"}:${details.workspace || ""}`;
        if (workspace.current !== next) { client.clear(); resetDraft(); }
        workspace.current = next;
        setPreferenceWorkspace(next);
        setExecution({ mode: details.execution === "cloud" ? "cloud" : details.execution === "shared" ? "shared" : "local", workspace: details.workspace, user: details.user, role: details.role, capabilities: details.capabilities, accountFeatures: details.accountFeatures });
        selectIdentity(details.user?.id || "");
        selectWorkspace(details.workspace || "");
        setMemberships(details.memberships);
        setTheme(preference("ding.theme", "system")); setDensity(preference("ding.density", "comfortable")); setTimeMode(preference("ding.time", "local"));
        setSession("ready");
        setError("");
      })
      .catch((e) => {
        setError(e.message);
        setSession("locked");
      })
      .finally(() => {
        sessionAttempt = undefined;
      });
  };
  useEffect(() => {
    establish();
    const lock = () => {
      forgetSession();
      void client.cancelQueries();
      client.clear();
      if (workspace.current && accessMode.current !== "local") resetDraft();
      setSession("locked");
    };
    window.addEventListener("ding:unauthorized", lock);
    return () => window.removeEventListener("ding:unauthorized", lock);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.dataset.density = density;
    savePreference("ding.density", density);
    savePreference("ding.theme", theme);
  }, [theme, density]);
  const info = useQuery({
    queryKey: ["info"],
    queryFn: ({ signal }) => api<ControlInfo>("/info", { signal }),
    enabled: session === "ready",
    refetchInterval: 15000,
    refetchIntervalInBackground: false,
  });
  const current = nav.find((n) => location.pathname.startsWith(n.path));
  if (session !== "ready")
    return (
      <main className="connect-page">
        <div className="connect-card">
          <div className="brand">
            <img className="brand-bell" src={bell} width="36" height="36" alt="" /><strong>Ding<span>.</span></strong>
          </div>
          <p className="eyebrow">Your signals, explained</p>
          <h1>
            {session === "loading"
              ? "Connecting to Ding…"
              : auth?.access === "shared" ? `Sign in to ${auth.name || "Ding"}.` : "Open your console."}
          </h1>
          <p>
            Inspect your watches, follow the evidence, and see what happened to
            every notification.
          </p>
          {error && (
            <div className="notice" role="alert">
              {error}
            </div>
          )}
          {auth && <SignIn config={auth} connected={establish} />}
          {session === "locked" && (!auth || auth.access === "personal") && (
            <button
              className="button primary"
              onClick={() => {
                sessionAttempt = undefined;
                establish();
              }}
            >
              Reconnect <ArrowRight size={16} />
            </button>
          )}
        </div>
      </main>
    );
  return (
    <ExecutionContext.Provider value={execution}>
    <TimeContext.Provider value={timeMode}>
      <div className="app-shell">
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <aside className="sidebar">
          <NavLink to="/watches" className="brand" aria-label="Ding Console">
            <img className="brand-bell" src={bell} width="36" height="36" alt="" /><strong>Ding<span>.</span></strong>
          </NavLink>
          <nav aria-label="Main navigation">
            {nav.filter(n => n.path !== "/workbench" || execution.mode === "local" || execution.capabilities?.includes("preview")).map((n) => (
              <NavLink
                key={n.path}
                to={n.path}
                className={({ isActive }) =>
                  isActive ? "nav-link active" : "nav-link"
                }
              >
                <n.icon size={17} />
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="instance">
            <span
              className={`status-dot ${info.isError || !connected ? "offline" : ""}`}
            />
            <span>
              {info.isError || !connected
                ? "Connection interrupted"
                : execution.mode === "cloud" ? "Runs in Ding Cloud" : execution.mode === "shared" ? `Runs on ${auth?.name || "this server"}` : "Runs on this computer"}
            </span>
            <code>{info.data?.listen || window.location.host}</code>
            <small>{info.data?.version || "Ding"}</small>
          </div>
        </aside>
        <div className="workspace">
          <header className="location-bar">
            <div>
              Console <span>/</span> {current?.label || "Watches"}
            </div>
            <div className={execution.mode === "local" ? "toolbar" : "toolbar shared-toolbar"}>
              {execution.mode !== "local" && <>
                <span>{execution.user?.name} · {execution.role}</span>
                <select aria-label="Workspace" value={execution.workspace || ""} onChange={async e => {
                  const next = e.target.value; setSession("loading");
                  await client.cancelQueries(); selectWorkspace(next); client.clear(); resetDraft(); sessionAttempt = undefined; establish();
                }}>{memberships?.map(m => <option key={m.workspace} value={m.workspace}>{m.name}</option>)}</select>
              </>}
              <CommandMenu />
              <select
                aria-label="Display density"
                value={density}
                onChange={(e) => setDensity(e.target.value)}
              >
                <option value="comfortable">Comfortable</option>
                <option value="compact">Compact</option>
              </select>
              <select
                aria-label="Display timezone"
                value={timeMode}
                onChange={(e) => {
                  setTimeMode(e.target.value);
                  savePreference("ding.time", e.target.value);
                }}
              >
                <option value="local">Local time</option>
                <option value="utc">UTC</option>
                <option value="relative">Relative time</option>
              </select>
              <label className="theme-select">
                <Monitor size={15} />
                <select
                  aria-label="Appearance"
                  value={theme}
                  onChange={(e) => setTheme(e.target.value)}
                >
                  <option value="system">System</option>
                  <option value="light">Light</option>
                  <option value="dark">Dark</option>
                </select>
              </label>
              <button
                className="icon-button"
                aria-label="Log out"
                onClick={async () => {
                  try {
                    await logout();
                    await client.cancelQueries(); client.clear();
                    if (workspace.current && accessMode.current !== "local") resetDraft();
                    setSession("locked");
                  } catch (e) {
                    setError((e as Error).message);
                  }
                }}
              >
                <LogOut size={16} />
              </button>
            </div>
          </header>
          {error && (
            <div className="connection-warning" role="alert">
              {error}
            </div>
          )}
          <div className="mobile-instance">
            <span
              className={`status-dot ${info.isError || !connected ? "offline" : ""}`}
            />
            <code>{info.data?.listen || window.location.host}</code>
          </div>
          {(info.isError || !connected) && (
            <div className="connection-warning" role="status">
              Connection interrupted. Displayed data may be stale. Last
              connected{" "}
              {info.dataUpdatedAt
                ? new Date(info.dataUpdatedAt).toLocaleTimeString()
                : "unknown"}
              .
            </div>
          )}
          <main id="main" className="page" key={workspace.current}>
            <Routes>
              <Route path="/" element={<Navigate to="/watches" replace />} />
              <Route path="/watches" element={<Watches />} />
              <Route path="/watches/:id" element={<WatchDetail />} />
              <Route path="/events" element={<Events />} />
              <Route path="/events/:id" element={<EventDetail />} />
              <Route path="/deliveries" element={<Deliveries />} />
              <Route path="/deliveries/:id" element={<DeliveryDetail />} />
              <Route path="/workbench" element={execution.mode === "local" || execution.capabilities?.includes("preview") ? <Workbench /> : <Navigate to="/watches" replace />} />
              <Route path="/start" element={execution.mode === "cloud" ? <CloudFirstWatch /> : execution.mode === "shared" ? <Workbench /> : <FirstWatch />} />
              <Route path="/system" element={<System />} />
              <Route
                path="*"
                element={
                  <div className="empty">
                    <h1>Page not found</h1>
                    <NavLink to="/watches">Return to watches</NavLink>
                  </div>
                }
              />
            </Routes>
          </main>
        </div>
      </div>
    </TimeContext.Provider>
    </ExecutionContext.Provider>
  );
}
