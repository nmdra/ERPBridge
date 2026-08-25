import { useEffect, useMemo, useState, type ReactNode } from "react";

import { ThemeContext, type Theme } from "./theme-context";

const STORAGE_KEY = "erpbridge-console-theme";

function readStoredTheme(): Theme {
  const value = localStorage.getItem(STORAGE_KEY);
  return value === "light" || value === "dark" || value === "system"
    ? value
    : "light";
}

function systemTheme(): "light" | "dark" {
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function reducedMotion(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(readStoredTheme);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, theme);
    const root = document.documentElement;
    root.dataset.theme = theme === "system" ? systemTheme() : theme;
    root.dataset.reducedMotion = reducedMotion() ? "true" : "false";
    root.style.colorScheme = root.dataset.theme;
  }, [theme]);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") {
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => {
      if (theme === "system") {
        document.documentElement.dataset.theme = systemTheme();
        document.documentElement.style.colorScheme = systemTheme();
      }
      document.documentElement.dataset.reducedMotion = motion.matches
        ? "true"
        : "false";
    };
    media.addEventListener("change", update);
    motion.addEventListener("change", update);
    return () => {
      media.removeEventListener("change", update);
      motion.removeEventListener("change", update);
    };
  }, [theme]);

  const value = useMemo(() => ({ theme, setTheme }), [theme]);
  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}
