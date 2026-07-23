import {
	GetCollections,
	GetCollectionStructures,
	GetDatabaseInfo,
	GetSchemas
} from '$lib/wailsjs/go/db/Service';
import { database } from '$lib/wailsjs/go/models';
import { getSqlDialectDefinition, normalizeSqlDialect, type SqlDialect } from '$lib/sql/dialects';

export interface SqlTableMetadata {
	schema: string;
	name: string;
	columns: database.Structure[];
	columnsLoaded: boolean;
}

export interface SqlAutocompleteMetadata {
	engine: string;
	dialect: SqlDialect;
	database: string;
	schemas: string[];
	tables: SqlTableMetadata[];
	isLoading: boolean;
	error: string;
}

export interface SqlTableReference {
	schema?: string;
	table: string;
}

interface SchemaInfo {
	tables: string[];
	columns: Record<string, database.Structure[]>;
	loadedColumns: Record<string, boolean>;
}

let schemas = $state<string[]>([]);
let schemaInfo = $state<Record<string, SchemaInfo>>({});
let databaseInfo = $state<database.Info | null>(null);
let isLoading = $state(false);
let loadError = $state('');

let loadGeneration = 0;
let activeLoad: Promise<void> | null = null;
let hasLoaded = false;
const columnLoads = new Map<string, Promise<void>>();

