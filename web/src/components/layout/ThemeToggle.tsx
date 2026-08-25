import { Laptop, Moon, Sun } from "lucide-react";

import { useTheme, type Theme } from "../../theme/theme-context";
import { Button } from "../ui/button";

const nextTheme: Record<Theme, Theme> = {
    light: "dark",
    dark: "system",
    system: "light",
};

const icon: Record<Theme, typeof Sun> = {
    light: Sun,
    dark: Moon,
    system: Laptop,
};

export function ThemeToggle() {
    const { theme, setTheme } = useTheme();
    const Icon = icon[theme];
    return (
        <Button
            aria-label={`Theme: ${theme}`}
            onClick={() => setTheme(nextTheme[theme])}
            title="Change theme"
            variant="ghost"
        >
            <Icon aria-hidden="true" size={17} />
        </Button>
    );
}
