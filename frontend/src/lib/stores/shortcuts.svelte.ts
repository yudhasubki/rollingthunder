import {
	defaultShortcuts,
	parseShortcuts,
	SHORTCUT_STORAGE_KEY,
	type CommandID,
	type ShortcutMap
} from '$lib/commands/shortcuts';

let hydrated = false;
let shortcuts = $state<ShortcutMap>({ ...defaultShortcuts });

function hydrate(): void {
	if (hydrated || typeof window === 'undefined') return;
	hydrated = true;
	shortcuts = parseShortcuts(localStorage.getItem(SHORTCUT_STORAGE_KEY));
}

function persist(): void {
	if (typeof window === 'undefined') return;
	localStorage.setItem(SHORTCUT_STORAGE_KEY, JSON.stringify(shortcuts));
}

export const shortcutStore = {
	get bindings(): ShortcutMap {
		hydrate();
		return shortcuts;
	},
	get(command: CommandID): string {
		hydrate();
		return shortcuts[command];
	},
	set(command: CommandID, shortcut: string) {
		hydrate();
		shortcuts = { ...shortcuts, [command]: shortcut };
		persist();
	},
	reset() {
		shortcuts = { ...defaultShortcuts };
		persist();
	}
};
