// Schema store for SQL autocomplete
// Stores table and column information for the connected database

import { GetCollections, GetCollectionStructures, GetSchemas } from '$lib/wailsjs/go/db/Service';
import { database } from '$lib/wailsjs/go/models';

interface SchemaInfo {
    tables: string[];
    columns: Record<string, database.Structure[]>; // table -> columns
}

// Store state
let schemas = $state<string[]>([]);
let schemaInfo = $state<Record<string, SchemaInfo>>({}); // schema -> SchemaInfo
let isLoading = $state(false);

// Getters
export function getSchemas() {
    return schemas;
}

export function getSchemaInfo() {
    return schemaInfo;
}

export function isSchemaLoading() {
    return isLoading;
}

// Get all table names for autocomplete (formatted as schema.table)
export function getAllTables(): string[] {
    const tables: string[] = [];
    for (const [schema, info] of Object.entries(schemaInfo)) {
        for (const table of info.tables) {
            tables.push(`${schema}.${table}`);
        }
    }
    return tables;
}

// Get columns for a specific table
export function getColumnsForTable(schema: string, table: string): database.Structure[] {
    return schemaInfo[schema]?.columns[table] || [];
}

// Get all column names (for simple autocomplete)
export function getAllColumns(): string[] {
    const columns: string[] = [];
    for (const [, info] of Object.entries(schemaInfo)) {
        for (const [, cols] of Object.entries(info.columns)) {
            for (const col of cols) {
                if (!columns.includes(col.name)) {
                    columns.push(col.name);
                }
            }
        }
    }
    return columns;
}

// Load all schema info for autocomplete
export async function loadSchemaInfo(): Promise<void> {
    if (isLoading) return;
    isLoading = true;

    try {
        // Get all schemas
        const schemaRes = await GetSchemas();
        if (schemaRes.errors?.length) {
            throw new Error(schemaRes.errors[0].detail);
        }
        schemas = schemaRes.data || [];

        // Load tables for each schema
        for (const schema of schemas) {
            if (!schemaInfo[schema]) {
                schemaInfo[schema] = { tables: [], columns: {} };
            }

            const tablesRes = await GetCollections([schema]);
            if (tablesRes.errors?.length) continue;

            schemaInfo[schema].tables = tablesRes.data || [];

            // Load columns for each table (lazy load - only first 10 tables initially)
            const tablesToLoad = schemaInfo[schema].tables.slice(0, 10);
            for (const table of tablesToLoad) {
                await loadColumnsForTable(schema, table);
            }
        }
    } catch (e) {
        console.error('Failed to load schema info:', e);
    } finally {
        isLoading = false;
    }
}

// Load columns for a specific table
export async function loadColumnsForTable(schema: string, table: string): Promise<void> {
    if (schemaInfo[schema]?.columns[table]) return; // Already loaded

    try {
        const reqTable = new database.Table();
        reqTable.Name = table;
        reqTable.Schema = schema;

        const colsRes = await GetCollectionStructures(reqTable);
        if (colsRes.errors?.length) return;

        if (!schemaInfo[schema]) {
            schemaInfo[schema] = { tables: [], columns: {} };
        }
        schemaInfo[schema].columns[table] = colsRes.data || [];
    } catch (e) {
        console.error(`Failed to load columns for ${schema}.${table}:`, e);
    }
}

// Get SQL keywords for autocomplete
export function getSQLKeywords(): string[] {
    return [
        'SELECT', 'FROM', 'WHERE', 'AND', 'OR', 'NOT', 'IN', 'LIKE', 'BETWEEN',
        'IS', 'NULL', 'TRUE', 'FALSE', 'AS', 'ON', 'JOIN', 'LEFT', 'RIGHT',
        'INNER', 'OUTER', 'FULL', 'CROSS', 'ORDER', 'BY', 'ASC', 'DESC',
        'GROUP', 'HAVING', 'LIMIT', 'OFFSET', 'UNION', 'ALL', 'DISTINCT',
        'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE', 'CREATE',
        'TABLE', 'INDEX', 'VIEW', 'DROP', 'ALTER', 'ADD', 'COLUMN',
        'PRIMARY', 'KEY', 'FOREIGN', 'REFERENCES', 'CONSTRAINT', 'DEFAULT',
        'UNIQUE', 'CHECK', 'CASCADE', 'RESTRICT', 'TRUNCATE', 'BEGIN',
        'COMMIT', 'ROLLBACK', 'TRANSACTION', 'CASE', 'WHEN', 'THEN', 'ELSE',
        'END', 'CAST', 'COALESCE', 'NULLIF', 'EXISTS', 'COUNT', 'SUM',
        'AVG', 'MIN', 'MAX', 'LOWER', 'UPPER', 'LENGTH', 'SUBSTRING',
        'TRIM', 'CONCAT', 'NOW', 'CURRENT_DATE', 'CURRENT_TIME', 'CURRENT_TIMESTAMP'
    ];
}
