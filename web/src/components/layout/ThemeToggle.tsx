import { useTheme, type Theme } from "../../theme/theme-context";

const themeLabels: Record<Theme, string> = {
  light: "Light",
  dark: "Dark",
  system: "System",
};

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  return (
    <label className="inline-flex items-center">
      <span className="sr-only">Color theme</span>
      <select
        aria-label="Color theme"
        className="h-9 rounded-lg border border-border bg-card px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onChange={(event) => setTheme(event.target.value as Theme)}
        value={theme}
      >
        {(Object.keys(themeLabels) as Theme[]).map((option) => (
          <option key={option} value={option}>
            {themeLabels[option]} theme
          </option>
        ))}
      </select>
    </label>
  );
}
