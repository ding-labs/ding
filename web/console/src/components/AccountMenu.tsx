import { useRef } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { LogOut, Settings, SlidersHorizontal, UserRound } from "lucide-react";
import { Link } from "react-router-dom";

export function AccountMenu({
  name,
  fallbackName = "Local session",
  role,
  workspace,
  onPreferences,
  onLogout,
}: {
  name?: string;
  fallbackName?: string;
  role?: string;
  workspace?: string;
  onPreferences: () => void;
  onLogout: () => void;
}) {
  const preferencesRequested = useRef(false);
  const initials = name
    ?.trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => Array.from(part)[0])
    .join("")
    .toUpperCase();
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button className="account-trigger" aria-label="Open account menu">
          {initials || <UserRound size={17} aria-hidden="true" />}
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content
          className="dropdown account-menu"
          align="end"
          sideOffset={8}
          collisionPadding={12}
          onCloseAutoFocus={(event) => {
            if (preferencesRequested.current) {
              event.preventDefault();
              preferencesRequested.current = false;
              onPreferences();
            }
          }}
        >
          <Menu.Label className="account-identity">
            <strong>{name || fallbackName}</strong>
            <span>
              {role || "Personal access"}
              {workspace && ` · ${workspace}`}
            </span>
          </Menu.Label>
          <Menu.Separator className="account-separator" />
          <Menu.Item asChild>
            <Link to="/settings">
              <Settings size={15} />
              Settings
            </Link>
          </Menu.Item>
          <Menu.Item
            onSelect={() => {
              preferencesRequested.current = true;
            }}
          >
            <SlidersHorizontal size={15} />
            Preferences
          </Menu.Item>
          <Menu.Item onSelect={onLogout}>
            <LogOut size={15} />
            Log out
          </Menu.Item>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
