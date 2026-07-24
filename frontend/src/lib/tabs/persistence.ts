import type { Tab } from '$lib/models/Tab';

export const WORKSPACE_STORAGE_KEY = 'rollingthunder.workspace';
export const WORKSPACE_STORAGE_VERSION = 1;

export interface PersistedQueryTab {
	id: string;
	title: string;
	kind: 'query';
	sql: string;
	savedQueryId?: string;
}

export interface PersistedWorkspace {
	tabs: PersistedQueryTab[];
	activeTabId: string | null;
}

export interface PersistedWorkspaceEnvelope {
	version: number;
	workspaces: Record<string, PersistedWorkspace>;
}

export interface WorkspaceConnectionIdentity {
	profileId?: string;
	name: string;
	driver: string;
	host: string;
	database: string;
}

export function getConnectionWorkspaceKey(connection: WorkspaceConnectionIdentity): string {
	if (connection.profileId) return `profile:${connection.profileId}`;
	return [
		'connection',
		connection.driver || 'postgres',
		connection.host || 'local',
		connection.database || 'default',
		connection.name || 'unnamed'
	]
		.map((part) => encodeURIComponent(part.trim().toLowerCase()))
		.join(':');
}

export function emptyWorkspaceEnvelope(): PersistedWorkspaceEnvelope {
	return {
		version: WORKSPACE_STORAGE_VERSION,
		workspaces: {}
	};
}

export function parseWorkspaceEnvelope(raw: string | null): PersistedWorkspaceEnvelope {
	if (!raw) return emptyWorkspaceEnvelope();
	try {
		const parsed = JSON.parse(raw);
		if (
			!parsed ||
			parsed.version !== WORKSPACE_STORAGE_VERSION ||
			typeof parsed.workspaces !== 'object' ||
			Array.isArray(parsed.workspaces)
		) {
			return emptyWorkspaceEnvelope();
		}

		const workspaces: Record<string, PersistedWorkspace> = {};
		for (const [key, candidate] of Object.entries(parsed.workspaces as Record<string, any>)) {
			const tabs = Array.isArray(candidate?.tabs)
				? candidate.tabs
						.filter(
							(tab: any) =>
								tab &&
								tab.kind === 'query' &&
								typeof tab.id === 'string' &&
								typeof tab.sql === 'string'
						)
						.map(
							(tab: any): PersistedQueryTab => ({
								id: tab.id,
								title: typeof tab.title === 'string' && tab.title ? tab.title : 'SQL Query',
								kind: 'query',
								sql: tab.sql,
								savedQueryId: typeof tab.savedQueryId === 'string' ? tab.savedQueryId : undefined
							})
						)
				: [];
			const activeTabId =
				typeof candidate?.activeTabId === 'string' &&
				tabs.some((tab: PersistedQueryTab) => tab.id === candidate.activeTabId)
					? candidate.activeTabId
					: (tabs.at(-1)?.id ?? null);
			workspaces[key] = { tabs, activeTabId };
		}
		return {
			version: WORKSPACE_STORAGE_VERSION,
			workspaces
		};
	} catch {
		return emptyWorkspaceEnvelope();
	}
}

export function persistableWorkspace(tabs: Tab[], activeTabId: string | null): PersistedWorkspace {
	const queryTabs = tabs
		.filter((tab): tab is Tab & { kind: 'query' } => tab.kind === 'query')
		.map(
			(tab): PersistedQueryTab => ({
				id: tab.id,
				title: tab.title || 'SQL Query',
				kind: 'query',
				sql: tab.sql || '',
				savedQueryId: tab.savedQueryId
			})
		);
	return {
		tabs: queryTabs,
		activeTabId: queryTabs.some((tab) => tab.id === activeTabId)
			? activeTabId
			: (queryTabs.at(-1)?.id ?? null)
	};
}

export function restoreQueryTabs(
	connectionId: string,
	workspace: PersistedWorkspace | undefined
): Tab[] {
	if (!workspace) return [];
	return workspace.tabs.map((tab) => ({
		...tab,
		connectionId,
		level: 'info'
	}));
}
