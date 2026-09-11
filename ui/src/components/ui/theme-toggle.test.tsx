/**
 * Task 6.1 — ThemeToggle: renders a Select labeled "Theme" with the three
 * modes, selecting one persists through the theme context, and the control is
 * axe-clean inside the theme provider.
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import axe from 'axe-core';
import { ThemeProvider } from '@/context/theme-provider';
import { ThemeToggle } from './theme-toggle';

const THEME_KEY = 'procrastinator-theme';

const renderToggle = () =>
  render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  );

describe('ThemeToggle', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove('light');
    document.documentElement.classList.remove('dark');
  });

  it('renders the trigger with an accessible name and the default mode label', () => {
    renderToggle();
    const trigger = screen.getByRole('combobox', { name: 'Theme' });
    expect(trigger).toBeInTheDocument();
    // Default mode is "system" → the SelectValue shows "System".
    expect(trigger).toHaveTextContent('System');
  });

  it('selecting Dark persists the preference and switches the html class', async () => {
    const user = userEvent.setup();
    renderToggle();

    await user.click(screen.getByRole('combobox', { name: 'Theme' }));
    const option = await screen.findByRole('option', { name: 'Dark' });
    await user.click(option);

    expect(localStorage.getItem(THEME_KEY)).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
    // The trigger reflects the new mode.
    expect(screen.getByRole('combobox', { name: 'Theme' })).toHaveTextContent('Dark');
  });

  it('selecting Light persists the preference and switches the html class', async () => {
    const user = userEvent.setup();
    localStorage.setItem(THEME_KEY, 'dark');
    renderToggle();

    await user.click(screen.getByRole('combobox', { name: 'Theme' }));
    const option = await screen.findByRole('option', { name: 'Light' });
    await user.click(option);

    expect(localStorage.getItem(THEME_KEY)).toBe('light');
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    expect(screen.getByRole('combobox', { name: 'Theme' })).toHaveTextContent('Light');
  });

  it('exposes the three mode options when opened', async () => {
    const user = userEvent.setup();
    renderToggle();

    await user.click(screen.getByRole('combobox', { name: 'Theme' }));
    const options = await screen.findAllByRole('option');
    expect(options).toHaveLength(3);
    expect(screen.getByRole('option', { name: 'Light' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Dark' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'System' })).toBeInTheDocument();
  });

  it('meets the 44px touch-target minimum (review-7-1 F1)', () => {
    // jsdom has no layout engine, so assert on the class contract that
    // produces the height: min-h-11 = 44px.
    renderToggle();
    const trigger = screen.getByRole('combobox', { name: 'Theme' });
    expect(trigger.className).toContain('min-h-11');
  });

  it('is axe-clean inside the theme provider', async () => {
    // Note: axe only runs on the (closed) control. With the select open, the
    // Radix portal content lands outside the container and jsdom trips the
    // "region" landmark rule — the same reason the app-shell tests axe the
    // container / dialog rather than document.body with a portal open.
    const { container } = renderToggle();
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });
});
