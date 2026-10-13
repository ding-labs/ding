import { useState } from "react";
import {
  ServiceCredentialDetails,
  type Service,
} from "./ServiceCredentialDetails";
import { useExecution } from "../app/execution";
import { ErrorBox, useRead } from "../components/common";
import {
  CreateCredential,
  CredentialSecret,
  type IssuedCredential,
} from "./ServiceCredentialDialogs";
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
    <section className="panel">
      <h2>Service credentials</h2>
      <p>
        Workspace-owned automation keeps running when its creator leaves. Give
        each producer access to its selected watches. Rotation preserves retry
        history and allows the previous key for one hour.
      </p>
      <ErrorBox error={list.error} />
      {info.data?.push && !info.data.push.admissionOpen && (
        <p role="status">
          New cloud input is temporarily closed. Existing receipts can still be
          reconciled.
        </p>
      )}
      <p className="muted">
        If an issuance response is lost, inspect the credential list and key
        metadata. Rotate to obtain a replacement, then revoke the uncertain key.
        Secrets cannot be retrieved again.
      </p>
      <ul>
        {list.data?.map((s) => (
          <li key={s.id}>
            {s.name} · {s.scopes.join(", ")} · {s.watches.join(", ")} · expires{" "}
            {new Date(s.expiresAt).toLocaleString()}{" "}
            <button className="button" onClick={() => setSelectedID(s.id)}>
              Manage
            </button>
          </li>
        ))}
      </ul>
      <button
        type="button"
        className="button primary"
        onClick={() => setCreating(true)}
      >
        Add credential
      </button>
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
