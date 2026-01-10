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

// State
const state = $state({
    connections: [] as ConnectionInfo[],
    activeConnection: null as ConnectionInfo | null
});

// Functions
export async function refreshConnections() {
    try {
        const res = await GetActiveConnections();
        if (res.data) {
            state.connections = res.data;
            state.activeConnection = state.connections.find(c => c.isActive) || null;
        }
    } catch (e) {
        console.error('Failed to get active connections:', e);
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

// Export store object
export const connectionStore = {
    get connections() { return state.connections; },
    get activeConnection() { return state.activeConnection; },
    refreshConnections,
    switchToConnection,
    removeConnection
};
