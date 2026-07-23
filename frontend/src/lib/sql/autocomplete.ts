import type * as Monaco from 'monaco-editor';
import {
	ensureColumnsForTables,
	getSqlAutocompleteMetadata,
	loadSchemaInfo,
	resolveSqlTable,
	type SqlAutocompleteMetadata,
	type SqlTableMetadata,
	type SqlTableReference
} from '$lib/stores/schema.svelte';
import {
	analyzeCompletionContext,
	getStatementAtCursor,
	isCursorInCommentOrString,
	normalizeIdentifier,
	type CompletionContext,
	type ParsedTableReference
} from '$lib/sql/context';
import { formatSqlKeyword, getSqlDialectDefinition, quoteSqlIdentifier } from '$lib/sql/dialects';
import type { database } from '$lib/wailsjs/go/models';

interface ResolvedTableReference {
	reference: ParsedTableReference;
	table: SqlTableMetadata;
}

let providerRegistration: Monaco.IDisposable | null = null;
let providerClients = 0;
let registeredMonaco: typeof Monaco | null = null;

function findSchema(metadata: SqlAutocompleteMetadata, name: string): string | undefined {
	const normalized = normalizeIdentifier(name);
	return metadata.schemas.find((schema) => normalizeIdentifier(schema) === normalized);
}

function resolveReferences(references: ParsedTableReference[]): ResolvedTableReference[] {
	return references
		.map((reference) => ({
			reference,
			table: resolveSqlTable(reference)
		}))
		.filter((item): item is ResolvedTableReference => Boolean(item.table));
}

function getQualifierTable(
	qualifier: string,
	references: ResolvedTableReference[],
	metadata: SqlAutocompleteMetadata
): ResolvedTableReference | null {
	const normalized = normalizeIdentifier(qualifier);
	const referenced = references.find(
		(item) =>
			normalizeIdentifier(item.reference.alias || '') === normalized ||
			normalizeIdentifier(item.reference.table) === normalized
	);
	if (referenced) return referenced;

	const matchingTables = metadata.tables.filter(
		(table) => normalizeIdentifier(table.name) === normalized
	);
	if (matchingTables.length !== 1) return null;

	return {
		reference: {
			schema: matchingTables[0].schema,
			table: matchingTables[0].name
		},
		table: matchingTables[0]
	};
}

function getColumnDetail(column: database.Structure): string {
	const parts = [column.data_type || 'column'];
	if (column.is_primary) parts.push('primary key');
	if (column.is_unique && !column.is_primary) parts.push('unique');
	if (column.nullable === false) parts.push('required');
	return parts.join(' · ');
}

function addSuggestion(
	suggestions: Monaco.languages.CompletionItem[],
	seen: Set<string>,
	suggestion: Monaco.languages.CompletionItem
): void {
	const label = typeof suggestion.label === 'string' ? suggestion.label : suggestion.label.label;
	const key = `${suggestion.kind}:${label}:${suggestion.insertText}`;
	if (seen.has(key)) return;
	seen.add(key);
	suggestions.push(suggestion);
}

