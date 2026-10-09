import { useEffect, useState } from "react";
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
import { api, connect, logout } from "../api/client";
import type { ControlInfo } from "../api/contracts";

import { Workbench } from "../pages/Workbench";
import "./draft";
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
  const [session, setSession] = useState<"loading" | "ready" | "locked">(
    "loading",
  );
  const [error, setError] = useState("");
  const [theme, setTheme] = useState(
    () => localStorage.getItem("ding.theme") || "system",
  );
  const [timeMode, setTimeMode] = useState(
    localStorage.getItem("ding.time") || "local",
  );
  const client = useQueryClient();
  const location = useLocation();
  const establish = () => {
    sessionAttempt ??= connect();
    sessionAttempt
      .then(() => {
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
    const lock = () => setSession("locked");
    window.addEventListener("ding:unauthorized", lock);
    return () => window.removeEventListener("ding:unauthorized", lock);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("ding.theme", theme);
  }, [theme]);
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
            ding<span>.</span>
          </div>
          <p className="eyebrow">Your signals, explained</p>
          <h1>
            {session === "loading"
              ? "Connecting to Ding…"
              : "Open your console."}
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
          <div className="launch-instructions">
            <span>Run in your terminal</span>
            <code>ding ui</code>
            <p>
              The command opens a one-use link. Credentials stay on your
              machine.
            </p>
          </div>
          {session === "locked" && (
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
    <TimeContext.Provider value={timeMode}>
      <div className="app-shell">
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <aside className="sidebar">
          <NavLink to="/watches" className="brand" aria-label="Ding Console">
            ding<span>.</span>
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
            <span className={`status-dot ${info.isError ? "offline" : ""}`} />
            <span>
              {info.isError ? "Connection interrupted" : "Connected daemon"}
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
              <select
                aria-label="Display timezone"
                value={timeMode}
                onChange={(e) => {
                  setTimeMode(e.target.value);
                  localStorage.setItem("ding.time", e.target.value);
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
                  await logout();
                  client.clear();
                  setSession("locked");
                }}
              >
                <LogOut size={16} />
              </button>
            </div>
          </header>
          {info.isError && (
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
              {nav.slice(4).map((n) => (
                <Route
                  key={n.path}
                  path={n.path + "/*"}
                  element={<FoundationPage title={n.label} />}
                />
              ))}
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
  );
}
function FoundationPage({ title }: { title: string }) {
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Ding Console</p>
          <h1>{title}</h1>
        </div>
      </div>
      <div className="empty">
        <Radio size={30} />
        <h2>The console is connected.</h2>
        <p>This foundation is ready for the operational views.</p>
      </div>
    </>
  );
}
