// src/lib/stores/status.store.svelte.ts

import { database } from "$lib/wailsjs/go/models";

const state = $state({
    status: '',
    level: 'info' as 'info' | 'warn' | 'error',
    databaseInfo: null as database.Info | null
});

export function getStatus() { return state.status; }
export function getLevel() { return state.level; }
export function getDatabaseInfo() { return state.databaseInfo; }

// For computed values, export functions
export function getSegments() {
    return state.databaseInfo
        ? [
            state.databaseInfo.engine,
            state.databaseInfo.version,
            state.databaseInfo.database,
            ...(state.status ? [state.status] : [])
          ]
        : [];
}

export function setStatus(newStatus: string) {
    state.status = newStatus;
}

export function setLevel(newLevel: 'info' | 'warn' | 'error') {
    state.level = newLevel;
}

export function setDatabaseInfo(info: database.Info | null) {
    state.databaseInfo = info;
}

// Update methods remain the same
export function updateStatus(
    status: string, 
    level: 'info' | 'warn' | 'error' = 'info'
) {
    setStatus(status);
    setLevel(level);
}

export function updateLevel(newLevel: 'info' | 'warn' | 'error') {
    state.level = newLevel;
}

export function updateDatabaseInfo(info: database.Info | null) {
    state.databaseInfo = info;
}