function normalizeName(value: string): string {
	return value.replace(/^["`[]|["`\]]$/g, '').toLowerCase();
}

function findNameCaseInsensitive(values: string[], target: string): string | undefined {
	const normalizedTarget = normalizeName(target);
	return values.find((value) => normalizeName(value) === normalizedTarget);
}

function getDefaultNamespace(info: database.Info | null): string {
	const dialect = normalizeSqlDialect(info?.engine);
	if (dialect === 'sqlite') return 'main';
	return info?.database || 'default';
}

function getTableKey(schema: string, table: string): string {
	return `${normalizeName(schema)}.${normalizeName(table)}`;
}

function makeEmptySchemaInfo(): SchemaInfo {
	return {
		tables: [],
		columns: {},
		loadedColumns: {}
	};
}

function setColumns(
	schema: string,
	table: string,
	columns: database.Structure[],
	generation: number
): void {
	if (generation !== loadGeneration) return;

	const actualSchema = findNameCaseInsensitive(schemas, schema) ?? schema;
	const current = schemaInfo[actualSchema] ?? makeEmptySchemaInfo();
	const actualTable = findNameCaseInsensitive(current.tables, table) ?? table;

	schemaInfo = {
		...schemaInfo,
		[actualSchema]: {
			...current,
			columns: {
				...current.columns,
				[actualTable]: columns
			},
			loadedColumns: {
				...current.loadedColumns,
				[actualTable]: true
			}
		}
	};
}

export function getSchemas(): string[] {
	return schemas;
}

export function getSchemaInfo(): Record<string, SchemaInfo> {
	return schemaInfo;
}

export function isSchemaLoading(): boolean {
	return isLoading;
}

export function getSchemaLoadError(): string {
	return loadError;
}

export function getDatabaseEngine(): string {
	return databaseInfo?.engine || 'SQL';
}

export function getSqlAutocompleteMetadata(): SqlAutocompleteMetadata {
	const tables: SqlTableMetadata[] = [];

	for (const schema of schemas) {
		const info = schemaInfo[schema];
		if (!info) continue;

		for (const table of info.tables) {
			tables.push({
				schema,
				name: table,
				columns: info.columns[table] || [],
				columnsLoaded: Boolean(info.loadedColumns[table])
			});
		}
	}

	return {
		engine: databaseInfo?.engine || 'SQL',
		dialect: normalizeSqlDialect(databaseInfo?.engine),
		database: databaseInfo?.database || '',
		schemas: [...schemas],
		tables,
		isLoading,
		error: loadError
	};
}

export function getAllTables(): string[] {
	return getSqlAutocompleteMetadata().tables.map((table) => `${table.schema}.${table.name}`);
}

export function getColumnsForTable(schema: string, table: string): database.Structure[] {
	const actualSchema = findNameCaseInsensitive(schemas, schema);
	if (!actualSchema) return [];

	const info = schemaInfo[actualSchema];
	const actualTable = findNameCaseInsensitive(info?.tables || [], table);
	return actualTable ? info.columns[actualTable] || [] : [];
}

export function getAllColumns(): string[] {
	const columns = new Set<string>();

	for (const table of getSqlAutocompleteMetadata().tables) {
		for (const column of table.columns) {
			columns.add(column.name);
		}
	}

	return [...columns];
}

export function resolveSqlTable(reference: SqlTableReference): SqlTableMetadata | null {
	const metadata = getSqlAutocompleteMetadata();
	const normalizedTable = normalizeName(reference.table);
	const normalizedSchema = reference.schema ? normalizeName(reference.schema) : '';

	const matches = metadata.tables.filter((table) => {
		if (normalizeName(table.name) !== normalizedTable) return false;
		return !normalizedSchema || normalizeName(table.schema) === normalizedSchema;
	});

	if (matches.length === 1) return matches[0];
	if (normalizedSchema) return matches[0] ?? null;

	const preferredSchemas = ['public', 'main', metadata.database].filter(Boolean).map(normalizeName);
	return (
		matches.find((table) => preferredSchemas.includes(normalizeName(table.schema))) ??
		matches[0] ??
		null
	);
}

export async function loadColumnsForTable(
	schema: string,
	table: string,
	force = false
): Promise<void> {
	const actualSchema = findNameCaseInsensitive(schemas, schema) ?? schema;
	const info = schemaInfo[actualSchema];
	const actualTable = findNameCaseInsensitive(info?.tables || [], table) ?? table;

	if (!force && info?.loadedColumns[actualTable]) return;

	const key = getTableKey(actualSchema, actualTable);
	const existing = columnLoads.get(key);
	if (existing && !force) return existing;

	const generation = loadGeneration;
	const task = (async () => {
		try {
			const request = new database.Table({
				Schema: actualSchema,
				Name: actualTable
			});
			const response = await GetCollectionStructures(request);
			if (response.errors?.length) {
				throw new Error(response.errors[0].detail);
			}
			setColumns(actualSchema, actualTable, response.data || [], generation);
		} catch (error) {
			console.error(`Failed to load columns for ${actualSchema}.${actualTable}:`, error);
		} finally {
			if (columnLoads.get(key) === task) {
				columnLoads.delete(key);
			}
		}
	})();

	columnLoads.set(key, task);
	return task;
}

export async function ensureColumnsForTables(references: SqlTableReference[]): Promise<void> {
	const uniqueTables = new Map<string, SqlTableMetadata>();

	for (const reference of references) {
		const table = resolveSqlTable(reference);
		if (table) {
			uniqueTables.set(getTableKey(table.schema, table.name), table);
		}
	}

	await Promise.all(
		[...uniqueTables.values()].map((table) => loadColumnsForTable(table.schema, table.name))
	);
}

async function discoverSchemaInfo(generation: number): Promise<void> {
	const [databaseResponse, schemasResponse] = await Promise.all([GetDatabaseInfo(), GetSchemas()]);

	if (databaseResponse.errors?.length) {
		throw new Error(databaseResponse.errors[0].detail);
	}
	if (schemasResponse.errors?.length) {
		throw new Error(schemasResponse.errors[0].detail);
	}

	const nextDatabaseInfo = databaseResponse.data || null;
	let nextSchemas = schemasResponse.data || [];
	const nextSchemaInfo: Record<string, SchemaInfo> = {};

	if (nextSchemas.length === 0) {
		nextSchemas = [getDefaultNamespace(nextDatabaseInfo)];
		const response = await GetCollections([]);
		if (response.errors?.length) {
			throw new Error(response.errors[0].detail);
		}
		nextSchemaInfo[nextSchemas[0]] = {
			...makeEmptySchemaInfo(),
			tables: response.data || []
		};
	} else {
		await Promise.all(
			nextSchemas.map(async (schema) => {
				const response = await GetCollections([schema]);
				nextSchemaInfo[schema] = {
					...makeEmptySchemaInfo(),
					tables: response.errors?.length ? [] : response.data || []
				};
			})
		);
	}

	if (generation !== loadGeneration) return;

	databaseInfo = nextDatabaseInfo;
	schemas = nextSchemas;
	schemaInfo = nextSchemaInfo;
	hasLoaded = true;

	const preferred = getSqlAutocompleteMetadata()
		.tables.sort((left, right) => {
			const preferredSchemas = ['public', 'main', nextDatabaseInfo?.database || ''];
			const leftIndex = preferredSchemas.indexOf(left.schema);
			const rightIndex = preferredSchemas.indexOf(right.schema);
			const leftRank = leftIndex === -1 ? preferredSchemas.length : leftIndex;
			const rightRank = rightIndex === -1 ? preferredSchemas.length : rightIndex;
			return leftRank - rightRank || left.name.localeCompare(right.name);
		})
		.slice(0, 8);

	void Promise.all(preferred.map((table) => loadColumnsForTable(table.schema, table.name)));
}

export async function loadSchemaInfo(force = false): Promise<void> {
	if (activeLoad) return activeLoad;
	if (hasLoaded && !force) return;

	const generation = ++loadGeneration;
	if (force) hasLoaded = false;
	columnLoads.clear();
	isLoading = true;
	loadError = '';

	const task = (async () => {
		try {
			await discoverSchemaInfo(generation);
		} catch (error) {
			if (generation === loadGeneration) {
				loadError = error instanceof Error ? error.message : 'Could not load SQL metadata';
				console.error('Failed to load schema info:', error);
			}
		} finally {
			if (generation === loadGeneration) {
				isLoading = false;
			}
			if (activeLoad === task) {
				activeLoad = null;
			}
		}
	})();

	activeLoad = task;
	return task;
}

export function resetSchemaInfo(): void {
	loadGeneration += 1;
	activeLoad = null;
	columnLoads.clear();
	schemas = [];
	schemaInfo = {};
	databaseInfo = null;
	isLoading = false;
	loadError = '';
	hasLoaded = false;
}

export function getSQLKeywords(): string[] {
	return getSqlDialectDefinition(databaseInfo?.engine).keywords;
}
