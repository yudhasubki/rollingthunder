import {
	normalizeIdentifier,
	parseTableReferences,
	removeSqlNoise,
	type ParsedTableReference
} from './context.ts';
import type { SqlAutocompleteMetadata, SqlTableMetadata } from '../stores/schema.svelte.ts';

export interface SqlIdentifier {
	text: string;
	start: number;
	end: number;
	parts: string[];
}

export interface SqlObjectTarget {
	schema: string;
	table: string;
	column?: string;
}

export function getSqlIdentifierAtOffset(sql: string, offset: number): SqlIdentifier | null {
	const clean = removeSqlNoise(sql);
	const clamped = Math.max(0, Math.min(offset, clean.length));
	const allowed = /[A-Za-z0-9_$."`\[\]]/;
	let start = clamped;
	let end = clamped;
	while (start > 0 && allowed.test(clean[start - 1])) start -= 1;
	while (end < clean.length && allowed.test(clean[end])) end += 1;
	const text = sql.slice(start, end).trim();
	if (!text) return null;
	const parts = text
		.split('.')
		.map((part) => part.trim().replace(/^["`[]|["`\]]$/g, ''))
		.filter(Boolean);
	return parts.length > 0 ? { text, start, end, parts } : null;
}

function findTable(
	metadata: SqlAutocompleteMetadata,
	schema: string | undefined,
	name: string
): SqlTableMetadata | undefined {
	const candidates = metadata.tables.filter(
		(table) => normalizeIdentifier(table.name) === normalizeIdentifier(name)
	);
	if (schema) {
		return candidates.find(
			(table) => normalizeIdentifier(table.schema) === normalizeIdentifier(schema)
		);
	}
	return (
		candidates.find((table) => ['public', 'main'].includes(normalizeIdentifier(table.schema))) ||
		candidates[0]
	);
}

function tableForAlias(
	references: ParsedTableReference[],
	qualifier: string
): ParsedTableReference | undefined {
	return references.find(
		(reference) =>
			normalizeIdentifier(reference.alias || '') === normalizeIdentifier(qualifier) ||
			normalizeIdentifier(reference.table) === normalizeIdentifier(qualifier)
	);
}

export function resolveSqlObjectTarget(
	sql: string,
	identifier: SqlIdentifier,
	metadata: SqlAutocompleteMetadata
): SqlObjectTarget | null {
	const references = parseTableReferences(sql);
	if (identifier.parts.length >= 2) {
		const qualifier = identifier.parts.at(-2) || '';
		const name = identifier.parts.at(-1) || '';
		const reference = tableForAlias(references, qualifier);
		if (reference) {
			const table = findTable(metadata, reference.schema, reference.table);
			if (table) return { schema: table.schema, table: table.name, column: name };
		}
		const table = findTable(metadata, qualifier, name);
		if (table) return { schema: table.schema, table: table.name };
	}

	const name = identifier.parts.at(-1) || '';
	const directTable = findTable(metadata, undefined, name);
	if (directTable) return { schema: directTable.schema, table: directTable.name };

	const columnMatches = references
		.map((reference) => findTable(metadata, reference.schema, reference.table))
		.filter((table): table is SqlTableMetadata => Boolean(table))
		.filter((table) =>
			table.columns.some((column) => normalizeIdentifier(column.name) === normalizeIdentifier(name))
		);
	if (columnMatches.length === 1) {
		return {
			schema: columnMatches[0].schema,
			table: columnMatches[0].name,
			column: name
		};
	}
	return null;
}
