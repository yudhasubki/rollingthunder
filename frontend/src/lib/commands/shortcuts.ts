import { APPLICATION_STORAGE } from '../config/application.ts';

export type CommandID =
	| 'commandPalette'
	| 'newQuery'
	| 'runQuery'
	| 'formatQuery'
	| 'explainQuery'
	| 'saveQuery'
	| 'importData'
	| 'nextTab'
	| 'previousTab'
	| 'toggleConsole'
	| 'manageConnections';

export type ShortcutMap = Record<CommandID, string>;

export const SHORTCUT_STORAGE_KEY = APPLICATION_STORAGE.shortcuts;

export interface CommandDefinition {
	id: CommandID;
	label: string;
	description: string;
	group: 'Workspace' | 'Query' | 'Navigation';
	queryOnly?: boolean;
}

export const commandDefinitions: CommandDefinition[] = [
	{
		id: 'commandPalette',
		label: 'Open command palette',
		description: 'Search actions and keyboard shortcuts',
		group: 'Workspace'
	},
	{
		id: 'newQuery',
		label: 'New query',
		description: 'Open an empty SQL query tab',
		group: 'Workspace'
	},
	{
		id: 'runQuery',
		label: 'Run current statement',
		description: 'Execute the selection or statement at the cursor',
		group: 'Query',
		queryOnly: true
	},
	{
		id: 'formatQuery',
		label: 'Format SQL',
		description: 'Format the selection or complete query',
		group: 'Query',
		queryOnly: true
	},
	{
		id: 'explainQuery',
		label: 'Explain query',
		description: 'Build an estimated plan without executing the query',
		group: 'Query',
		queryOnly: true
	},
	{
		id: 'saveQuery',
		label: 'Save named query',
		description: 'Save the current SQL locally',
		group: 'Query',
		queryOnly: true
	},
	{
		id: 'importData',
		label: 'Import CSV or JSON',
		description: 'Load a file into an existing or new table',
		group: 'Workspace'
	},
	{
		id: 'nextTab',
		label: 'Next tab',
		description: 'Activate the tab to the right',
		group: 'Navigation'
	},
	{
		id: 'previousTab',
		label: 'Previous tab',
		description: 'Activate the tab to the left',
		group: 'Navigation'
	},
	{
		id: 'toggleConsole',
		label: 'Toggle activity console',
		description: 'Show or hide database activity',
		group: 'Workspace'
	},
	{
		id: 'manageConnections',
		label: 'Manage connections',
		description: 'Open connection profiles without leaving the workspace',
		group: 'Workspace'
	}
];

export const defaultShortcuts: ShortcutMap = {
	commandPalette: 'Mod+K',
	newQuery: 'Mod+N',
	runQuery: 'Mod+Enter',
	formatQuery: 'Shift+Alt+F',
	explainQuery: 'Mod+Shift+E',
	saveQuery: 'Mod+Shift+S',
	importData: 'Mod+Shift+I',
	nextTab: 'Mod+Alt+ArrowRight',
	previousTab: 'Mod+Alt+ArrowLeft',
	toggleConsole: 'Mod+J',
	manageConnections: 'Mod+,'
};

export function normalizeShortcut(value: string): string {
	const parts = value
		.split('+')
		.map((part) => part.trim())
		.filter(Boolean);
	const modifiers = ['Mod', 'Ctrl', 'Meta', 'Alt', 'Shift'];
	const normalizedModifiers = modifiers.filter((modifier) =>
		parts.some((part) => part.toLowerCase() === modifier.toLowerCase())
	);
	const key = parts.find(
		(part) => !modifiers.some((modifier) => modifier.toLowerCase() === part.toLowerCase())
	);
	const normalizedKey = key?.length === 1 ? key.toUpperCase() : key;
	return [...normalizedModifiers, normalizedKey || ''].filter(Boolean).join('+');
}

export function shortcutFromKeyboardEvent(
	event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>
): string {
	const parts: string[] = [];
	if (event.ctrlKey || event.metaKey) parts.push('Mod');
	if (event.altKey) parts.push('Alt');
	if (event.shiftKey) parts.push('Shift');
	const key =
		event.key.length === 1 ? event.key.toUpperCase() : event.key === ' ' ? 'Space' : event.key;
	if (!['Control', 'Meta', 'Alt', 'Shift'].includes(key)) parts.push(key);
	return normalizeShortcut(parts.join('+'));
}

export function matchesShortcut(
	event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>,
	shortcut: string
): boolean {
	return shortcutFromKeyboardEvent(event) === normalizeShortcut(shortcut);
}

export function parseShortcuts(raw: string | null): ShortcutMap {
	if (!raw) return { ...defaultShortcuts };
	try {
		const parsed = JSON.parse(raw);
		return Object.fromEntries(
			Object.entries(defaultShortcuts).map(([command, fallback]) => [
				command,
				typeof parsed?.[command] === 'string' && parsed[command]
					? normalizeShortcut(parsed[command])
					: fallback
			])
		) as ShortcutMap;
	} catch {
		return { ...defaultShortcuts };
	}
}
