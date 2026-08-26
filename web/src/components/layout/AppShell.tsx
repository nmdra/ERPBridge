import * as Dialog from "@radix-ui/react-dialog";
import {
  Activity,
  BarChart3,
  Boxes,
  FileText,
  Home,
  Menu,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Plug,
  Settings,
  Wrench,
  X,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link, useLocation } from "wouter";

import type { ContextProjection } from "../../hooks/useConsole";
import { cn } from "../../lib/cn";
import { Button } from "../ui/button";
import { ThemeToggle } from "./ThemeToggle";

type NavigationItem = {
  label: string;
  href: string;
  icon: typeof Home;
  matches?: string[];
};

type NavigationGroup = {
  label: string;
  items: NavigationItem[];
};

const navigationGroups: NavigationGroup[] = [
  {
    label: "Monitor",
    items: [
      { label: "Overview", href: "/", icon: Home },
      { label: "Logs", href: "/logs", icon: FileText },
      { label: "Metrics", href: "/metrics", icon: BarChart3 },
    ],
  },
  {
    label: "Inventory",
    items: [
      {
        label: "Contexts",
        href: "/contexts",
        icon: Boxes,
        matches: ["/contexts", "/deployments"],
      },
      { label: "Tools", href: "/tools", icon: Wrench },
      { label: "Plugins", href: "/plugins", icon: Plug },
    ],
  },
  {
    label: "Diagnose",
    items: [
      {
        label: "Integration topology",
        href: "/topology",
        icon: Network,
      },
    ],
  },
];

function isActive(location: string, item: NavigationItem) {
  const paths = item.matches ?? [item.href];
  return paths.some(
    (path) =>
      location === path || (path !== "/" && location.startsWith(`${path}/`)),
  );
}

function NavigationLinks({
  location,
  collapsed,
  onNavigate,
}: {
  location: string;
  collapsed: boolean;
  onNavigate?: () => void;
}) {
  return (
    <nav aria-label="Primary" className="space-y-6">
      {navigationGroups.map((group) => (
        <div key={group.label}>
          <p
            className={cn(
              "mb-2 px-3 text-[0.65rem] font-semibold uppercase tracking-[0.16em] text-sidebar-foreground/60",
              collapsed && "lg:sr-only",
            )}
          >
            {group.label}
          </p>
          <div className="space-y-1">
            {group.items.map(({ label, href, icon: Icon, ...item }) => {
              const active = isActive(location, {
                label,
                href,
                icon: Icon,
                ...item,
              });
              return (
                <Link
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "flex min-h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    collapsed && "lg:justify-center lg:px-2",
                    active &&
                      "bg-sidebar-accent text-sidebar-accent-foreground shadow-sm",
                  )}
                  href={href}
                  key={label}
                  onClick={onNavigate}
                  title={collapsed ? label : undefined}
                >
                  <Icon aria-hidden="true" size={17} />
                  <span className={collapsed ? "lg:sr-only" : undefined}>
                    {label}
                  </span>
                </Link>
              );
            })}
          </div>
        </div>
      ))}
      <div className="border-t border-sidebar-foreground/10 pt-4">
        <Link
          aria-current={location === "/settings" ? "page" : undefined}
          className={cn(
            "flex min-h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            collapsed && "lg:justify-center lg:px-2",
            location === "/settings" &&
              "bg-sidebar-accent text-sidebar-accent-foreground shadow-sm",
          )}
          href="/settings"
          onClick={onNavigate}
          title={collapsed ? "Settings" : undefined}
        >
          <Settings aria-hidden="true" size={17} />
          <span className={collapsed ? "lg:sr-only" : undefined}>Settings</span>
        </Link>
      </div>
    </nav>
  );
}

function Brand({ collapsed }: { collapsed: boolean }) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 py-3",
        collapsed ? "lg:justify-center" : "px-2",
      )}
    >
      <Activity
        className="shrink-0 text-primary"
        aria-hidden="true"
        size={20}
      />
      <span
        className={cn(
          "font-semibold tracking-tight",
          collapsed && "lg:sr-only",
        )}
      >
        ERPBridge{" "}
        <span className="text-[0.65rem] font-normal text-sidebar-foreground/60">
          Console
        </span>
      </span>
    </div>
  );
}

