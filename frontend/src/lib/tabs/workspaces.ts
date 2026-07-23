import type { Tab } from '../models/Tab';

export interface ConnectionWorkspace {
	tabs: Tab[];
	activeTabId: string | null;
}

export function findWorkspaceForTab(
	workspaces: Record<string, ConnectionWorkspace>,
	tabId: string
): ConnectionWorkspace | null {
	return (
		Object.values(workspaces).find((workspace) => workspace.tabs.some((tab) => tab.id === tabId)) ??
		null
	);
}

export function findTableTabForConnection(
	workspaces: Record<string, ConnectionWorkspace>,
	connectionId: string,
	schema: string,
	table: string
): Tab | undefined {
	return workspaces[connectionId]?.tabs.find(
		(tab) => tab.kind === 'table' && tab.schema === schema && tab.table === table
	);
}
