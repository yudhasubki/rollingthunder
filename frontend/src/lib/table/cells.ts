export type CellKind = 'null' | 'boolean' | 'number' | 'json' | 'datetime' | 'binary' | 'text';

export interface CellPresentation {
	kind: CellKind;
	text: string;
	title: string;
	booleanValue?: boolean;
}

export interface ColumnTypeMetadata {
	data_type?: string | null;
	length?: number | null;
	is_enum?: boolean | null;
	type_schema?: string | null;
	type_name?: string | null;
}

const BOOLEAN_TYPE = /\b(bool|boolean)\b/i;
const NUMBER_TYPE =
	/\b(smallint|integer|bigint|int2|int4|int8|serial|bigserial|decimal|numeric|real|double|float|money)\b/i;
const JSON_TYPE = /\b(json|jsonb)\b/i;
const DATETIME_TYPE = /\b(date|time|timestamp|timestamptz|datetime)\b/i;
const BINARY_TYPE = /\b(bytea|blob|binary|varbinary)\b/i;

function stringifyValue(value: any): string {
	if (typeof value === 'string') return value;
	if (typeof value === 'object') {
		try {
			return JSON.stringify(value);
		} catch {
			return String(value);
		}
	}
	return String(value);
}

function parseBoolean(value: any): boolean | null {
	if (typeof value === 'boolean') return value;
	if (typeof value === 'number') return value === 1 ? true : value === 0 ? false : null;
	if (typeof value !== 'string') return null;

	const normalized = value.trim().toLowerCase();
	if (['true', 't', '1', 'yes'].includes(normalized)) return true;
	if (['false', 'f', '0', 'no'].includes(normalized)) return false;
	return null;
}

function getJsonPreview(value: any): string {
	let parsed = value;
	if (typeof value === 'string') {
		try {
			parsed = JSON.parse(value);
		} catch {
			return value.replace(/\s+/g, ' ').trim();
		}
	}

	if (Array.isArray(parsed)) {
		return `[ ${parsed.length} ${parsed.length === 1 ? 'item' : 'items'} ]`;
	}
	if (parsed && typeof parsed === 'object') {
		const count = Object.keys(parsed).length;
		return `{ ${count} ${count === 1 ? 'field' : 'fields'} }`;
	}
	return String(parsed);
}

function formatDatePreview(value: any): string {
	const raw = stringifyValue(value);
	return raw
		.replace(/^(\d{4}-\d{2}-\d{2})T/, '$1 ')
		.replace(/Z$/, ' UTC')
		.replace(/([+-]\d{2}:\d{2})$/, ' $1');
}

export function getCellPresentation(value: any, dataType = ''): CellPresentation {
	if (value === null || value === undefined) {
		return { kind: 'null', text: 'NULL', title: 'NULL' };
	}

	const title = stringifyValue(value);
	const booleanValue = BOOLEAN_TYPE.test(dataType) ? parseBoolean(value) : null;
	if (booleanValue !== null) {
		return {
			kind: 'boolean',
			text: booleanValue ? 'TRUE' : 'FALSE',
			title,
			booleanValue
		};
	}

	if (JSON_TYPE.test(dataType) || typeof value === 'object') {
		return {
			kind: 'json',
			text: getJsonPreview(value),
			title
		};
	}

	if (DATETIME_TYPE.test(dataType)) {
		return {
			kind: 'datetime',
			text: formatDatePreview(value),
			title
		};
	}

	if (BINARY_TYPE.test(dataType)) {
		const size = typeof value === 'string' ? value.length : title.length;
		return {
			kind: 'binary',
			text: `<binary · ${size.toLocaleString()} chars>`,
			title
		};
	}

	if (NUMBER_TYPE.test(dataType) || typeof value === 'number') {
		return { kind: 'number', text: title, title };
	}

	return {
		kind: 'text',
		text: title.replace(/\s+/g, ' ').trim(),
		title
	};
}

export function getDefaultColumnWidth(name: string, dataType = ''): number {
	const labelWidth = Math.max(0, name.length - 8) * 7;

	if (BOOLEAN_TYPE.test(dataType)) return Math.max(116, 116 + labelWidth);
	if (NUMBER_TYPE.test(dataType)) return Math.max(128, 128 + labelWidth);
	if (DATETIME_TYPE.test(dataType)) return Math.max(190, 150 + labelWidth);
	if (JSON_TYPE.test(dataType)) return Math.max(170, 140 + labelWidth);
	if (BINARY_TYPE.test(dataType)) return Math.max(160, 140 + labelWidth);
	return Math.min(320, Math.max(168, 132 + labelWidth));
}

export function getColumnTypeLabel(column: ColumnTypeMetadata): string {
	if (column.is_enum) {
		const enumName = [column.type_schema, column.type_name].filter(Boolean).join('.');
		return enumName ? `enum · ${enumName}` : 'enum';
	}

	const dataType = column.data_type || 'unknown';
	return column.length && !dataType.includes('(') ? `${dataType}(${column.length})` : dataType;
}
