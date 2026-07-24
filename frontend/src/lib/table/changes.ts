export type DataRow = Record<string, any>;

export const STAGED_ROW_ID = '_rtStageId';
export const STAGED_ORIGINAL = '_rtOriginal';
export const STAGED_CHANGED_COLUMNS = '_rtChangedColumns';

export function isInternalRowField(column: string): boolean {
	return column === '_isNew' || column.startsWith('_rt') || column.startsWith('temp_');
}

export function stripInternalRowFields(row: DataRow): DataRow {
	return Object.fromEntries(Object.entries(row).filter(([column]) => !isInternalRowField(column)));
}

export function getOriginalRow(row: DataRow): DataRow {
	return (row[STAGED_ORIGINAL] as DataRow | undefined) ?? row;
}

export function getChangedColumns(row: DataRow): string[] {
	const columns = row[STAGED_CHANGED_COLUMNS];
	return Array.isArray(columns)
		? columns.filter((column): column is string => typeof column === 'string')
		: [];
}

export function rowValueEquals(left: any, right: any): boolean {
	if (Object.is(left, right)) return true;
	if (left === null || right === null || left === undefined || right === undefined) {
		return false;
	}
	if (typeof left !== 'object' || typeof right !== 'object') return false;
	try {
		return JSON.stringify(left) === JSON.stringify(right);
	} catch {
		return false;
	}
}

export function findChangedColumns(original: DataRow, current: DataRow): string[] {
	const columns = new Set([...Object.keys(original), ...Object.keys(current)]);
	return [...columns].filter(
		(column) => !isInternalRowField(column) && !rowValueEquals(original[column], current[column])
	);
}

export function getRowIdentity(row: DataRow, primaryKeys: string[]): string | null {
	const stageID = row[STAGED_ROW_ID];
	if (typeof stageID === 'string' && stageID) return `staged:${stageID}`;
	if (primaryKeys.length === 0) return null;

	const original = getOriginalRow(row);
	const values = primaryKeys.map((column) => original[column]);
	if (values.some((value) => value === undefined || value === null)) return null;
	return `primary:${JSON.stringify(values)}`;
}

export function describeRow(row: DataRow, primaryKeys: string[], fallbackIndex: number): string {
	const original = getOriginalRow(row);
	if (primaryKeys.length > 0) {
		const keyDescription = primaryKeys
			.map((column) => `${column}=${formatChangeValue(original[column])}`)
			.join(', ');
		if (keyDescription) return keyDescription;
	}

	const firstValue = Object.entries(stripInternalRowFields(original)).find(
		([, value]) => value !== null && value !== undefined && value !== ''
	);
	return firstValue
		? `${firstValue[0]}=${formatChangeValue(firstValue[1])}`
		: `Row ${fallbackIndex + 1}`;
}

export function formatChangeValue(value: any): string {
	if (value === null) return 'NULL';
	if (value === undefined) return 'DEFAULT';
	if (typeof value === 'string') {
		const normalized = value.replace(/\s+/g, ' ');
		return normalized.length > 48 ? `${normalized.slice(0, 45)}…` : normalized;
	}
	if (typeof value === 'object') {
		try {
			const serialized = JSON.stringify(value);
			return serialized.length > 48 ? `${serialized.slice(0, 45)}…` : serialized;
		} catch {
			return '[value]';
		}
	}
	return String(value);
}
