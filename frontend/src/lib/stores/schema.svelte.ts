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

interface ConnectionSchemaState {
	schemas: string[];
	schemaInfo: Record<string, SchemaInfo>;
	databaseInfo: database.Info | null;
	isLoading: boolean;
	loadError: string;
}

interface ConnectionSchemaRuntime {
	generation: number;
	activeLoad: Promise<void> | null;
	hasLoaded: boolean;
	columnLoads: Map<string, Promise<void>>;
}

const emptyConnectionState = (): ConnectionSchemaState => ({
	schemas: [],
	schemaInfo: {},
	databaseInfo: null,
	isLoading: false,
	loadError: ''
});

let states = $state<Record<string, ConnectionSchemaState>>({});
const runtimes = new Map<string, ConnectionSchemaRuntime>();

function requireConnectionId(connectionId: string): string {
	if (!connectionId) throw new Error('A connection ID is required for schema metadata');
	return connectionId;
}

function getState(connectionId: string): ConnectionSchemaState {
	return states[connectionId] ?? emptyConnectionState();
}

function ensureState(connectionId: string): ConnectionSchemaState {
	const id = requireConnectionId(connectionId);
	if (!states[id]) states[id] = emptyConnectionState();
	return states[id];
}

function getRuntime(connectionId: string): ConnectionSchemaRuntime {
	const id = requireConnectionId(connectionId);
	let runtime = runtimes.get(id);
	if (!runtime) {
		runtime = {
			generation: 0,
			activeLoad: null,
			hasLoaded: false,
			columnLoads: new Map()
		};
		runtimes.set(id, runtime);
	}
	return runtime;
}

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
	connectionId: string,
	schema: string,
	table: string,
	columns: database.Structure[],
	generation: number
): void {
	const runtime = getRuntime(connectionId);
	if (generation !== runtime.generation) return;

	const currentState = ensureState(connectionId);
	const actualSchema = findNameCaseInsensitive(currentState.schemas, schema) ?? schema;
	const current = currentState.schemaInfo[actualSchema] ?? makeEmptySchemaInfo();
	const actualTable = findNameCaseInsensitive(current.tables, table) ?? table;

	states[connectionId] = {
		...currentState,
		schemaInfo: {
			...currentState.schemaInfo,
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
		}
	};
}

export function getSchemas(connectionId: string): string[] {
	return getState(connectionId).schemas;
}

export function getSchemaInfo(connectionId: string): Record<string, SchemaInfo> {
	return getState(connectionId).schemaInfo;
}

export function isSchemaLoading(connectionId: string): boolean {
	return getState(connectionId).isLoading;
}

export function getSchemaLoadError(connectionId: string): string {
	return getState(connectionId).loadError;
}

export function getDatabaseEngine(connectionId: string): string {
	return getState(connectionId).databaseInfo?.engine || 'SQL';
}

export function getSqlAutocompleteMetadata(connectionId: string): SqlAutocompleteMetadata {
	const state = getState(connectionId);
	const tables: SqlTableMetadata[] = [];

	for (const schema of state.schemas) {
		const info = state.schemaInfo[schema];
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
		engine: state.databaseInfo?.engine || 'SQL',
		dialect: normalizeSqlDialect(state.databaseInfo?.engine),
		database: state.databaseInfo?.database || '',
		schemas: [...state.schemas],
		tables,
		isLoading: state.isLoading,
		error: state.loadError
	};
}

export function getAllTables(connectionId: string): string[] {
	return getSqlAutocompleteMetadata(connectionId).tables.map(
		(table) => `${table.schema}.${table.name}`
	);
}

export function getColumnsForTable(
	connectionId: string,
	schema: string,
	table: string
): database.Structure[] {
	const state = getState(connectionId);
	const actualSchema = findNameCaseInsensitive(state.schemas, schema);
	if (!actualSchema) return [];

	const info = state.schemaInfo[actualSchema];
	const actualTable = findNameCaseInsensitive(info?.tables || [], table);
	return actualTable ? info.columns[actualTable] || [] : [];
}

export function getAllColumns(connectionId: string): string[] {
	const columns = new Set<string>();

	for (const table of getSqlAutocompleteMetadata(connectionId).tables) {
		for (const column of table.columns) {
			columns.add(column.name);
		}
	}

	return [...columns];
}

