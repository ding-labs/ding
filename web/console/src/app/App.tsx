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
import { api, connect, logout } from "../api/client";
import type { ControlInfo } from "../api/contracts";

import { preference, savePreference, setPreferenceWorkspace } from "./preferences";
import { ExecutionContext, discoverCloud, type Execution } from "./execution";
import { CommandMenu } from "../components/CommandMenu";
import { System } from "../pages/System";
import { Workbench } from "../pages/Workbench";
import { FirstWatch } from "../pages/FirstWatch";
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
    sessionAttempt ??= connect();
    sessionAttempt
      .then((details) => {
        const next = details.workspace || "";
        if (workspace.current !== next) { client.clear(); resetDraft(); }
        workspace.current = next;
        setPreferenceWorkspace(next);
        setExecution({ mode: details.execution === "cloud" ? "cloud" : "local", workspace: next });
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
    void discoverCloud().then(cloud => { if (cloud) setExecution(current => ({ ...current, mode: "cloud" })); });
    establish();
    const lock = () => {
      client.clear();
      if (workspace.current) resetDraft();
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
              : execution.mode === "cloud" ? "Your watches, always on." : "Open your console."}
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
          {execution.mode === "cloud" ? <div className="launch-instructions">
            <p>Sign in with GitHub when you choose hosted execution. No credit card or onboarding questionnaire. Local Ding remains free and account-free.</p>
            <a className="button primary" href="/auth/login">Continue with GitHub <ArrowRight size={16} /></a>
          </div> : <div className="launch-instructions">
            <span>Run in your terminal</span>
            <code>ding ui</code>
            <p>
              The command opens a one-use link. Credentials stay on your
              machine.
            </p>
          </div>}
          {session === "locked" && execution.mode === "local" && (
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
            {nav.map((n) => (
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
                : "Connected daemon"}
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
            <div className="toolbar">
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
                    client.clear();
                    if (workspace.current) resetDraft();
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
          <main id="main" className="page">
            <Routes>
              <Route path="/" element={<Navigate to="/watches" replace />} />
              <Route path="/watches" element={<Watches />} />
              <Route path="/watches/:id" element={<WatchDetail />} />
              <Route path="/events" element={<Events />} />
              <Route path="/events/:id" element={<EventDetail />} />
              <Route path="/deliveries" element={<Deliveries />} />
              <Route path="/deliveries/:id" element={<DeliveryDetail />} />
              <Route path="/workbench" element={<Workbench />} />
              <Route path="/start" element={<FirstWatch />} />
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
