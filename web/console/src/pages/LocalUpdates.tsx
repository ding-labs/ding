import type { ControlLocalUpdates } from "../api/contracts";
import { ErrorBox, Time, useRead } from "../components/common";

export function LocalUpdates() {
  const { data, error } = useRead<ControlLocalUpdates>("/local/updates", 60000);
  return <section className="panel">
    <h2>Updates</h2>
    <ErrorBox error={error} />
    {data && <>
      <p>{data.owner === "homebrew" ? "Homebrew manages this installation. Use brew upgrade ding." :
        !data.configured ? "This source build has no configured release channel. Local monitoring remains fully available." :
        data.check?.available ? `Version ${data.check.available} is available.` :
        data.check ? "No newer release was found at the last check." : "The first signed release check has not run yet."}</p>
      {data.check && <p className="muted">Last checked <Time value={data.check.checkedAt} /></p>}
      {data.check?.error && <p>{data.check.error}</p>}
      <p>{data.settings.checks ? "Signed release checks are enabled, at most once daily." : "Online update checks are disabled."}{" "}
        {data.settings.automatic ? `Automatic compatible updates are enabled during ${String(data.settings.hourUTC).padStart(2, "0")}:00–${String(data.settings.hourUTC).padStart(2, "0")}:59 UTC.` : "Automatic installation is off."}</p>
      {data.configured && data.owner === "standalone" && <p>Run <code>ding update check</code> to inspect the release. Install with <code>ding update install --yes</code>; a backup and brief service restart are required. Windows uses its signed installer.</p>}
      <p className="muted">Change preferences with <code>ding update configure --help</code>. Local monitoring works without update checks.</p>
    </>}
  </section>;
}
