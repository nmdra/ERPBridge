import {
  Activity,
  BarChart3,
  Boxes,
  FileText,
  Home,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Wrench,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link, useLocation } from "wouter";

import type { ContextProjection } from "../../hooks/useConsole";
import { cn } from "../../lib/cn";
import { ThemeToggle } from "./ThemeToggle";

const navigation = [
  { label: "Overview", href: "/", icon: Home },
  { label: "Deployments", href: "/deployments", icon: Boxes },
  { label: "Logs", href: "/logs", icon: FileText },
  { label: "Metrics", href: "/metrics", icon: BarChart3 },
  { label: "Tools", href: "/tools", icon: Wrench },
  { label: "Topology", href: "/topology", icon: Network },
];

export function AppShell({
  children,
  contexts,
  selectedContext,
  onContextChange,
}: {
  children: ReactNode;
  contexts: ContextProjection[] | null;
  selectedContext: string;
  onContextChange: (context: string) => void;
}) {
  const [location] = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const title =
    navigation.find((item) => item.href === location)?.label ?? "Settings";
  const contextOptions = contexts?.length
    ? contexts
    : [{ name: selectedContext } as ContextProjection];
  const labelClass = collapsed ? "lg:sr-only" : undefined;

  return (
    <div
      className={cn(
        "min-h-screen bg-background text-foreground lg:grid",
        collapsed ? "lg:grid-cols-[4.5rem_1fr]" : "lg:grid-cols-[15rem_1fr]",
      )}
    >
      <aside
        className={cn(
          "border-b border-border bg-sidebar lg:min-h-screen lg:border-b-0 lg:border-r",
          collapsed ? "p-2" : "p-4",
        )}
      >
        <div
          className={cn(
            "flex items-center justify-between gap-2 py-3",
            collapsed ? "lg:px-0" : "px-2",
          )}
        >
          <div className="flex min-w-0 items-center gap-2">
            <Activity
              className="shrink-0 text-primary"
              aria-hidden="true"
              size={20}
            />
            <span className={cn("font-semibold tracking-tight", labelClass)}>
              ERPBridge{" "}
              <span className="text-[0.65rem] font-normal text-muted-foreground">
                Console
              </span>
            </span>
          </div>
          <button
            aria-expanded={!collapsed}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="inline-flex min-h-9 min-w-9 shrink-0 items-center justify-center rounded-md text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => setCollapsed((value) => !value)}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            type="button"
          >
            {collapsed ? (
              <PanelLeftOpen aria-hidden="true" size={17} />
            ) : (
              <PanelLeftClose aria-hidden="true" size={17} />
            )}
          </button>
        </div>
        <nav aria-label="Primary" className="mt-5 space-y-1">
          {navigation.map(({ label, href, icon: Icon }) => (
            <Link
              className={cn(
                "flex min-h-10 items-center gap-3 rounded-md px-3 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                collapsed && "lg:justify-center lg:px-2",
                location === href &&
                  "bg-sidebar-accent text-sidebar-accent-foreground",
              )}
              href={href}
              key={label}
              title={collapsed ? label : undefined}
            >
              <Icon aria-hidden="true" size={17} />
              <span className={labelClass}>{label}</span>
            </Link>
          ))}
        </nav>
        <div className="mt-8 border-t border-border pt-4">
          <Link
            className={cn(
              "flex min-h-10 items-center gap-3 rounded-md px-3 text-sm font-medium text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              collapsed && "lg:justify-center lg:px-2",
              location === "/settings" &&
                "bg-sidebar-accent text-sidebar-accent-foreground",
            )}
            href="/settings"
            title={collapsed ? "Settings" : undefined}
          >
            <Settings aria-hidden="true" size={17} />
            <span className={labelClass}>Settings</span>
          </Link>
        </div>
      </aside>
      <div className="min-w-0">
        <header className="flex min-h-16 items-center justify-between border-b border-border bg-card px-5 lg:px-8">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              ERPBridge{" "}
              <span className="text-[0.6rem] font-normal normal-case tracking-normal">
                Console
              </span>
            </p>
            <h1 className="text-lg font-semibold">{title}</h1>
          </div>
          <div className="flex items-center gap-2">
            <label className="sr-only" htmlFor="deployment-select">
              Select deployment
            </label>
            <select
              className="h-9 rounded-md border border-border bg-card px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              id="deployment-select"
              onChange={(event) => onContextChange(event.target.value)}
              value={selectedContext}
            >
              {contextOptions.map((context) => (
                <option key={context.name} value={context.name}>
                  {context.name}
                </option>
              ))}
            </select>
            <ThemeToggle />
          </div>
        </header>
        <main className="p-5 lg:p-8">{children}</main>
      </div>
    </div>
  );
}
