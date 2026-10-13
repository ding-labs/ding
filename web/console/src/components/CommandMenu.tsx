import { useEffect, useRef, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  Search,
  ArrowRight,
  ArrowLeft,
  Check,
  ChevronRight,
  X,
} from "lucide-react";
import { useNavigate } from "react-router-dom";
const commands = [
  { name: "Find a watch", to: "/watches" },
  { name: "Create a watch", to: "/workbench" },
  { name: "Inspect events", to: "/events" },
  { name: "Investigate deliveries", to: "/deliveries" },
  { name: "Run Doctor", to: "/system" },
  { name: "Back up this daemon", to: "/system?tab=Backup" },
  { name: "Read CLI help", to: "/system?tab=CLI+setup" },
];
type Preference = {
  id: string;
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
};
export function CommandMenu({
  open,
  onOpenChange,
  preferences,
  initialSearch = "",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preferences: Preference[];
  initialSearch?: string;
}) {
  const [search, setSearch] = useState("");
  const [preferenceId, setPreferenceId] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const results = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const selected = preferences.find((p) => p.id === preferenceId);
  const matches = (label: string) =>
    label.toLowerCase().includes(search.trim().toLowerCase());
  const navigation = commands.filter((c) => matches(c.name));
  const settings = preferences.filter((p) =>
    matches(
      `Preferences ${p.label} ${p.options.map((o) => o.label).join(" ")}`,
    ),
  );
  const options = selected?.options.filter((o) => matches(o.label));
  const empty = selected
    ? !options?.length
    : !navigation.length && !settings.length;
  const back = () => {
    setPreferenceId(null);
    setSearch("");
  };
  useEffect(() => {
    if (open) {
      setSearch(initialSearch);
      setPreferenceId(null);
    }
  }, [open, initialSearch]);
  useEffect(() => {
    input.current?.focus();
  }, [preferenceId]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (
        (e.metaKey || e.ctrlKey) &&
        e.key.toLowerCase() === "k" &&
        (open || !t.closest("input,textarea,[contenteditable=true]"))
      ) {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [open, onOpenChange]);
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Trigger asChild>
        <button
          className="button command-trigger"
          aria-label="Open command menu"
        >
          <Search size={14} />
          <span>Go to…</span>
          <kbd>
            {/Mac|iPhone|iPad/.test(navigator.platform) ? "⌘ K" : "Ctrl K"}
          </kbd>
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content
          className="dialog-content command-menu"
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            input.current?.focus();
          }}
          onEscapeKeyDown={(e) => {
            if (selected) {
              e.preventDefault();
              back();
            }
          }}
        >
          <div className="command-heading">
            {selected && (
              <button
                className="icon-button"
                aria-label="Back to all commands"
                onClick={back}
              >
                <ArrowLeft size={17} />
              </button>
            )}
            <Dialog.Title>
              {selected?.label || "Go to a task or preference"}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="icon-button" aria-label="Close command menu">
                <X size={17} />
              </button>
            </Dialog.Close>
          </div>
          <Dialog.Description className="sr-only">
            Search tasks and preferences. Use the arrow keys to select, Enter to
            choose, or Escape to go back or close.
          </Dialog.Description>
          <input
            ref={input}
            aria-label={
              selected
                ? `Search ${selected.label.toLowerCase()} options`
                : "Search tasks and preferences"
            }
            placeholder={
              selected ? "Find an option…" : "Find a task or preference…"
            }
            onKeyDown={(e) => {
              if (["ArrowDown", "ArrowUp", "Enter"].includes(e.key)) {
                e.preventDefault();
                const buttons =
                  results.current?.querySelectorAll<HTMLButtonElement>(
                    "button",
                  );
                const button =
                  buttons?.[e.key === "ArrowUp" ? buttons.length - 1 : 0];
                if (e.key === "Enter") button?.click();
                else button?.focus();
              }
            }}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div
            className="command-results"
            ref={results}
            onKeyDown={(e) => {
              if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
              e.preventDefault();
              const buttons = Array.from(
                e.currentTarget.querySelectorAll("button"),
              );
              const i = buttons.indexOf(
                document.activeElement as HTMLButtonElement,
              );
              buttons[
                (i + (e.key === "ArrowDown" ? 1 : -1) + buttons.length) %
                  buttons.length
              ]?.focus();
            }}
          >
            {selected ? (
              options?.map((option) => (
                <button
                  key={option.value}
                  aria-pressed={selected.value === option.value}
                  onClick={() => {
                    selected.onChange(option.value);
                    back();
                  }}
                >
                  <span>{option.label}</span>
                  {selected.value === option.value && (
                    <Check size={16} aria-hidden="true" />
                  )}
                </button>
              ))
            ) : (
              <>
                {navigation.length > 0 && <h3>Navigate</h3>}
                {navigation.map((c) => (
                  <button
                    key={c.to}
                    onClick={() => {
                      onOpenChange(false);
                      navigate(c.to);
                    }}
                  >
                    <span>{c.name}</span>
                    <ArrowRight size={15} />
                  </button>
                ))}
                {settings.length > 0 && <h3>Preferences</h3>}
                {settings.map((p) => (
                  <button
                    key={p.id}
                    onClick={() => {
                      setPreferenceId(p.id);
                      setSearch("");
                    }}
                  >
                    <span>{p.label}</span>
                    <small>
                      {p.options.find((o) => o.value === p.value)?.label}
                    </small>
                    <ChevronRight size={15} aria-hidden="true" />
                  </button>
                ))}
              </>
            )}
            {empty && (
              <p className="command-empty" role="status">
                No matching {selected ? "options" : "tasks or preferences"}.
              </p>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
