import { SwitchConnection, GetActiveConnections, DisconnectConnection } from '$lib/wailsjs/go/db/Service';

// Types
export interface ConnectionInfo {
    id: string;
    name: string;
    database: string;
    host: string;
    color: string;
    isActive: boolean;
}

// State - exported for direct reactive access
export const connectionState = $state({
    connections: [] as ConnectionInfo[],
    activeConnection: null as ConnectionInfo | null,
    isLoaded: false
});

// Functions
export async function refreshConnections() {
    try {
        const res = await GetActiveConnections();
        // Always update - use empty array if no data
        connectionState.connections = res.data || [];
        connectionState.activeConnection = connectionState.connections.find(c => c.isActive) || null;
        connectionState.isLoaded = true;
    } catch (e) {
        console.error('Failed to get active connections:', e);
        connectionState.isLoaded = true;
    }
}

export async function switchToConnection(connectionId: string) {
    try {
        const res = await SwitchConnection(connectionId);
        if (res.data) {
            await refreshConnections();
            return true;
        }
    } catch (e) {
        console.error('Failed to switch connection:', e);
    }
    return false;
}

export async function removeConnection(connectionId: string) {
    try {
        const res = await DisconnectConnection(connectionId);
        if (res.data) {
            await refreshConnections();
            return true;
        }
    } catch (e) {
        console.error('Failed to disconnect:', e);
    }
    return false;
}

// Convenience export for backward compatibility
export const connectionStore = {
    get connections() { return connectionState.connections; },
    get activeConnection() { return connectionState.activeConnection; },
    get isLoaded() { return connectionState.isLoaded; },
    refreshConnections,
    switchToConnection,
    removeConnection
};
