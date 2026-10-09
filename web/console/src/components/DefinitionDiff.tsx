// A linear-time changed-block diff. Common leading/trailing context is collapsed;
// the middle is shown literally so even large manifests cannot freeze the browser.
export function DefinitionDiff({
  before = "",
  after = "",
}: {
  before?: string;
  after?: string;
}) {
  const a = before.split("\n"),
    b = after.split("\n");
  let start = 0,
    end = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  while (
    end < a.length - start &&
    end < b.length - start &&
    a[a.length - 1 - end] === b[b.length - 1 - end]
  )
    end++;
  if (before === after) return <p className="muted">Definition unchanged.</p>;
  const lines = [
    ...a
      .slice(Math.max(0, start - 3), start)
      .map((s) => ({ kind: "context", text: "  " + s })),
    ...a
      .slice(start, a.length - end)
      .map((s) => ({ kind: "removed", text: "− " + s })),
    ...b
      .slice(start, b.length - end)
      .map((s) => ({ kind: "added", text: "+ " + s })),
    ...b
      .slice(b.length - end, b.length - end + 3)
      .map((s) => ({ kind: "context", text: "  " + s })),
  ];
  return (
    <details className="raw">
      <summary>Show changed block</summary>
      <pre aria-label="Definition changes">
        {lines.slice(0, 400).map((l, i) => (
          <span className={`diff-line ${l.kind}`} key={i}>
            {l.text.slice(0, 1000)}
            {"\n"}
          </span>
        ))}
        {lines.length > 400
          ? "Preview limited to 400 lines. Download the complete review for every definition."
          : ""}
      </pre>
    </details>
  );
}