export function resolveSqlTable(
	connectionId: string,
	reference: SqlTableReference
): SqlTableMetadata | null {
	const metadata = getSqlAutocompleteMetadata(connectionId);
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
	connectionId: string,
	schema: string,
	table: string,
	force = false
): Promise<void> {
	const state = getState(connectionId);
	const runtime = getRuntime(connectionId);
	const actualSchema = findNameCaseInsensitive(state.schemas, schema) ?? schema;
	const info = state.schemaInfo[actualSchema];
	const actualTable = findNameCaseInsensitive(info?.tables || [], table) ?? table;

	if (!force && info?.loadedColumns[actualTable]) return;

	const key = getTableKey(actualSchema, actualTable);
	const existing = runtime.columnLoads.get(key);
	if (existing && !force) return existing;

	const generation = runtime.generation;
	const task = (async () => {
		try {
			const request = new database.Table({
				Schema: actualSchema,
				Name: actualTable
			});
			const response = await GetCollectionStructures(connectionId, request);
			if (response.errors?.length) {
				throw new Error(response.errors[0].detail);
			}
			setColumns(connectionId, actualSchema, actualTable, response.data || [], generation);
		} catch (error) {
			console.error(`Failed to load columns for ${actualSchema}.${actualTable}:`, error);
		} finally {
			if (runtime.columnLoads.get(key) === task) {
				runtime.columnLoads.delete(key);
			}
		}
	})();

	runtime.columnLoads.set(key, task);
	return task;
}

export async function ensureColumnsForTables(
	connectionId: string,
	references: SqlTableReference[]
): Promise<void> {
	const uniqueTables = new Map<string, SqlTableMetadata>();

	for (const reference of references) {
		const table = resolveSqlTable(connectionId, reference);
		if (table) {
			uniqueTables.set(getTableKey(table.schema, table.name), table);
		}
	}

	await Promise.all(
		[...uniqueTables.values()].map((table) =>
			loadColumnsForTable(connectionId, table.schema, table.name)
		)
	);
}

async function discoverSchemaInfo(connectionId: string, generation: number): Promise<void> {
	const runtime = getRuntime(connectionId);
	const [databaseResponse, schemasResponse] = await Promise.all([
		GetDatabaseInfo(connectionId),
		GetSchemas(connectionId)
	]);

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
		const response = await GetCollections(connectionId, []);
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
				const response = await GetCollections(connectionId, [schema]);
				nextSchemaInfo[schema] = {
					...makeEmptySchemaInfo(),
					tables: response.errors?.length ? [] : response.data || []
				};
			})
		);
	}

	if (generation !== runtime.generation) return;

	states[connectionId] = {
		databaseInfo: nextDatabaseInfo,
		schemas: nextSchemas,
		schemaInfo: nextSchemaInfo,
		isLoading: true,
		loadError: ''
	};
	runtime.hasLoaded = true;

	const preferred = getSqlAutocompleteMetadata(connectionId)
		.tables.sort((left, right) => {
			const preferredSchemas = ['public', 'main', nextDatabaseInfo?.database || ''];
			const leftIndex = preferredSchemas.indexOf(left.schema);
			const rightIndex = preferredSchemas.indexOf(right.schema);
			const leftRank = leftIndex === -1 ? preferredSchemas.length : leftIndex;
			const rightRank = rightIndex === -1 ? preferredSchemas.length : rightIndex;
			return leftRank - rightRank || left.name.localeCompare(right.name);
		})
		.slice(0, 8);

	void Promise.all(
		preferred.map((table) => loadColumnsForTable(connectionId, table.schema, table.name))
	);
}

export async function loadSchemaInfo(connectionId: string, force = false): Promise<void> {
	ensureState(connectionId);
	const runtime = getRuntime(connectionId);
	if (runtime.activeLoad) return runtime.activeLoad;
	if (runtime.hasLoaded && !force) return;

	const generation = ++runtime.generation;
	if (force) runtime.hasLoaded = false;
	runtime.columnLoads.clear();
	states[connectionId] = {
		...getState(connectionId),
		isLoading: true,
		loadError: ''
	};

	const task = (async () => {
		try {
			await discoverSchemaInfo(connectionId, generation);
		} catch (error) {
			if (generation === runtime.generation) {
				states[connectionId] = {
					...getState(connectionId),
					loadError: error instanceof Error ? error.message : 'Could not load SQL metadata'
				};
				console.error('Failed to load schema info:', error);
			}
		} finally {
			if (generation === runtime.generation) {
				states[connectionId] = {
					...getState(connectionId),
					isLoading: false
				};
			}
			if (runtime.activeLoad === task) {
				runtime.activeLoad = null;
			}
		}
	})();

	runtime.activeLoad = task;
	return task;
}

export function resetSchemaInfo(connectionId?: string): void {
	if (connectionId) {
		const runtime = runtimes.get(connectionId);
		if (runtime) {
			runtime.generation += 1;
			runtime.activeLoad = null;
			runtime.columnLoads.clear();
		}
		runtimes.delete(connectionId);
		delete states[connectionId];
		return;
	}

	for (const runtime of runtimes.values()) {
		runtime.generation += 1;
		runtime.activeLoad = null;
		runtime.columnLoads.clear();
	}
	runtimes.clear();
	states = {};
}

export function getSQLKeywords(connectionId: string): string[] {
	return getSqlDialectDefinition(getState(connectionId).databaseInfo?.engine).keywords;
}
