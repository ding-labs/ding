import { useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Search, ArrowRight } from "lucide-react";
import { useNavigate } from "react-router-dom";
const commands = [
  { name: "Find a watch", to: "/watches" },
  { name: "Create a watch", to: "/workbench" },
  { name: "Inspect events", to: "/events" },
  { name: "Investigate deliveries", to: "/deliveries" },
  { name: "Run Doctor", to: "/system" },
  { name: "Back up this daemon", to: "/system?tab=Backup" },
  { name: "Import legacy rules", to: "/workbench?tab=Legacy+import" },
  { name: "Read CLI help", to: "/system?tab=CLI+setup" },
];
export function CommandMenu() {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const navigate = useNavigate();
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (
        (e.metaKey || e.ctrlKey) &&
        e.key.toLowerCase() === "k" &&
        !t.closest("input,textarea,[contenteditable=true]")
      ) {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        setSearch("");
      }}
    >
      <Dialog.Trigger asChild>
        <button
          className="button command-trigger"
          aria-label="Open command menu"
        >
          <Search size={14} />
          <span>Go to…</span>
          <kbd>⌘ / Ctrl K</kbd>
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content command-menu">
          <Dialog.Title>Go to a task</Dialog.Title>
          <Dialog.Description>
            Search navigation commands. Use the arrow keys to select, Enter to
            open, or Escape to close.
          </Dialog.Description>
          <input
            aria-label="Search navigation commands"
            placeholder="What would you like to do?"
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                document
                  .querySelector<HTMLButtonElement>(".command-results button")
                  ?.focus();
              }
            }}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div
            className="command-results"
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
            {commands
              .filter((c) =>
                c.name.toLowerCase().includes(search.toLowerCase()),
              )
              .map((c) => (
                <button
                  key={c.to}
                  onClick={() => {
                    setOpen(false);
                    navigate(c.to);
                  }}
                >
                  {c.name}
                  <ArrowRight size={15} />
                </button>
              ))}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
