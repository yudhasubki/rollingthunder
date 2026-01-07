export const stagedChanges = $state({
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
});

export function stageDataAdd(row) {
    stagedChanges.data.added.push(row);
}

export function stageDataInsert(row: Partial<any>) {
    // Don't set ID - let database auto-generate it
    stagedChanges.data.added.push({ ...row, _isNew: true });
}

export function updateStagedRow(index: number, data: Record<string, any>) {
    // Update an existing staged row in-place
    if (index >= 0 && index < stagedChanges.data.added.length) {
        Object.assign(stagedChanges.data.added[index], data);
    }
}

export function stageDataUpdate(row) {
    stagedChanges.data.updated.push(row);
}

export function stageDataDelete(row) {
    stagedChanges.data.deleted.push(row);
}

export function stageStructureAdd(col) {
    stagedChanges.structure.added.push(col);
}

export function discardStagedChanges() {
    stagedChanges.data = { added: [], updated: [], deleted: [] };
    stagedChanges.structure = { added: [], updated: [], deleted: [] };
    stagedChanges.indices = { added: [], deleted: [] };
}

export function hasChanges() {
    return (
        stagedChanges.data.added.length > 0 ||
        stagedChanges.data.updated.length > 0 ||
        stagedChanges.data.deleted.length > 0 ||
        stagedChanges.structure.added.length > 0 ||
        stagedChanges.structure.updated.length > 0 ||
        stagedChanges.structure.deleted.length > 0 ||
        stagedChanges.indices.added.length > 0 ||
        stagedChanges.indices.deleted.length > 0
    );
}