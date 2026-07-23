export type SqlDialect = 'postgresql' | 'mysql' | 'sqlite' | 'generic';

export interface SqlFunctionDefinition {
	name: string;
	signature: string;
	description: string;
	insertText: string;
}

export interface SqlSnippetDefinition {
	label: string;
	prefix: string;
	description: string;
	insertText: string;
}

export interface SqlDialectDefinition {
	id: SqlDialect;
	label: string;
	identifierQuote: '"' | '`';
	keywords: string[];
	functions: SqlFunctionDefinition[];
	snippets: SqlSnippetDefinition[];
}

const CORE_KEYWORDS = [
	'SELECT',
	'FROM',
	'WHERE',
	'AND',
	'OR',
	'NOT',
	'IN',
	'LIKE',
	'BETWEEN',
	'IS',
	'NULL',
	'TRUE',
	'FALSE',
	'AS',
	'ON',
	'JOIN',
	'LEFT',
	'RIGHT',
	'INNER',
	'OUTER',
	'FULL',
	'CROSS',
	'ORDER',
	'BY',
	'ASC',
	'DESC',
	'GROUP',
	'HAVING',
	'LIMIT',
	'OFFSET',
	'UNION',
	'ALL',
	'DISTINCT',
	'INSERT',
	'INTO',
	'VALUES',
	'UPDATE',
	'SET',
	'DELETE',
	'CREATE',
	'TABLE',
	'INDEX',
	'VIEW',
	'DROP',
	'ALTER',
	'ADD',
	'COLUMN',
	'PRIMARY',
	'KEY',
	'FOREIGN',
	'REFERENCES',
	'CONSTRAINT',
	'DEFAULT',
	'UNIQUE',
	'CHECK',
	'CASCADE',
	'RESTRICT',
	'TRUNCATE',
	'BEGIN',
	'COMMIT',
	'ROLLBACK',
	'TRANSACTION',
	'CASE',
	'WHEN',
	'THEN',
	'ELSE',
	'END',
	'CAST',
	'EXISTS',
	'WITH',
	'RECURSIVE',
	'OVER',
	'PARTITION',
	'WINDOW',
	'ROWS',
	'RANGE',
	'CURRENT',
	'ROW',
	'PRECEDING',
	'FOLLOWING',
	'FETCH',
	'FIRST',
	'ONLY',
	'NULLS'
];

const CORE_FUNCTIONS: SqlFunctionDefinition[] = [
	{
		name: 'COUNT',
		signature: 'COUNT(expression)',
		description: 'Count rows or non-null values.',
		insertText: 'COUNT(${1:*})'
	},
	{
		name: 'SUM',
		signature: 'SUM(expression)',
		description: 'Sum numeric values.',
		insertText: 'SUM(${1:expression})'
	},
	{
		name: 'AVG',
		signature: 'AVG(expression)',
		description: 'Calculate the average of numeric values.',
		insertText: 'AVG(${1:expression})'
	},
	{
		name: 'MIN',
		signature: 'MIN(expression)',
		description: 'Return the minimum value.',
		insertText: 'MIN(${1:expression})'
	},
	{
		name: 'MAX',
		signature: 'MAX(expression)',
		description: 'Return the maximum value.',
		insertText: 'MAX(${1:expression})'
	},
	{
		name: 'COALESCE',
		signature: 'COALESCE(value, fallback)',
		description: 'Return the first non-null value.',
		insertText: 'COALESCE(${1:value}, ${2:fallback})'
	},
	{
		name: 'NULLIF',
		signature: 'NULLIF(value, comparison)',
		description: 'Return null when both expressions are equal.',
		insertText: 'NULLIF(${1:value}, ${2:comparison})'
	},
	{
		name: 'LOWER',
		signature: 'LOWER(text)',
		description: 'Convert text to lowercase.',
		insertText: 'LOWER(${1:text})'
	},
	{
		name: 'UPPER',
		signature: 'UPPER(text)',
		description: 'Convert text to uppercase.',
		insertText: 'UPPER(${1:text})'
	},
	{
		name: 'SUBSTRING',
		signature: 'SUBSTRING(text, start, length)',
		description: 'Extract part of a text value.',
		insertText: 'SUBSTRING(${1:text}, ${2:start}, ${3:length})'
	},
	{
		name: 'TRIM',
		signature: 'TRIM(text)',
		description: 'Remove leading and trailing whitespace.',
		insertText: 'TRIM(${1:text})'
	}
];

