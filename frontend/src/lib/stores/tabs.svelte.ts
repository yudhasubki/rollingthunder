import type { Tab } from '$lib/models/Tab';
import type { database } from '$lib/wailsjs/go/models';
import { clearTabState } from '$lib/stores/staged.svelte';
import {
	findTableTabForConnection,
	findWorkspaceForTab,
	type ConnectionWorkspace
} from '$lib/tabs/workspaces';

const state = $state({
	activeConnectionId: null as string | null,
	workspaces: {} as Record<string, ConnectionWorkspace>
});

function requireConnectionId(connectionId: string): string {
	if (!connectionId) {
		throw new Error('A connection ID is required to create a workspace tab');
	}
	return connectionId;
}

function getWorkspace(connectionId: string, create = true): ConnectionWorkspace | null {
	const id = requireConnectionId(connectionId);
	const existing = state.workspaces[id];
	if (existing || !create) return existing ?? null;

	const workspace: ConnectionWorkspace = {
		tabs: [],
		activeTabId: null
	};
	state.workspaces[id] = workspace;
	return workspace;
}

function getActiveWorkspace(): ConnectionWorkspace | null {
	if (!state.activeConnectionId) return null;
	return getWorkspace(state.activeConnectionId, false);
}

function appendTab(connectionId: string, tab: Tab): string {
	const workspace = getWorkspace(connectionId);
	if (!workspace) throw new Error('Connection workspace is unavailable');

	workspace.tabs = [...workspace.tabs, tab];
	workspace.activeTabId = tab.id;
	return tab.id;
}

const activeTab = $derived.by(() => {
	const workspace = getActiveWorkspace();
	if (!workspace?.activeTabId) return null;
	return workspace.tabs.find((tab) => tab.id === workspace.activeTabId) ?? null;
});

export const tabsStore = {
	get activeConnectionId() {
		return state.activeConnectionId;
	},

	get tabs() {
		return getActiveWorkspace()?.tabs ?? [];
	},

	get allTabs() {
		return Object.values(state.workspaces).flatMap((workspace) => workspace.tabs);
	},

	get activeTabId() {
		return getActiveWorkspace()?.activeTabId ?? null;
	},

	get activeTab() {
		return activeTab;
	},

	setActiveConnection(connectionId: string | null) {
		state.activeConnectionId = connectionId;
		if (!connectionId) return;

		const workspace = getWorkspace(connectionId);
		if (workspace && !workspace.activeTabId && workspace.tabs.length > 0) {
			workspace.activeTabId = workspace.tabs.at(-1)?.id ?? null;
		}
	},

	removeWorkspace(connectionId: string) {
		if (!connectionId || !state.workspaces[connectionId]) return;
		for (const tab of state.workspaces[connectionId].tabs) {
			clearTabState(tab.id);
		}
		delete state.workspaces[connectionId];
		if (state.activeConnectionId === connectionId) {
			state.activeConnectionId = null;
		}
	},

	hasOpenTabs(connectionId: string) {
		return Boolean(getWorkspace(connectionId, false)?.tabs.length);
	},

	newQueryTab(connectionId: string) {
		const id = crypto.randomUUID();
		return appendTab(connectionId, {
			id,
			connectionId,
			title: 'SQL Query',
			kind: 'query',
			sql: '',
			level: 'info'
		});
	},

	newQueryTabWithContent(connectionId: string, sql: string, title?: string) {
		const id = crypto.randomUUID();
		return appendTab(connectionId, {
			id,
			connectionId,
			title: title || 'SQL Query',
			kind: 'query',
			sql,
			level: 'info'
		});
	},

	newTableTab(connectionId: string, schema: string, table: string) {
		const existing = findTableTabForConnection(state.workspaces, connectionId, schema, table);
		if (existing) {
			const workspace = getWorkspace(connectionId);
			if (workspace) workspace.activeTabId = existing.id;
			return existing.id;
		}

		const id = crypto.randomUUID();
		return appendTab(connectionId, {
			id,
			connectionId,
			title: `${schema}.${table}`,
			kind: 'table',
			schema,
			table,
			level: 'info'
		});
	},

	newCreateTableTab(connectionId: string, schema: string) {
		const id = crypto.randomUUID();
		return appendTab(connectionId, {
			id,
			connectionId,
			title: 'New Table',
			kind: 'createTable',
			schema,
			level: 'info'
		});
	},

	newSchemaDiagramTab(connectionId: string, schema: string) {
		const workspace = getWorkspace(connectionId);
		if (!workspace) throw new Error('Connection workspace is unavailable');

		const existing = workspace.tabs.find(
			(tab) => tab.kind === 'schemaDiagram' && tab.schema === schema
		);
		if (existing) {
			workspace.activeTabId = existing.id;
			return existing.id;
		}

		const id = crypto.randomUUID();
		return appendTab(connectionId, {
			id,
			connectionId,
			title: `${schema} diagram`,
			kind: 'schemaDiagram',
			schema,
			level: 'info'
		});
	},

	newDatabaseObjectTab(connectionId: string, reference: database.ObjectReference) {
		const workspace = getWorkspace(connectionId);
		if (!workspace) throw new Error('Connection workspace is unavailable');

		const existing = workspace.tabs.find(
			(tab) =>
				tab.kind === 'databaseObject' &&
				((reference.id && tab.objectId === reference.id) ||
					(!reference.id &&
						tab.objectKind === reference.kind &&
						tab.schema === reference.schema &&
						tab.objectName === reference.name &&
						tab.objectSignature === reference.signature))
		);
		if (existing) {
			workspace.activeTabId = existing.id;
			return existing.id;
		}

		const signature = reference.signature ? `(${reference.signature})` : '';
		const id = crypto.randomUUID();
		return appendTab(connectionId, {
			id,
			connectionId,
			title: `${reference.name}${signature}`,
			kind: 'databaseObject',
			schema: reference.schema,
			objectId: reference.id,
			objectKind: reference.kind,
			objectName: reference.name,
			objectSignature: reference.signature,
			parentSchema: reference.parentSchema,
			parentName: reference.parentName,
			level: 'info'
		});
	},

	closeTab(id: string) {
		const workspace = findWorkspaceForTab(state.workspaces, id);
		if (!workspace) return;

		const closingIndex = workspace.tabs.findIndex((tab) => tab.id === id);
		workspace.tabs = workspace.tabs.filter((tab) => tab.id !== id);
		clearTabState(id);
		if (workspace.activeTabId === id) {
			const nextIndex = Math.min(closingIndex, workspace.tabs.length - 1);
			workspace.activeTabId = nextIndex >= 0 ? workspace.tabs[nextIndex].id : null;
		}
	},

	setActive(id: string) {
		const workspace = getActiveWorkspace();
		if (workspace?.tabs.some((tab) => tab.id === id)) {
			workspace.activeTabId = id;
		}
	},

	updateTab(id: string, patch: Partial<Tab>) {
		const workspace = findWorkspaceForTab(state.workspaces, id);
		if (!workspace) return;
		workspace.tabs = workspace.tabs.map((tab) =>
			tab.id === id ? { ...tab, ...patch, id: tab.id, connectionId: tab.connectionId } : tab
		);
	},

	findTableTab(connectionId: string, schema: string, table: string) {
		return findTableTabForConnection(state.workspaces, connectionId, schema, table);
	}
};

export const {
	newQueryTab,
	newQueryTabWithContent,
	newTableTab,
	newSchemaDiagramTab,
	newDatabaseObjectTab,
	closeTab,
	setActive,
	updateTab,
	findTableTab
} = tabsStore;