function buildCompletionItems(
	monaco: typeof Monaco,
	context: CompletionContext,
	metadata: SqlAutocompleteMetadata,
	range: Monaco.IRange
): Monaco.languages.CompletionItem[] {
	const suggestions: Monaco.languages.CompletionItem[] = [];
	const seen = new Set<string>();
	const dialect = getSqlDialectDefinition(metadata.engine);
	const references = resolveReferences(context.tableReferences);
	const qualifierSchema = context.qualifier ? findSchema(metadata, context.qualifier) : undefined;
	const qualifierTable =
		context.qualifier && !qualifierSchema
			? getQualifierTable(context.qualifier, references, metadata)
			: null;

	const addTables = (schemaFilter?: string) => {
		const sameNameCounts = new Map<string, number>();
		for (const table of metadata.tables) {
			const key = normalizeIdentifier(table.name);
			sameNameCounts.set(key, (sameNameCounts.get(key) || 0) + 1);
		}

		for (const table of metadata.tables) {
			if (schemaFilter && normalizeIdentifier(table.schema) !== normalizeIdentifier(schemaFilter)) {
				continue;
			}

			const needsSchema =
				!schemaFilter && (sameNameCounts.get(normalizeIdentifier(table.name)) || 0) > 1;
			const tableName = quoteSqlIdentifier(table.name, metadata.engine);
			const insertText = needsSchema
				? `${quoteSqlIdentifier(table.schema, metadata.engine)}.${tableName}`
				: tableName;

			addSuggestion(suggestions, seen, {
				label: table.name,
				kind: monaco.languages.CompletionItemKind.Struct,
				insertText,
				detail: `Table · ${table.schema}`,
				documentation: `${table.columns.length} loaded columns`,
				sortText: `10-${table.name.toLowerCase()}`,
				range
			});
		}

		if (!schemaFilter) {
			for (const schema of metadata.schemas) {
				addSuggestion(suggestions, seen, {
					label: schema,
					kind: monaco.languages.CompletionItemKind.Module,
					insertText: quoteSqlIdentifier(schema, metadata.engine),
					detail: 'Schema / namespace',
					sortText: `11-${schema.toLowerCase()}`,
					commitCharacters: ['.'],
					range
				});
			}
			for (const cte of context.ctes) {
				addSuggestion(suggestions, seen, {
					label: cte,
					kind: monaco.languages.CompletionItemKind.Interface,
					insertText: cte,
					detail: 'Common table expression',
					sortText: `09-${cte.toLowerCase()}`,
					range
				});
			}
		}
	};

	const addColumns = () => {
		let targets: ResolvedTableReference[] = [];

		if (qualifierTable) {
			targets = [qualifierTable];
		} else if (!context.qualifier && references.length > 0) {
			targets = references;
		} else if (!context.qualifier) {
			targets = metadata.tables
				.filter((table) => table.columnsLoaded)
				.map((table) => ({
					reference: { schema: table.schema, table: table.name },
					table
				}));
		}

		if (targets.length > 0) {
			addSuggestion(suggestions, seen, {
				label: context.qualifier ? '*' : 'All columns (*)',
				kind: monaco.languages.CompletionItemKind.Field,
				insertText: '*',
				detail: context.qualifier ? `All columns from ${context.qualifier}` : 'All columns',
				sortText: '00-*',
				range
			});
		}

		const columnCounts = new Map<string, number>();
		for (const target of targets) {
			for (const column of target.table.columns) {
				const key = normalizeIdentifier(column.name);
				columnCounts.set(key, (columnCounts.get(key) || 0) + 1);
			}
		}

		for (const target of targets) {
			const qualifier = target.reference.alias || target.reference.table;
			for (const column of target.table.columns) {
				const duplicate = (columnCounts.get(normalizeIdentifier(column.name)) || 0) > 1;
				const qualifiedLabel = duplicate && !context.qualifier;
				const columnName = quoteSqlIdentifier(column.name, metadata.engine);
				const insertText = qualifiedLabel
					? `${quoteSqlIdentifier(qualifier, metadata.engine)}.${columnName}`
					: columnName;

				addSuggestion(suggestions, seen, {
					label: qualifiedLabel ? `${qualifier}.${column.name}` : column.name,
					kind: monaco.languages.CompletionItemKind.Field,
					insertText,
					detail: `${target.table.schema}.${target.table.name} · ${getColumnDetail(column)}`,
					sortText: `01-${column.name.toLowerCase()}-${qualifier.toLowerCase()}`,
					range
				});
			}
		}

		if (!context.qualifier) {
			for (const reference of references.filter((item) => item.reference.alias)) {
				const alias = reference.reference.alias || '';
				addSuggestion(suggestions, seen, {
					label: alias,
					kind: monaco.languages.CompletionItemKind.Variable,
					insertText: alias,
					detail: `Alias · ${reference.table.schema}.${reference.table.name}`,
					sortText: `02-${alias.toLowerCase()}`,
					commitCharacters: ['.'],
					range
				});
			}
		}
	};

	if (qualifierSchema) {
		addTables(qualifierSchema);
		return suggestions;
	}
	if (context.qualifier) {
		addColumns();
		return suggestions;
	}

	if (context.kind === 'table') {
		addTables(context.qualifier);
	} else if (context.kind === 'column') {
		addColumns();
	}

	if (context.kind !== 'table') {
		for (const definition of dialect.functions) {
			const name = formatSqlKeyword(definition.name, context.useLowercaseKeywords);
			const insertText = definition.insertText.replace(definition.name, name);
			addSuggestion(suggestions, seen, {
				label: definition.name,
				kind: monaco.languages.CompletionItemKind.Function,
				insertText,
				insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
				detail: definition.signature,
				documentation: definition.description,
				sortText: `20-${definition.name.toLowerCase()}`,
				range
			});
		}
	}

	for (const keyword of dialect.keywords) {
		addSuggestion(suggestions, seen, {
			label: keyword,
			kind: monaco.languages.CompletionItemKind.Keyword,
			insertText: formatSqlKeyword(keyword, context.useLowercaseKeywords),
			detail: `${dialect.label} keyword`,
			sortText: `30-${keyword.toLowerCase()}`,
			range
		});
	}

	if (context.kind === 'general') {
		for (const snippet of dialect.snippets) {
			addSuggestion(suggestions, seen, {
				label: snippet.label,
				kind: monaco.languages.CompletionItemKind.Snippet,
				insertText: snippet.insertText,
				insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
				filterText: `${snippet.prefix} ${snippet.label}`,
				detail: `${dialect.label} snippet`,
				documentation: snippet.description,
				sortText: `05-${snippet.prefix}`,
				range
			});
		}
	}

	if (context.kind === 'general') {
		addTables();
	}

	return suggestions;
}

