/** @type {import('tailwindcss').Config} */
export default {
      content: ["./index.html", "./src/**/*.{ts,tsx}"],
      theme: {
            extend: {
                  colors: {
                        background: "hsl(var(--background))",
                        foreground: "hsl(var(--foreground))",
                        card: "hsl(var(--card))",
                        "card-foreground": "hsl(var(--card-foreground))",
                        muted: "hsl(var(--muted))",
                        "muted-foreground": "hsl(var(--muted-foreground))",
                        primary: "hsl(var(--primary))",
                        "primary-foreground": "hsl(var(--primary-foreground))",
                        border: "hsl(var(--border))",
                        ring: "hsl(var(--ring))",
                        success: "hsl(var(--success))",
                        warning: "hsl(var(--warning))",
                        "warning-foreground": "hsl(var(--warning-foreground))",
                        destructive: "hsl(var(--destructive))",
                        info: "hsl(var(--info))",
                        sidebar: "hsl(var(--sidebar))",
                        "sidebar-foreground": "hsl(var(--sidebar-foreground))",
                        "sidebar-accent": "hsl(var(--sidebar-accent))",
                        "sidebar-accent-foreground":
                              "hsl(var(--sidebar-accent-foreground))",
                  },
            },
      },
      plugins: [],
};
