import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { StoreConsoleDestination, StoreDestinationPage } from "../api/contracts";
import { ErrorBox } from "./common";

export function SavedDestinationPicker({ value, onChange, disabled, selectedRef }: {
  value?: StoreConsoleDestination;
  selectedRef?: string;
  onChange: (value?: StoreConsoleDestination) => void;
  disabled?: boolean;
}) {
  const query = useInfiniteQuery({
    queryKey: ["onboarding-destinations"],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) => api<StoreDestinationPage>(`/console/destinations?limit=50&cursor=${encodeURIComponent(pageParam)}`, { signal }),
    getNextPageParam: page => page.more ? page.cursor : undefined,
  });
  const destinations = [...new Map(query.data?.pages.flatMap(page => page.destinations).map(d => [d.definition.metadata.id, d])).values()];
  return <>
    <label>Saved destination<select disabled={disabled} value={selectedRef ?? value?.definition.metadata.id ?? ""}
      onChange={event => onChange(destinations.find(d => d.definition.metadata.id === event.target.value))}>
      <option value="">Set up new delivery</option>
      {selectedRef && !destinations.some(d => d.definition.metadata.id === selectedRef) && <option value={selectedRef}>{selectedRef}</option>}
      {destinations.map(d => <option key={d.definition.metadata.id} value={d.definition.metadata.id}>
        {d.definition.metadata.name || d.definition.metadata.id} ({d.definition.spec.type})
      </option>)}
    </select></label>
    <div className="actions">
      <button className="button" type="button" disabled={disabled || query.isFetching} onClick={() => { onChange(undefined); void query.refetch(); }}>Refresh saved destinations</button>
      {query.hasNextPage && <button className="button" type="button" disabled={disabled || query.isFetching} onClick={() => void query.fetchNextPage()}>Load more destinations</button>}
    </div>
    <ErrorBox error={query.error} />
    {value && <p className="muted">This watch will reference {value.definition.metadata.id}. Its saved configuration will not be changed.</p>}
  </>;
}
