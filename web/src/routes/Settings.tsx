import { Eye, LockKeyhole, Palette } from "lucide-react";

import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent } from "../components/ui/card";
import { StateBanner } from "../components/ui/state-banner";
import { useTheme, type Theme } from "../theme/theme-context";

const themeOptions: Array<{
  value: Theme;
  label: string;
  description: string;
}> = [
  {
    value: "system",
    label: "System",
    description: "Follow the operating system preference.",
  },
  {
    value: "light",
    label: "Light",
    description: "Use the light console palette.",
  },
  {
    value: "dark",
    label: "Dark",
    description: "Use the dark console palette.",
  },
];

export function Settings() {
  const { theme, setTheme } = useTheme();
  return (
    <div className="space-y-6">
      <PageHeader
        description="Presentation preferences for this browser. Operational configuration remains the bridgectl CLI's responsibility."
        eyebrow="Preferences"
        title="Settings"
      />
      <StateBanner
        message="These preferences are stored locally in this browser. They do not change ERPBridge or any selected context."
        title="Read-only console preferences"
        tone="info"
      />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardContent>
            <div className="flex items-start gap-3">
              <Palette
                aria-hidden="true"
                className="mt-0.5 text-primary"
                size={20}
              />
              <div>
                <h2 className="text-lg font-semibold">Appearance</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Choose the palette that best supports your working
                  environment.
                </p>
              </div>
            </div>
            <fieldset className="mt-5 space-y-2">
              <legend className="sr-only">Color theme</legend>
              {themeOptions.map((option) => (
                <label
                  className="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 hover:bg-muted/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5"
                  key={option.value}
                >
                  <input
                    checked={theme === option.value}
                    className="mt-1 h-4 w-4 accent-primary"
                    name="theme"
                    onChange={() => setTheme(option.value)}
                    type="radio"
                    value={option.value}
                  />
                  <span>
                    <span className="block text-sm font-medium">
                      {option.label}
                    </span>
                    <span className="mt-1 block text-sm text-muted-foreground">
                      {option.description}
                    </span>
                  </span>
                </label>
              ))}
            </fieldset>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <div className="flex items-start gap-3">
              <Eye
                aria-hidden="true"
                className="mt-0.5 text-primary"
                size={20}
              />
              <div>
                <h2 className="text-lg font-semibold">Accessibility</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  The console respects reduced-motion preferences and keeps a
                  semantic text alternative for charts and topology.
                </p>
              </div>
            </div>
            <ul className="mt-5 space-y-3 text-sm text-muted-foreground">
              <li>
                Keyboard focus indicators remain visible throughout the console.
              </li>
              <li>
                Charts include a text summary and an accessible data table.
              </li>
              <li>
                Topology retains its accessible relationship list beside the
                canvas.
              </li>
            </ul>
          </CardContent>
        </Card>
      </div>
      <Card>
        <CardContent className="flex items-start gap-3">
          <LockKeyhole
            aria-hidden="true"
            className="mt-0.5 text-success"
            size={20}
          />
          <div>
            <h2 className="font-semibold">Security boundary</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              This console remains loopback-only, capability-protected,
              read-only, and free of credentials, raw payloads, and full
              upstream URLs.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
