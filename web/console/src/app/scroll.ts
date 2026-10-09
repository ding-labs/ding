import { useEffect } from "react";
import { useLocation } from "react-router-dom";
// Ephemeral navigation context; never persists payloads or identifiers to disk.
const positions = new Map<string, { y: number; focus?: string }>();
export function useNavigationContext() {
  const location = useLocation();
  const key = location.pathname + location.search;
  useEffect(() => {
    let y = positions.get(key)?.y || 0;
    let focus = positions.get(key)?.focus;
    const frame = requestAnimationFrame(() => {
      if (focus)
        Array.from(document.querySelectorAll<HTMLAnchorElement>("main a[href]"))
          .find((a) => a.getAttribute("href") === focus)
          ?.focus({ preventScroll: true });
      window.scrollTo(0, y);
    });
    const scroll = () => {
      y = window.scrollY;
    };
    const focused = (e: Event) => {
      const a = (e.target as HTMLElement).closest<HTMLAnchorElement>(
        "main a[href]",
      );
      if (a) focus = a.getAttribute("href") || undefined;
    };
    window.addEventListener("scroll", scroll, { passive: true });
    document.addEventListener("focusin", focused);
    document.addEventListener("click", focused, true);
    return () => {
      cancelAnimationFrame(frame);
      positions.set(key, { y, focus });
      if (positions.size > 100)
        positions.delete(positions.keys().next().value!);
      window.removeEventListener("scroll", scroll);
      document.removeEventListener("focusin", focused);
      document.removeEventListener("click", focused, true);
    };
  }, [key]);
}
