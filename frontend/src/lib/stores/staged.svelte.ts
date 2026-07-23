type StagedRow = Record<string, any>;

export interface TabStagedChanges {
	data: {
		added: StagedRow[];
		updated: StagedRow[];
		deleted: StagedRow[];
	};
	structure: {
		added: StagedRow[];
		updated: StagedRow[];
		deleted: StagedRow[];
	};
	indices: {
		added: StagedRow[];
		deleted: StagedRow[];
	};
}

function createEmptyChanges(): TabStagedChanges {
	return {
		data: {
			added: [],
			updated: [],
			deleted: []
		},
		structure: {
			added: [],
			updated: [],
			deleted: []
		},
		indices: {
			added: [],
			deleted: []
		}
	};
}

const changesByTab = $state<Record<string, TabStagedChanges>>({});
const createTableSubmits = $state<Record<string, (() => Promise<boolean>) | undefined>>({});

export function getStagedChanges(tabId: string): TabStagedChanges {
	return (tabId && changesByTab[tabId]) || createEmptyChanges();
}

function ensureStagedChanges(tabId: string): TabStagedChanges {
	if (!tabId) throw new Error('A tab ID is required to stage changes');
	if (!changesByTab[tabId]) {
		changesByTab[tabId] = createEmptyChanges();
	}
	return changesByTab[tabId];
}

export function stageDataInsert(tabId: string, row: Partial<StagedRow>) {
	ensureStagedChanges(tabId).data.added.push({ ...row, _isNew: true });
}

export function stageDataUpdate(tabId: string, row: StagedRow) {
	const staged = ensureStagedChanges(tabId).data.updated;
	const rowId = row.id ?? row._id;
	const existingIndex = staged.findIndex((candidate) => (candidate.id ?? candidate._id) === rowId);
	if (rowId !== undefined && existingIndex >= 0) {
		staged[existingIndex] = row;
	} else {
		staged.push(row);
	}
}

export function stageDataDelete(tabId: string, row: StagedRow) {
	ensureStagedChanges(tabId).data.deleted.push(row);
}

export function stageStructureAdd(tabId: string, column: StagedRow) {
	ensureStagedChanges(tabId).structure.added.push(column);
}

export function discardStagedChanges(tabId: string) {
	if (!tabId) return;
	changesByTab[tabId] = createEmptyChanges();
}

export function hasChanges(tabId: string | null | undefined) {
	if (!tabId) return false;
	const staged = getStagedChanges(tabId);
	return (
		staged.data.added.length > 0 ||
		staged.data.updated.length > 0 ||
		staged.data.deleted.length > 0 ||
		staged.structure.added.length > 0 ||
		staged.structure.updated.length > 0 ||
		staged.structure.deleted.length > 0 ||
		staged.indices.added.length > 0 ||
		staged.indices.deleted.length > 0
	);
}

export function setCreateTableSubmit(tabId: string, submit: (() => Promise<boolean>) | null) {
	if (!tabId) return;
	if (submit) {
		createTableSubmits[tabId] = submit;
	} else {
		delete createTableSubmits[tabId];
	}
}

export function getCreateTableSubmit(tabId: string | null | undefined) {
	return tabId ? (createTableSubmits[tabId] ?? null) : null;
}

export function clearTabState(tabId: string) {
	if (!tabId) return;
	delete changesByTab[tabId];
	delete createTableSubmits[tabId];
}