export function AppShell({
  children,
  contexts,
  selectedContext,
  onContextChange,
  onRefresh,
}: {
  children: ReactNode;
  contexts: ContextProjection[] | null;
  selectedContext: string;
  onContextChange: (context: string) => void;
  onRefresh?: () => void;
}) {
  const [location] = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const activeItem = navigationGroups
    .flatMap((group) => group.items)
    .find((item) => isActive(location, item));
  const title =
    activeItem?.label ?? (location === "/settings" ? "Settings" : "Console");
  const contextOptions = contexts?.length
    ? contexts
    : selectedContext
      ? [{ name: selectedContext } as ContextProjection]
      : [];

  return (
    <>
      <a
        className="sr-only fixed left-4 top-4 z-[60] rounded-lg bg-card px-4 py-2 text-sm font-medium text-foreground shadow-lg focus:not-sr-only focus:outline-none focus:ring-2 focus:ring-ring"
        href="#main-content"
      >
        Skip to main content
      </a>
      <div
        className={cn(
          "min-h-screen bg-background text-foreground lg:grid",
          collapsed ? "lg:grid-cols-[4.5rem_1fr]" : "lg:grid-cols-[16rem_1fr]",
        )}
      >
        <aside
          aria-label="Sidebar navigation"
          className={cn(
            "hidden border-r border-border bg-sidebar p-4 lg:block lg:min-h-screen",
            collapsed && "p-2",
          )}
        >
          <div className="flex items-center justify-between gap-2">
            <Brand collapsed={collapsed} />
            <button
              aria-expanded={!collapsed}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              className="inline-flex min-h-9 min-w-9 shrink-0 items-center justify-center rounded-lg text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
          <div className="mt-6">
            <NavigationLinks collapsed={collapsed} location={location} />
          </div>
        </aside>

        <div className="min-w-0">
          <header className="sticky top-0 z-20 flex min-h-16 flex-wrap items-center justify-between gap-3 border-b border-border bg-card/95 px-4 py-3 shadow-sm backdrop-blur sm:px-6 lg:px-8">
            <div className="flex min-w-0 items-center gap-3">
              <Dialog.Root
                onOpenChange={setMobileNavigationOpen}
                open={mobileNavigationOpen}
              >
                <Dialog.Trigger asChild>
                  <Button
                    aria-label="Open navigation"
                    className="lg:hidden"
                    variant="secondary"
                  >
                    <Menu aria-hidden="true" size={17} />
                    <span className="sr-only">Menu</span>
                  </Button>
                </Dialog.Trigger>
                <Dialog.Portal>
                  <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 lg:hidden" />
                  <Dialog.Content className="fixed inset-y-0 left-0 z-50 w-[min(20rem,88vw)] overflow-y-auto bg-sidebar p-5 text-sidebar-foreground shadow-xl focus:outline-none lg:hidden">
                    <div className="flex items-center justify-between gap-3">
                      <Dialog.Title className="font-semibold tracking-tight">
                        ERPBridge Console
                      </Dialog.Title>
                      <Dialog.Close asChild>
                        <Button aria-label="Close navigation" variant="ghost">
                          <X aria-hidden="true" size={18} />
                        </Button>
                      </Dialog.Close>
                    </div>
                    <Dialog.Description className="mt-2 text-sm text-sidebar-foreground/70">
                      Navigate read-only monitoring and diagnostic surfaces.
                    </Dialog.Description>
                    <div className="mt-8">
                      <NavigationLinks
                        collapsed={false}
                        location={location}
                        onNavigate={() => setMobileNavigationOpen(false)}
                      />
                    </div>
                  </Dialog.Content>
                </Dialog.Portal>
              </Dialog.Root>
              <div className="min-w-0">
                <p className="truncate text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
                  {title}
                </p>
                <p className="truncate text-sm text-muted-foreground">
                  Context:{" "}
                  <span className="font-medium text-foreground">
                    {selectedContext || "Loading"}
                  </span>
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <label className="sr-only" htmlFor="context-select">
                Select context
              </label>
              <select
                aria-label="Select context"
                className="h-9 max-w-[13rem] rounded-lg border border-border bg-card px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                disabled={!contextOptions.length}
                id="context-select"
                onChange={(event) => onContextChange(event.target.value)}
                value={selectedContext}
              >
                {contextOptions.length ? (
                  contextOptions.map((context) => (
                    <option key={context.name} value={context.name}>
                      {context.name}
                    </option>
                  ))
                ) : (
                  <option value="">Loading contexts…</option>
                )}
              </select>
              {onRefresh ? (
                <Button
                  aria-label="Refresh contexts"
                  onClick={onRefresh}
                  variant="secondary"
                >
                  Refresh
                </Button>
              ) : null}
              <ThemeToggle />
            </div>
          </header>
          <main
            className="mx-auto max-w-[1600px] p-4 outline-none sm:p-6 lg:p-8"
            id="main-content"
            tabIndex={-1}
          >
            {children}
          </main>
        </div>
      </div>
    </>
  );
}
