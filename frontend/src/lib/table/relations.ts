export interface ForeignKeyMetadata {
	foreign_key?: string | null;
	foreign_schema?: string | null;
	foreign_table?: string | null;
	foreign_column?: string | null;
}

export interface ForeignRelation {
	schema: string;
	table: string;
	column: string;
}

function unquoteIdentifier(identifier: string): string {
	const trimmed = identifier.trim();
	if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
		return trimmed.slice(1, -1).replaceAll('""', '"');
	}
	return trimmed;
}

function splitQualifiedIdentifier(identifier: string): string[] {
	const parts: string[] = [];
	let current = '';
	let quoted = false;

	for (let index = 0; index < identifier.length; index += 1) {
		const character = identifier[index];
		if (character === '"') {
			current += character;
			if (quoted && identifier[index + 1] === '"') {
				current += identifier[index + 1];
				index += 1;
			} else {
				quoted = !quoted;
			}
			continue;
		}

		if (character === '.' && !quoted) {
			parts.push(unquoteIdentifier(current));
			current = '';
			continue;
		}
		current += character;
	}

	if (current.trim()) parts.push(unquoteIdentifier(current));
	return parts;
}

export function getForeignRelation(
	metadata: ForeignKeyMetadata,
	fallbackSchema = ''
): ForeignRelation | null {
	const typedTable = metadata.foreign_table?.trim();
	if (typedTable) {
		return {
			schema: metadata.foreign_schema?.trim() || fallbackSchema,
			table: typedTable,
			column: metadata.foreign_column?.trim() || ''
		};
	}

	const legacyReference = metadata.foreign_key?.trim();
	if (!legacyReference) return null;

	const parenthesized = legacyReference.match(/^(.*?)\((.*?)\)$/);
	if (parenthesized) {
		const targetParts = splitQualifiedIdentifier(parenthesized[1]);
		const table = targetParts.at(-1) || '';
		if (!table) return null;

		return {
			schema: targetParts.length > 1 ? targetParts.at(-2) || fallbackSchema : fallbackSchema,
			table,
			column: unquoteIdentifier(parenthesized[2])
		};
	}

	const dottedParts = splitQualifiedIdentifier(legacyReference);
	if (dottedParts.length === 2) {
		return {
			schema: fallbackSchema,
			table: dottedParts[0],
			column: dottedParts[1]
		};
	}
	if (dottedParts.length >= 3) {
		return {
			schema: dottedParts.at(-3) || fallbackSchema,
			table: dottedParts.at(-2) || '',
			column: dottedParts.at(-1) || ''
		};
	}

	return null;
}
