/**
 * ThemeToggle (task 6.1 — dark mode): a compact Select for picking the
 * theme (Light / Dark / System). Uses the theme context from
 * `@/context/theme-provider` and the shadcn `Select`.
 *
 * The trigger carries `aria-label="Theme"` so the app-shell axe tests (which
 * run `axe.run` on the whole shell) see a stable accessible name; the current
 * mode is additionally reflected by an icon + the `SelectValue` label.
 */
import { Monitor, Moon, Sun } from 'lucide-react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useTheme } from '@/context/theme-provider';
import type { ThemeMode } from '@/context/theme-provider';

const MODE_ICONS: Record<ThemeMode, typeof Sun> = {
  light: Sun,
  dark: Moon,
  system: Monitor,
};

export const ThemeToggle = () => {
  const { theme, setTheme } = useTheme();
  const Icon = MODE_ICONS[theme];

  return (
    <Select value={theme} onValueChange={(value) => setTheme(value as ThemeMode)}>
      <SelectTrigger size="sm" aria-label="Theme" className="w-fit min-h-11 gap-1.5">
        <Icon aria-hidden="true" className="size-3.5" />
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="light">Light</SelectItem>
        <SelectItem value="dark">Dark</SelectItem>
        <SelectItem value="system">System</SelectItem>
      </SelectContent>
    </Select>
  );
};
