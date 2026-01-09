// Sidebar store for managing table list state and refresh
// Allows other components to trigger table list refresh

export const sidebarState = $state({
    refreshTables: null as (() => Promise<void>) | null,
    addTable: null as ((tableName: string) => void) | null,
    removeTable: null as ((tableName: string) => void) | null
});

export function setSidebarRefresh(fn: (() => Promise<void>) | null) {
    sidebarState.refreshTables = fn;
}

export function setSidebarAddTable(fn: ((tableName: string) => void) | null) {
    sidebarState.addTable = fn;
}

export function setSidebarRemoveTable(fn: ((tableName: string) => void) | null) {
    sidebarState.removeTable = fn;
}

// Convenience functions to call sidebar actions
export async function refreshSidebarTables() {
    if (sidebarState.refreshTables) {
        await sidebarState.refreshTables();
    }
}

export function addTableToSidebar(tableName: string) {
    if (sidebarState.addTable) {
        sidebarState.addTable(tableName);
    }
}

export function removeTableFromSidebar(tableName: string) {
    if (sidebarState.removeTable) {
        sidebarState.removeTable(tableName);
    }
}
