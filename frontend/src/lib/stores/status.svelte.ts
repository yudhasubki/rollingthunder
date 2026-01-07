// src/lib/stores/status.store.svelte.ts

import { database } from "$lib/wailsjs/go/models";

interface LogEntry {
    timestamp: Date;
    message: string;
    level: 'info' | 'warn' | 'error';
}

const state = $state({
    status: '',
    level: 'info' as 'info' | 'warn' | 'error',
    databaseInfo: null as database.Info | null,
    consoleLogs: [] as LogEntry[],
    showConsole: false
});

export function getStatus() { return state.status; }
export function getLevel() { return state.level; }
export function getDatabaseInfo() { return state.databaseInfo; }
export function getConsoleLogs() { return state.consoleLogs; }
export function getShowConsole() { return state.showConsole; }

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

export function toggleConsole() {
    state.showConsole = !state.showConsole;
}

export function addConsoleLog(message: string, level: 'info' | 'warn' | 'error' = 'info') {
    state.consoleLogs = [
        { timestamp: new Date(), message, level },
        ...state.consoleLogs.slice(0, 99) // Keep last 100 logs
    ];
    // Auto-show console on error
    if (level === 'error') {
        state.showConsole = true;
    }
}

export function clearConsoleLogs() {
    state.consoleLogs = [];
}

// Update methods remain the same
export function updateStatus(
    status: string,
    level: 'info' | 'warn' | 'error' = 'info'
) {
    setStatus(status);
    setLevel(level);
    // Also add to console log if it's a meaningful message
    if (status) {
        addConsoleLog(status, level);
    }
}

export function updateLevel(newLevel: 'info' | 'warn' | 'error') {
    state.level = newLevel;
}

export function updateDatabaseInfo(info: database.Info | null) {
    state.databaseInfo = info;
}