import { writable, get } from 'svelte/store';
import type { Tab } from '$lib/models/Tab';
import { LogInfo } from '$lib/wailsjs/runtime/runtime';

export const tabs         = writable<Tab[]>([]);
export const activeTabId  = writable<string | null>(null);
export const activeSubTab = writable<'structure' | 'data'>('structure');

export function newQueryTab() {
    const id = crypto.randomUUID();
    const tab: Tab = {
        id,
        title: 'New Query',
        kind: 'query',
        sql: '',
        createdAt: Date.now(),
        level: 'info'
    };
    tabs.update(t => [...t, tab]);
    activeTabId.set(id);
}

export function newTableTab(schema: string, table: string) {
    const id = crypto.randomUUID();
    const tab: Tab = {
        id: id,
        title: `${schema}.${table}`,
        kind: 'table',
        schema: schema,
        table: table,
        createdAt: Date.now(),
        level: 'info'
    };
    tabs.update(t => [...t, tab]);
    activeTabId.set(id);
}

export function closeTab(id: string) {
    tabs.update(t => t.filter(tab => tab.id !== id));
    if (get(activeTabId) === id) {
        activeTabId.set(get(tabs).at(-1)?.id ?? null);
    }
}

export function setActive(id: string) {
    activeTabId.set(id);
}

export function updateTab(id: string, patch: Partial<Tab>) {
    tabs.update(ts => ts.map(t => (t.id === id ? { ...t, ...patch } : t)));
}