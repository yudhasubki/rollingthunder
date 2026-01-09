import type { Tab } from '$lib/models/Tab';

// Create a state object
const state = $state({
  tabs: [] as Tab[],
  activeTabId: null as string | null,
  activeSubTab: 'structure' as 'structure' | 'data'
});

// Create the derived activeTab
const _activeTab = $derived.by(() => {
  if (state.tabs.length === 0) return null;
  return state.tabs.find(tab => tab.id === state.activeTabId) ?? null
})

// Public Store
export const tabsStore = {
  // Getters
  get tabs() { return state.tabs },
  get activeTabId() { return state.activeTabId },
  get activeSubTab() { return state.activeSubTab },
  get activeTab() {

    if (_activeTab) {
      return _activeTab
    }
    return null;
  },

  // Actions
  newQueryTab() {
    const id = crypto.randomUUID();
    const tab: Tab = {
      id,
      title: 'SQL Query',
      kind: 'query',
      sql: '',
      level: 'info'
    };
    state.tabs = [...state.tabs, tab];
    state.activeTabId = id;
  },

  newTableTab(schema: string, table: string) {
    const id = crypto.randomUUID();
    const tab: Tab = {
      id,
      title: `${schema}.${table}`,
      kind: 'table',
      schema,
      table,
      level: 'info'
    };
    state.tabs = [...state.tabs, tab];
    state.activeTabId = id;

  },

  newCreateTableTab(schema: string) {
    const id = crypto.randomUUID();
    const tab: Tab = {
      id,
      title: 'New Table',
      kind: 'createTable',
      schema,
      level: 'info'
    };
    state.tabs = [...state.tabs, tab];
    state.activeTabId = id;
  },

  closeTab(id: string) {
    state.tabs = state.tabs.filter(tab => tab.id !== id);
    if (state.activeTabId === id) {
      state.activeTabId = state.tabs.at(-1)?.id ?? null;
    }
  },

  setActive(id: string) {
    state.activeTabId = id;
  },

  setActiveSubTab(tab: any) {
    state.activeSubTab = tab;
  },

  updateTab(id: string, patch: Partial<Tab>) {
    state.tabs = state.tabs.map(t => (t.id === id ? { ...t, ...patch } : t));
  },

  findTableTab(schema: string, table: string) {
    return state.tabs.find(t =>
      t.kind === 'table' &&
      t.schema === schema &&
      t.table === table
    );
  }
};

// Export individual properties (excluding activeTab to avoid conflict)
export const { tabs, activeTabId, activeSubTab } = state;
export const { newQueryTab, newTableTab, closeTab, setActive, updateTab, findTableTab, setActiveSubTab } = tabsStore;