import type { SortingState } from '@tanstack/table-core';

export function getNextSortingState(
	current: SortingState,
	column: string,
	multiColumn: boolean
): SortingState {
	const existingIndex = current.findIndex((sort) => sort.id === column);
	const existing = existingIndex >= 0 ? current[existingIndex] : undefined;

	if (multiColumn) {
		if (!existing) {
			return [...current, { id: column, desc: false }];
		}
		if (!existing.desc) {
			return current.map((sort, index) =>
				index === existingIndex ? { ...sort, desc: true } : sort
			);
		}
		return current.filter((_, index) => index !== existingIndex);
	}

	if (!existing) {
		return [{ id: column, desc: false }];
	}
	if (!existing.desc) {
		return [{ id: column, desc: true }];
	}
	return [];
}