const CORE_SNIPPETS: SqlSnippetDefinition[] = [
	{
		label: 'SELECT … FROM …',
		prefix: 'select',
		description: 'Select rows from a table.',
		insertText: 'SELECT ${1:*}\nFROM ${2:table}\nWHERE ${3:condition};'
	},
	{
		label: 'SELECT with JOIN',
		prefix: 'join',
		description: 'Select rows using an inner join.',
		insertText:
			'SELECT ${1:a.*}\nFROM ${2:first_table} AS ${3:a}\nJOIN ${4:second_table} AS ${5:b} ON ${6:a.id = b.first_id}\nWHERE ${7:condition};'
	},
	{
		label: 'INSERT row',
		prefix: 'insert',
		description: 'Insert a row with explicit columns.',
		insertText: 'INSERT INTO ${1:table} (${2:columns})\nVALUES (${3:values});'
	},
	{
		label: 'UPDATE rows',
		prefix: 'update',
		description: 'Update matching rows.',
		insertText: 'UPDATE ${1:table}\nSET ${2:column = value}\nWHERE ${3:condition};'
	},
	{
		label: 'DELETE rows',
		prefix: 'delete',
		description: 'Delete matching rows.',
		insertText: 'DELETE FROM ${1:table}\nWHERE ${2:condition};'
	},
	{
		label: 'WITH common table expression',
		prefix: 'cte',
		description: 'Start a query with a common table expression.',
		insertText:
			'WITH ${1:result} AS (\n\tSELECT ${2:*}\n\tFROM ${3:table}\n)\nSELECT ${4:*}\nFROM ${1:result};'
	},
	{
		label: 'CASE expression',
		prefix: 'case',
		description: 'Create a conditional expression.',
		insertText: 'CASE\n\tWHEN ${1:condition} THEN ${2:value}\n\tELSE ${3:fallback}\nEND'
	}
];

const POSTGRES_FUNCTIONS: SqlFunctionDefinition[] = [
	{
		name: 'NOW',
		signature: 'NOW()',
		description: 'Return the current transaction timestamp.',
		insertText: 'NOW()'
	},
	{
		name: 'DATE_TRUNC',
		signature: 'DATE_TRUNC(unit, timestamp)',
		description: 'Truncate a timestamp to a time unit.',
		insertText: "DATE_TRUNC('${1:day}', ${2:timestamp})"
	},
	{
		name: 'STRING_AGG',
		signature: 'STRING_AGG(value, delimiter)',
		description: 'Concatenate values from multiple rows.',
		insertText: "STRING_AGG(${1:value}, '${2:, }')"
	},
	{
		name: 'JSONB_BUILD_OBJECT',
		signature: 'JSONB_BUILD_OBJECT(key, value, …)',
		description: 'Build a JSONB object from key/value pairs.',
		insertText: "JSONB_BUILD_OBJECT('${1:key}', ${2:value})"
	},
	{
		name: 'JSONB_AGG',
		signature: 'JSONB_AGG(expression)',
		description: 'Aggregate values into a JSONB array.',
		insertText: 'JSONB_AGG(${1:expression})'
	},
	{
		name: 'GENERATE_SERIES',
		signature: 'GENERATE_SERIES(start, stop, step)',
		description: 'Generate a set of values.',
		insertText: 'GENERATE_SERIES(${1:start}, ${2:stop}, ${3:step})'
	},
	{
		name: 'TO_CHAR',
		signature: 'TO_CHAR(value, format)',
		description: 'Format a timestamp or number as text.',
		insertText: "TO_CHAR(${1:value}, '${2:YYYY-MM-DD}')"
	},
	{
		name: 'UNNEST',
		signature: 'UNNEST(array)',
		description: 'Expand an array into rows.',
		insertText: 'UNNEST(${1:array})'
	}
];