async function provideCompletionItems(
	monaco: typeof Monaco,
	model: Monaco.editor.ITextModel,
	position: Monaco.Position,
	token: Monaco.CancellationToken
): Promise<Monaco.languages.CompletionList> {
	const fullSql = model.getValue();
	const cursorOffset = model.getOffsetAt(position);
	const statement = getStatementAtCursor(fullSql, cursorOffset);
	const beforeCursor = statement.text.slice(0, statement.cursorOffset);

	if (isCursorInCommentOrString(beforeCursor)) {
		return { suggestions: [] };
	}

	await loadSchemaInfo();
	if (token.isCancellationRequested) return { suggestions: [] };

	const context = analyzeCompletionContext(statement);
	let metadata = getSqlAutocompleteMetadata();
	const referencesToLoad: SqlTableReference[] = [...context.tableReferences];

	if (context.qualifier && !findSchema(metadata, context.qualifier)) {
		const qualifierReference = context.tableReferences.find(
			(reference) =>
				normalizeIdentifier(reference.alias || '') ===
					normalizeIdentifier(context.qualifier || '') ||
				normalizeIdentifier(reference.table) === normalizeIdentifier(context.qualifier || '')
		);
		if (qualifierReference) {
			referencesToLoad.push(qualifierReference);
		} else {
			referencesToLoad.push({ table: context.qualifier });
		}
	}

	if (referencesToLoad.length > 0) {
		await ensureColumnsForTables(referencesToLoad);
		if (token.isCancellationRequested) return { suggestions: [] };
		metadata = getSqlAutocompleteMetadata();
	}

	const word = model.getWordUntilPosition(position);
	const range: Monaco.IRange = {
		startLineNumber: position.lineNumber,
		endLineNumber: position.lineNumber,
		startColumn: word.startColumn,
		endColumn: word.endColumn
	};

	return {
		suggestions: buildCompletionItems(monaco, context, metadata, range)
	};
}

export function registerSqlCompletionProvider(monaco: typeof Monaco): Monaco.IDisposable {
	providerClients += 1;

	if (!providerRegistration || registeredMonaco !== monaco) {
		providerRegistration?.dispose();
		registeredMonaco = monaco;
		providerRegistration = monaco.languages.registerCompletionItemProvider('sql', {
			triggerCharacters: ['.', ' '],
			provideCompletionItems: (model, position, _context, token) =>
				provideCompletionItems(monaco, model, position, token)
		});
	}

	let disposed = false;
	return {
		dispose() {
			if (disposed) return;
			disposed = true;
			providerClients = Math.max(0, providerClients - 1);
			if (providerClients === 0) {
				providerRegistration?.dispose();
				providerRegistration = null;
				registeredMonaco = null;
			}
		}
	};
}
