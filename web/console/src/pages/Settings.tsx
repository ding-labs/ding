import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { ControlSessionUser } from "../api/contracts";
import { useCapability, useExecution } from "../app/execution";
import { ErrorBox, Heading } from "../components/common";
import type { Preference } from "../components/CommandMenu";

function NameSetting({
  title,
  label,
  description,
  initial,
  saveLabel,
  onSave,
}: {
  title: string;
  label: string;
  description: string;
  initial: string;
  saveLabel: string;
  onSave: (name: string) => Promise<string>;
}) {
  const [name, setName] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<unknown>();
  const id = `settings-${title.toLowerCase()}`;
  useEffect(() => {
    setName(initial);
  }, [initial]);
  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(undefined);
    setSaved(false);
    try {
      setName(await onSave(name.trim()));
      setSaved(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel">
      <h2>{title}</h2>
      <p className="muted" id={`${id}-description`}>
        {description}
      </p>
      <form className="onboarding-form" onSubmit={save}>
        <fieldset disabled={busy}>
          <label htmlFor={id}>
            {label}
            <input
              id={id}
              value={name}
              required
              maxLength={100}
              autoComplete={title === "Profile" ? "nickname" : "off"}
              aria-describedby={`${id}-description`}
              placeholder={title === "Profile" ? "Your name" : "Workspace name"}
              onChange={(e) => {
                setName(e.target.value);
                setSaved(false);
              }}
            />
          </label>
          <div className="settings-save">
            <button
              className="button primary"
              disabled={busy || !name.trim() || name.trim() === initial}
            >
              {busy ? "Saving…" : saveLabel}
            </button>
            {saved && <span role="status">{title} saved.</span>}
          </div>
        </fieldset>
        <ErrorBox error={error} />
      </form>
    </section>
  );
}

export function Settings({
  workspaceName,
  preferences,
  onProfileSaved,
  onWorkspaceSaved,
}: {
  workspaceName: string;
  preferences: Preference[];
  onProfileSaved: (user: ControlSessionUser) => void;
  onWorkspaceSaved: (workspace: { id: string; name: string }) => void;
}) {
  const execution = useExecution();
  const owner = useCapability("owners");
  const query = useQueryClient();
  return (
    <>
      <Heading
        eyebrow="Your account"
        title="Settings"
        description={
          execution.mode === "local"
            ? "Choose how Ding looks and displays time."
            : "Manage your profile, workspace, and display preferences."
        }
      />
      <div className="settings-layout">
        {execution.mode !== "local" && (
          <>
            <NameSetting
              title="Profile"
              label="Display name"
              initial={execution.user?.name || ""}
              description="The name shown in your account menu and to workspace members."
              saveLabel="Save profile"
              onSave={async (name) => {
                const user = await api<ControlSessionUser>("/user/profile", {
                  method: "PATCH",
                  body: { name },
                });
                onProfileSaved(user);
                void query.invalidateQueries({ queryKey: ["/members"] });
                return user.name;
              }}
            />
            {owner ? (
              <NameSetting
                title="Workspace"
                label="Workspace name"
                initial={workspaceName}
                description="The name shown to everyone in this workspace."
                saveLabel="Save workspace"
                onSave={async (name) => {
                  const workspace = await api<{ id: string; name: string }>(
                    "/workspace",
                    { method: "PATCH", body: { name } },
                  );
                  onWorkspaceSaved(workspace);
                  return workspace.name;
                }}
              />
            ) : (
              <section className="panel">
                <h2>Workspace</h2>
                <p>{workspaceName}</p>
                <p className="muted">
                  Ask a workspace owner to change this name.
                </p>
              </section>
            )}
          </>
        )}
        <section className="panel">
          <h2>Preferences</h2>
          <p className="muted">
            Appearance and display settings for this browser and workspace.
          </p>
          <div className="settings-preferences">
            {preferences.map((p) => (
              <label
                className="stack-label"
                key={p.id}
                htmlFor={`settings-${p.id}`}
              >
                {p.label}
                <select
                  id={`settings-${p.id}`}
                  value={p.value}
                  onChange={(e) => p.onChange(e.target.value)}
                >
                  {p.options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          </div>
        </section>
        {execution.mode !== "local" && (
          <section className="panel">
            <h2>Sign-in and sessions</h2>
            <p className="muted">
              Review your active sessions and account security.
            </p>
            <Link className="button" to="/system?tab=Your+account">
              Manage sign-in and sessions
            </Link>
          </section>
        )}
      </div>
    </>
  );
}