const MYSQL_FUNCTIONS: SqlFunctionDefinition[] = [
	{
		name: 'NOW',
		signature: 'NOW()',
		description: 'Return the current date and time.',
		insertText: 'NOW()'
	},
	{
		name: 'IFNULL',
		signature: 'IFNULL(value, fallback)',
		description: 'Return a fallback when a value is null.',
		insertText: 'IFNULL(${1:value}, ${2:fallback})'
	},
	{
		name: 'GROUP_CONCAT',
		signature: 'GROUP_CONCAT(expression)',
		description: 'Concatenate values from a group.',
		insertText: 'GROUP_CONCAT(${1:expression})'
	},
	{
		name: 'DATE_FORMAT',
		signature: 'DATE_FORMAT(date, format)',
		description: 'Format a date value.',
		insertText: "DATE_FORMAT(${1:date}, '${2:%Y-%m-%d}')"
	},
	{
		name: 'STR_TO_DATE',
		signature: 'STR_TO_DATE(text, format)',
		description: 'Parse a text value as a date.',
		insertText: "STR_TO_DATE(${1:text}, '${2:%Y-%m-%d}')"
	},
	{
		name: 'JSON_EXTRACT',
		signature: 'JSON_EXTRACT(document, path)',
		description: 'Extract a value from a JSON document.',
		insertText: "JSON_EXTRACT(${1:document}, '${2:$.path}')"
	},
	{
		name: 'LAST_INSERT_ID',
		signature: 'LAST_INSERT_ID()',
		description: 'Return the latest auto-increment value.',
		insertText: 'LAST_INSERT_ID()'
	}
];

const SQLITE_FUNCTIONS: SqlFunctionDefinition[] = [
	{
		name: 'IFNULL',
		signature: 'IFNULL(value, fallback)',
		description: 'Return a fallback when a value is null.',
		insertText: 'IFNULL(${1:value}, ${2:fallback})'
	},
	{
		name: 'GROUP_CONCAT',
		signature: 'GROUP_CONCAT(expression, separator)',
		description: 'Concatenate non-null values from a group.',
		insertText: "GROUP_CONCAT(${1:expression}, '${2:,}')"
	},
	{
		name: 'STRFTIME',
		signature: 'STRFTIME(format, value)',
		description: 'Format a date or time value.',
		insertText: "STRFTIME('${1:%Y-%m-%d}', ${2:value})"
	},
	{
		name: 'DATETIME',
		signature: 'DATETIME(value, modifier)',
		description: 'Return a formatted date and time.',
		insertText: "DATETIME(${1:value}, '${2:modifier}')"
	},
	{
		name: 'JULIANDAY',
		signature: 'JULIANDAY(value)',
		description: 'Return the Julian day number.',
		insertText: 'JULIANDAY(${1:value})'
	},
	{
		name: 'JSON_EXTRACT',
		signature: 'JSON_EXTRACT(document, path)',
		description: 'Extract a value from a JSON document.',
		insertText: "JSON_EXTRACT(${1:document}, '${2:$.path}')"
	},
	{
		name: 'LAST_INSERT_ROWID',
		signature: 'LAST_INSERT_ROWID()',
		description: 'Return the rowid of the latest insert.',
		insertText: 'LAST_INSERT_ROWID()'
	}
];

