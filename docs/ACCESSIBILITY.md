# Accessibility

Rolling Thunder treats keyboard access, visible focus, readable status updates, and motion
preferences as release requirements. The desktop shell targets WCAG 2.2 AA behavior where the
native Wails webview and current component set allow it.

## Implemented behavior

- A keyboard-only skip link moves focus to the active page's `main-content` region.
- Buttons, links, inputs, tabs, grid controls, and custom toolbar controls retain a visible
  `:focus-visible` indicator.
- Blocking dialogs and drawers trap `Tab`/`Shift+Tab`, support `Escape` when an operation is not
  running, and restore focus to the previously focused control.
- Dialogs expose `role="dialog"`, `aria-modal="true"`, and a labelled heading.
- The status bar is a polite live region. Errors and destructive confirmations use alert or status
  semantics without relying on color alone.
- The activity console exposes its expanded state and associated region.
- Connection health has a text label in addition to its colored indicator.
- Animations and smooth scrolling are reduced when `prefers-reduced-motion: reduce` is active.
- Icon-only controls have accessible names; decorative icons are hidden where needed.

The command palette lists the active shortcuts. Defaults use `Mod` for Command on macOS and Control
on Windows/Linux:

| Action             | Default                                    |
| ------------------ | ------------------------------------------ |
| Command palette    | `Mod+K`                                    |
| New query          | `Mod+N`                                    |
| Run statement      | `Mod+Enter`                                |
| Format SQL         | `Shift+Alt+F`                              |
| Explain query      | `Mod+Shift+E`                              |
| Save named query   | `Mod+Shift+S`                              |
| Import data        | `Mod+Shift+I`                              |
| Previous/next tab  | `Mod+Alt+ArrowLeft` / `Mod+Alt+ArrowRight` |
| Activity console   | `Mod+J`                                    |
| Manage connections | `Mod+,`                                    |

Shortcuts can be changed from Query tooling settings.

## Automated checks

`npm test` contains accessibility contract tests for:

- the skip-link target and live status region;
- shared focus trapping and modal semantics on blocking dialogs;
- visible-focus and reduced-motion CSS.

`npm run lint` enforces repository formatting and JavaScript rules. The production Svelte build
runs compiler accessibility diagnostics as part of the frontend gate. These checks prevent common
regressions but do not replace assistive-technology testing.

## Manual release audit

Run this checklist on macOS, Windows, and Linux before a stable release:

1. Start with the mouse disconnected. Reach the skip link, header, connection selector, sidebar,
   tab strip, active content, activity console, and status bar in a predictable order.
2. Open every modal and drawer. Confirm focus enters it, wraps in both directions, cannot reach the
   obscured page, closes with `Escape` when safe, and returns to the invoking control.
3. Use arrow keys in tab lists, menus, comboboxes, and the schema explorer. Confirm the active item
   remains visible when a long tab strip scrolls.
4. Run a query, cancel one, trigger a validation error, degrade a connection, and reconnect it.
   Confirm VoiceOver, Narrator, or Orca announces the important status once without repeated noise.
5. Zoom the webview to 200%. Verify connection management, Structure, table data, query results, and
   confirmation dialogs do not hide required controls.
6. Check light, dark, and high-contrast OS modes. Text, focus rings, selected rows, errors, and
   disabled states must remain distinguishable without color alone.
7. Enable reduced motion at the OS level. Dialogs, spinners, tab scrolling, and drawer transitions
   must not create unnecessary motion.
8. Inspect table data with a screen reader. Column names, row selection, row actions, primary/foreign
   key markers, pagination, and the detail drawer must have useful names.

Record the operating system, webview version, assistive technology, failed step, and reproduction
details in the release issue. A failed keyboard trap, hidden destructive confirmation, or
unannounced blocking error is release-blocking.

## Reporting an accessibility issue

Open an issue with the affected screen, input method or assistive technology, operating system,
expected announcement or navigation, and a minimal reproduction. Do not include credentials,
queries containing private data, or exported diagnostic archives in a public issue.
