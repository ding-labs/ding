import { useSearchParams } from "react-router-dom";
import "./WatchColumnFilters.css";

const columns = [
  {
    name: "status",
    label: "Lifecycle",
    all: "All lifecycles",
    options: [
      ["running", "Running"],
      ["paused", "Paused"],
      ["deleted", "Deleted"],
    ],
  },
  {
    name: "condition",
    label: "Condition",
    all: "All conditions",
    options: [
      ["waiting", "Waiting for input"],
      ["open", "Open incidents"],
      ["clear", "No open incident"],
      ["evaluated", "Evaluated entities"],
    ],
  },
  {
    name: "health",
    label: "Source",
    all: "All source states",
    options: [
      ["error", "Acquisition error"],
      ["missing", "Missing credentials"],
      ["waiting", "Waiting"],
      ["received", "Input received"],
      ["clear", "No acquisition error"],
    ],
  },
  {
    name: "delivery",
    label: "Deliveries",
    all: "All deliveries",
    options: [
      ["failed", "Failed"],
      ["queued", "Queued / sending"],
      ["clear", "No delivery attention"],
      ["pending", "Any queued deliveries"],
    ],
  },
  {
    name: "input",
    label: "Latest input",
    all: "Any time",
    options: [
      ["never", "Not yet"],
      ["received", "Input received"],
      ["hour", "Last hour"],
      ["day", "Last 24 hours"],
      ["week", "Last 7 days"],
    ],
  },
];

export function WatchColumnFilters() {
  const [params, setParams] = useSearchParams();
  const change = (name: string, value: string, replace = false) => {
    const next = new URLSearchParams(params);
    next.delete("cursor");
    value ? next.set(name, value) : next.delete(name);
    setParams(next, { replace });
  };
  return (
    <tr className="watch-column-filters">
      <td>
        <input
          aria-label="Search watches"
          placeholder="Name or ID…"
          value={params.get("search") || ""}
          onChange={(event) => change("search", event.target.value, true)}
          type="search"
        />
      </td>
      {columns.map(({ name, label, all, options }) => (
        <td key={name}>
          <select
            aria-label={`Filter ${label.toLowerCase()}`}
            value={params.get(name) || ""}
            onChange={(event) => change(name, event.target.value)}
          >
            <option value="">{all}</option>
            {options.map(([value, text]) => (
              <option key={value} value={value}>
                {text}
              </option>
            ))}
          </select>
        </td>
      ))}
    </tr>
  );
}
