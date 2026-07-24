import {
	findChangedColumns,
	getOriginalRow,
	getRowIdentity,
	STAGED_CHANGED_COLUMNS,
	STAGED_ORIGINAL,
	STAGED_ROW_ID,
	stripInternalRowFields,
	type DataRow
} from '$lib/table/changes';

export type StagedRow = DataRow;

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
	ensureStagedChanges(tabId).data.added.push({
		...row,
		_isNew: true,
		[STAGED_ROW_ID]: crypto.randomUUID()
	});
}

export function updateStagedInsert(tabId: string, row: StagedRow, nextRow: StagedRow) {
	const staged = ensureStagedChanges(tabId).data.added;
	const stageID = row[STAGED_ROW_ID];
	const existingIndex = staged.findIndex((candidate) => candidate[STAGED_ROW_ID] === stageID);
	if (existingIndex >= 0) {
		staged[existingIndex] = {
			...nextRow,
			_isNew: true,
			[STAGED_ROW_ID]: stageID
		};
	}
}

export function stageDataUpdate(
	tabId: string,
	row: StagedRow,
	nextRow: StagedRow,
	primaryKeys: string[]
) {
	const staged = ensureStagedChanges(tabId).data.updated;
	const original = stripInternalRowFields(getOriginalRow(row));
	const current = stripInternalRowFields(nextRow);
	const changedColumns = findChangedColumns(original, current);
	const identity = getRowIdentity(original, primaryKeys);
	const existingIndex = staged.findIndex(
		(candidate) => getRowIdentity(candidate, primaryKeys) === identity
	);

	if (changedColumns.length === 0) {
		if (existingIndex >= 0) staged.splice(existingIndex, 1);
		return;
	}

	const stagedRow = {
		...current,
		[STAGED_ORIGINAL]: original,
		[STAGED_CHANGED_COLUMNS]: changedColumns
	};
	if (identity !== null && existingIndex >= 0) {
		staged[existingIndex] = stagedRow;
	} else {
		staged.push(stagedRow);
	}
}

export function stageDataDelete(tabId: string, row: StagedRow, primaryKeys: string[]) {
	const changes = ensureStagedChanges(tabId).data;
	if (row._isNew) {
		const stageID = row[STAGED_ROW_ID];
		const addedIndex = changes.added.findIndex((candidate) => candidate[STAGED_ROW_ID] === stageID);
		if (addedIndex >= 0) changes.added.splice(addedIndex, 1);
		return;
	}

	const original = stripInternalRowFields(getOriginalRow(row));
	const identity = getRowIdentity(original, primaryKeys);
	changes.updated = changes.updated.filter(
		(candidate) => getRowIdentity(candidate, primaryKeys) !== identity
	);
	if (!changes.deleted.some((candidate) => getRowIdentity(candidate, primaryKeys) === identity)) {
		changes.deleted.push(original);
	}
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