const DIALECTS: Record<SqlDialect, SqlDialectDefinition> = {
	postgresql: {
		id: 'postgresql',
		label: 'PostgreSQL',
		identifierQuote: '"',
		keywords: [
			...CORE_KEYWORDS,
			'ILIKE',
			'RETURNING',
			'CONFLICT',
			'DO',
			'NOTHING',
			'MATERIALIZED',
			'LATERAL',
			'FILTER',
			'VACUUM',
			'ANALYZE',
			'EXPLAIN',
			'JSONB',
			'ARRAY',
			'ANY',
			'ENUM',
			'SERIAL',
			'BIGSERIAL'
		],
		functions: [...CORE_FUNCTIONS, ...POSTGRES_FUNCTIONS],
		snippets: [
			...CORE_SNIPPETS,
			{
				label: 'PostgreSQL upsert',
				prefix: 'upsert',
				description: 'Insert or update using ON CONFLICT.',
				insertText:
					'INSERT INTO ${1:table} (${2:columns})\nVALUES (${3:values})\nON CONFLICT (${4:key}) DO UPDATE\nSET ${5:column = EXCLUDED.column}\nRETURNING *;'
			},
			{
				label: 'EXPLAIN ANALYZE',
				prefix: 'explain',
				description: 'Inspect the PostgreSQL execution plan.',
				insertText: 'EXPLAIN (ANALYZE, BUFFERS)\n${1:SELECT * FROM table};'
			}
		]
	},
	mysql: {
		id: 'mysql',
		label: 'MySQL',
		identifierQuote: '`',
		keywords: [
			...CORE_KEYWORDS,
			'AUTO_INCREMENT',
			'UNSIGNED',
			'ENGINE',
			'REPLACE',
			'SHOW',
			'DESCRIBE',
			'EXPLAIN',
			'USE',
			'DATABASES',
			'DUPLICATE'
		],
		functions: [...CORE_FUNCTIONS, ...MYSQL_FUNCTIONS],
		snippets: [
			...CORE_SNIPPETS,
			{
				label: 'MySQL upsert',
				prefix: 'upsert',
				description: 'Insert or update using ON DUPLICATE KEY.',
				insertText:
					'INSERT INTO ${1:table} (${2:columns})\nVALUES (${3:values})\nON DUPLICATE KEY UPDATE ${4:column = VALUES(column)};'
			},
			{
				label: 'SHOW CREATE TABLE',
				prefix: 'showcreate',
				description: 'Show the DDL for a MySQL table.',
				insertText: 'SHOW CREATE TABLE ${1:table};'
			}
		]
	},
	sqlite: {
		id: 'sqlite',
		label: 'SQLite',
		identifierQuote: '"',
		keywords: [
			...CORE_KEYWORDS,
			'PRAGMA',
			'GLOB',
			'ROWID',
			'WITHOUT',
			'REPLACE',
			'ABORT',
			'FAIL',
			'IGNORE',
			'ATTACH',
			'DETACH',
			'VACUUM'
		],
		functions: [...CORE_FUNCTIONS, ...SQLITE_FUNCTIONS],
		snippets: [
			...CORE_SNIPPETS,
			{
				label: 'SQLite upsert',
				prefix: 'upsert',
				description: 'Insert or update using ON CONFLICT.',
				insertText:
					'INSERT INTO ${1:table} (${2:columns})\nVALUES (${3:values})\nON CONFLICT (${4:key}) DO UPDATE\nSET ${5:column = excluded.column};'
			},
			{
				label: 'PRAGMA table_info',
				prefix: 'pragma',
				description: 'Inspect SQLite table columns.',
				insertText: 'PRAGMA table_info(${1:table});'
			}
		]
	},
	generic: {
		id: 'generic',
		label: 'SQL',
		identifierQuote: '"',
		keywords: CORE_KEYWORDS,
		functions: CORE_FUNCTIONS,
		snippets: CORE_SNIPPETS
	}
};

export function normalizeSqlDialect(engine?: string): SqlDialect {
	const normalized = engine?.trim().toLowerCase() ?? '';

	if (
		normalized.includes('postgres') ||
		normalized.includes('cockroach') ||
		normalized.includes('yugabyte')
	) {
		return 'postgresql';
	}
	if (normalized.includes('mysql') || normalized.includes('mariadb')) {
		return 'mysql';
	}
	if (normalized.includes('sqlite')) {
		return 'sqlite';
	}
	return 'generic';
}

export function getSqlDialectDefinition(engine?: string): SqlDialectDefinition {
	return DIALECTS[normalizeSqlDialect(engine)];
}

export function formatSqlKeyword(keyword: string, useLowercase: boolean): string {
	return useLowercase ? keyword.toLowerCase() : keyword.toUpperCase();
}

export function quoteSqlIdentifier(identifier: string, engine?: string): string {
	if (/^[A-Za-z_][A-Za-z0-9_$]*$/.test(identifier)) {
		return identifier;
	}

	const quote = getSqlDialectDefinition(engine).identifierQuote;
	return `${quote}${identifier.replaceAll(quote, quote + quote)}${quote}`;
}
