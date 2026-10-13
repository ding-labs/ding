import { useContext, useState } from "react";
import { CircleHelp, Plus } from "lucide-react";
import {
  ServiceCredentialDetails,
  permissionNames,
  type Service,
} from "./ServiceCredentialDetails";
import { useExecution } from "../app/execution";
import { ErrorBox, TimeContext, useRead } from "../components/common";
import {
  CreateCredential,
  CredentialSecret,
  type IssuedCredential,
} from "./ServiceCredentialDialogs";

function CredentialExpiry({ value }: { value: string }) {
  const mode = useContext(TimeContext);
  const date = new Date(value);
  if (Number.isNaN(date.getTime()))
    return <span className="muted">Not available</span>;
  const remaining = date.getTime() - Date.now();
  const days = Math.ceil(remaining / 86400000);
  const relative =
    remaining <= 0
      ? "Expired"
      : days === 1
        ? "Within a day"
        : `In ${days} days`;
  const calendar = date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    ...(mode === "utc" ? { timeZone: "UTC" } : {}),
  });
  return (
    <>
      <time
        dateTime={value}
        title={`${date.toLocaleString()} · ${date.toISOString()}`}
      >
        {mode === "relative" ? relative : calendar}
      </time>
      <small className={remaining <= 0 ? "service-expired" : "muted"}>
        {mode === "relative" ? calendar : relative}
      </small>
    </>
  );
}

export function Services() {
  const execution = useExecution();
  const info = useRead<{
    push?: { supported: boolean; admissionOpen: boolean };
  }>("/info");
  const ingest =
    execution.mode === "shared" || Boolean(info.data?.push?.supported);
  const list = useRead<Service[]>("/services");
  const [creating, setCreating] = useState(false);
  const [issued, setIssued] = useState<IssuedCredential>();
  const [selectedID, setSelectedID] = useState<string>();
  const selected = list.data?.find((service) => service.id === selectedID);
  return (
    <section className="services" aria-labelledby="services-heading">
      <div className="services-heading">
        <div>
          <h2 id="services-heading">
            Service credentials{" "}
            {list.data && (
              <span className="services-count">{list.data.length}</span>
            )}
          </h2>
          <p className="muted">
            Give your automations access to this workspace.
          </p>
        </div>
        <button
          type="button"
          className="button primary services-add"
          onClick={() => setCreating(true)}
        >
          <Plus size={16} />
          Add credential
        </button>
      </div>
      <ErrorBox error={list.error} retry={() => void list.refetch()} />
      {info.data?.push && !info.data.push.admissionOpen && (
        <p className="notice" role="status">
          New cloud input is temporarily closed. Existing receipts can still be
          reconciled.
        </p>
      )}
      {list.isPending && (
        <p className="notice" role="status">
          Loading credentials…
        </p>
      )}
      {list.data?.length === 0 && (
        <div className="panel services-empty">
          <h3>No credentials yet</h3>
          <p className="muted">
            Add a credential to let an automation inspect this workspace, retry
            deliveries, or send input to selected watches.
          </p>
        </div>
      )}
      {!!list.data?.length && (
        <div className="table-wrap services-table-wrap">
          <table className="services-table" role="table">
            <caption className="sr-only">Service credentials</caption>
            <thead role="rowgroup">
              <tr role="row">
                <th scope="col">Credential</th>
                <th scope="col">Access</th>
                <th scope="col">Key expires</th>
                <th scope="col">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody role="rowgroup">
              {list.data.map((service) => (
                <tr key={service.id} role="row">
                  <td role="cell">
                    <button
                      className="service-name"
                      onClick={() => setSelectedID(service.id)}
                    >
                      {service.name}
                    </button>
                  </td>
                  <td role="cell" data-label="Access">
                    <div className="service-permissions">
                      {service.scopes.map((scope) => (
                        <span className="service-permission" key={scope}>
                          {permissionNames[scope] || scope}
                        </span>
                      ))}
                    </div>
                    {service.scopes.includes("ingest") && (
                      <small className="muted">
                        {service.watches.length}{" "}
                        {service.watches.length === 1 ? "watch" : "watches"}
                      </small>
                    )}
                    {service.scopes.some((scope) => scope !== "ingest") && (
                      <small className="muted">Entire workspace</small>
                    )}
                  </td>
                  <td role="cell" data-label="Key expires">
                    <CredentialExpiry value={service.expiresAt} />
                  </td>
                  <td role="cell">
                    <button
                      className="text-button"
                      aria-label={`Manage ${service.name}`}
                      onClick={() => setSelectedID(service.id)}
                    >
                      Manage
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <details className="services-help">
        <summary>
          <CircleHelp size={15} />
          About service credentials
        </summary>
        <p>
          Credentials belong to this workspace and keep working when their
          creator leaves. Rotation preserves retry history and keeps the
          previous key valid for up to one hour.
        </p>
        <p>
          If a new key’s response is lost, check the credential details, rotate
          to get a replacement, and revoke the uncertain key. Secrets cannot be
          retrieved again.
        </p>
      </details>
      {creating && (
        <CreateCredential
          ingest={ingest}
          onClose={() => setCreating(false)}
          onIssued={(value) => {
            setCreating(false);
            setIssued(value);
          }}
          refresh={() => void list.refetch()}
        />
      )}
      {selected && !issued && (
        <ServiceCredentialDetails
          service={selected}
          onClose={() => setSelectedID(undefined)}
          onIssued={setIssued}
          refresh={list.refetch}
        />
      )}
      {issued && (
        <CredentialSecret
          credential={issued}
          onClose={() => setIssued(undefined)}
        />
      )}
    </section>
  );
}
