export type SqlDialect = 'postgresql' | 'mysql' | 'sqlite' | 'oracle' | 'sqlserver' | 'generic';

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
	identifierQuote: '"' | '`' | '[';
	identifierClose: '"' | '`' | ']';
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

const ORACLE_FUNCTIONS: SqlFunctionDefinition[] = [
	{
		name: 'SYSDATE',
		signature: 'SYSDATE',
		description: 'Return the current database server date and time.',
		insertText: 'SYSDATE'
	},
	{
		name: 'SYSTIMESTAMP',
		signature: 'SYSTIMESTAMP',
		description: 'Return the current timestamp with time zone.',
		insertText: 'SYSTIMESTAMP'
	},
	{
		name: 'NVL',
		signature: 'NVL(value, fallback)',
		description: 'Return a fallback when a value is null.',
		insertText: 'NVL(${1:value}, ${2:fallback})'
	},
	{
		name: 'DECODE',
		signature: 'DECODE(expression, search, result, default)',
		description: 'Map expression values to result values.',
		insertText: 'DECODE(${1:expression}, ${2:search}, ${3:result}, ${4:default})'
	},
	{
		name: 'LISTAGG',
		signature: 'LISTAGG(value, delimiter) WITHIN GROUP (ORDER BY value)',
		description: 'Concatenate ordered values from a group.',
		insertText: "LISTAGG(${1:value}, '${2:, }') WITHIN GROUP (ORDER BY ${1:value})"
	},
	{
		name: 'TO_DATE',
		signature: 'TO_DATE(text, format)',
		description: 'Parse text as an Oracle date.',
		insertText: "TO_DATE(${1:text}, '${2:YYYY-MM-DD}')"
	},
	{
		name: 'TO_TIMESTAMP',
		signature: 'TO_TIMESTAMP(text, format)',
		description: 'Parse text as an Oracle timestamp.',
		insertText: "TO_TIMESTAMP(${1:text}, '${2:YYYY-MM-DD HH24:MI:SS}')"
	},
	{
		name: 'TO_CHAR',
		signature: 'TO_CHAR(value, format)',
		description: 'Format a date, timestamp, or number as text.',
		insertText: "TO_CHAR(${1:value}, '${2:YYYY-MM-DD}')"
	},
	{
		name: 'JSON_VALUE',
		signature: 'JSON_VALUE(document, path)',
		description: 'Extract a scalar from a JSON document.',
		insertText: "JSON_VALUE(${1:document}, '${2:$.path}')"
	},
	{
		name: 'SYS_GUID',
		signature: 'SYS_GUID()',
		description: 'Generate a globally unique RAW identifier.',
		insertText: 'SYS_GUID()'
	}
];

const SQLSERVER_FUNCTIONS: SqlFunctionDefinition[] = [
	{
		name: 'GETDATE',
		signature: 'GETDATE()',
		description: 'Return the current database server date and time.',
		insertText: 'GETDATE()'
	},
	{
		name: 'SYSDATETIME',
		signature: 'SYSDATETIME()',
		description: 'Return the current high-precision server timestamp.',
		insertText: 'SYSDATETIME()'
	},
	{
		name: 'ISNULL',
		signature: 'ISNULL(value, fallback)',
		description: 'Return a fallback when a value is null.',
		insertText: 'ISNULL(${1:value}, ${2:fallback})'
	},
	{
		name: 'IIF',
		signature: 'IIF(condition, true_value, false_value)',
		description: 'Return one of two values based on a condition.',
		insertText: 'IIF(${1:condition}, ${2:true_value}, ${3:false_value})'
	},
	{
		name: 'STRING_AGG',
		signature: 'STRING_AGG(value, separator)',
		description: 'Concatenate values from a group.',
		insertText: "STRING_AGG(${1:value}, '${2:, }')"
	},
	{
		name: 'DATEADD',
		signature: 'DATEADD(datepart, number, date)',
		description: 'Add an interval to a date.',
		insertText: 'DATEADD(${1:day}, ${2:number}, ${3:date})'
	},
	{
		name: 'DATEDIFF',
		signature: 'DATEDIFF(datepart, start_date, end_date)',
		description: 'Return the difference between two dates.',
		insertText: 'DATEDIFF(${1:day}, ${2:start_date}, ${3:end_date})'
	},
	{
		name: 'JSON_VALUE',
		signature: 'JSON_VALUE(document, path)',
		description: 'Extract a scalar from JSON text.',
		insertText: "JSON_VALUE(${1:document}, '${2:$.path}')"
	},
	{
		name: 'NEWID',
		signature: 'NEWID()',
		description: 'Generate a uniqueidentifier value.',
		insertText: 'NEWID()'
	},
	{
		name: 'SCOPE_IDENTITY',
		signature: 'SCOPE_IDENTITY()',
		description: 'Return the latest identity value in the current scope.',
		insertText: 'SCOPE_IDENTITY()'
	}
];

const DIALECTS: Record<SqlDialect, SqlDialectDefinition> = {
	postgresql: {
		id: 'postgresql',
		label: 'PostgreSQL',
		identifierQuote: '"',
		identifierClose: '"',
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
		identifierClose: '`',
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
		identifierClose: '"',
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
	oracle: {
		id: 'oracle',
		label: 'Oracle',
		identifierQuote: '"',
		identifierClose: '"',
		keywords: [
			...CORE_KEYWORDS,
			'CONNECT',
			'START',
			'PRIOR',
			'LEVEL',
			'ROWNUM',
			'ROWID',
			'MINUS',
			'MERGE',
			'MATCHED',
			'PIVOT',
			'UNPIVOT',
			'MODEL',
			'SIBLINGS',
			'PACKAGE',
			'SEQUENCE',
			'SYNONYM',
			'PLSQL',
			'EXCEPTION',
			'BULK',
			'COLLECT',
			'RETURNING'
		],
		functions: [...CORE_FUNCTIONS, ...ORACLE_FUNCTIONS],
		snippets: [
			...CORE_SNIPPETS,
			{
				label: 'Oracle MERGE',
				prefix: 'merge',
				description: 'Insert or update rows with a MERGE statement.',
				insertText:
					'MERGE INTO ${1:target} target\nUSING ${2:source} source\nON (${3:target.id = source.id})\nWHEN MATCHED THEN\n\tUPDATE SET ${4:target.column = source.column}\nWHEN NOT MATCHED THEN\n\tINSERT (${5:columns}) VALUES (${6:values});'
			},
			{
				label: 'Oracle hierarchical query',
				prefix: 'connectby',
				description: 'Traverse hierarchical rows with CONNECT BY.',
				insertText:
					'SELECT ${1:*}\nFROM ${2:table}\nSTART WITH ${3:parent_id IS NULL}\nCONNECT BY PRIOR ${4:id = parent_id};'
			},
			{
				label: 'Oracle execution plan',
				prefix: 'explain',
				description: 'Populate the plan table for a statement.',
				insertText:
					'EXPLAIN PLAN FOR\n${1:SELECT * FROM table};\n\nSELECT * FROM TABLE(DBMS_XPLAN.DISPLAY);'
			}
		]
	},
	sqlserver: {
		id: 'sqlserver',
		label: 'SQL Server',
		identifierQuote: '[',
		identifierClose: ']',
		keywords: [
			...CORE_KEYWORDS,
			'TOP',
			'OUTPUT',
			'MERGE',
			'MATCHED',
			'APPLY',
			'CROSS',
			'OUTER',
			'PIVOT',
			'UNPIVOT',
			'IDENTITY',
			'GO',
			'DECLARE',
			'EXEC',
			'TRY',
			'CATCH',
			'THROW',
			'NOCOUNT',
			'CLUSTERED',
			'NONCLUSTERED',
			'INCLUDE'
		],
		functions: [...CORE_FUNCTIONS, ...SQLSERVER_FUNCTIONS],
		snippets: [
			...CORE_SNIPPETS,
			{
				label: 'SQL Server pagination',
				prefix: 'page',
				description: 'Page through an ordered SQL Server result.',
				insertText:
					'SELECT ${1:*}\nFROM ${2:table}\nORDER BY ${3:id}\nOFFSET ${4:0} ROWS FETCH NEXT ${5:100} ROWS ONLY;'
			},
			{
				label: 'SQL Server MERGE',
				prefix: 'merge',
				description: 'Synchronize source and target rows.',
				insertText:
					'MERGE ${1:target} AS target\nUSING ${2:source} AS source\nON ${3:target.id = source.id}\nWHEN MATCHED THEN\n\tUPDATE SET ${4:target.column = source.column}\nWHEN NOT MATCHED THEN\n\tINSERT (${5:columns}) VALUES (${6:values});'
			},
			{
				label: 'SQL Server TRY/CATCH',
				prefix: 'trycatch',
				description: 'Wrap T-SQL statements in structured error handling.',
				insertText: 'BEGIN TRY\n\t${1:statement};\nEND TRY\nBEGIN CATCH\n\tTHROW;\nEND CATCH;'
			}
		]
	},
	generic: {
		id: 'generic',
		label: 'SQL',
		identifierQuote: '"',
		identifierClose: '"',
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
	if (normalized.includes('oracle')) {
		return 'oracle';
	}
	if (
		normalized.includes('sqlserver') ||
		normalized.includes('sql server') ||
		normalized.includes('mssql')
	) {
		return 'sqlserver';
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

	const dialect = getSqlDialectDefinition(engine);
	const open = dialect.identifierQuote;
	const close = dialect.identifierClose;
	return `${open}${identifier.replaceAll(close, close + close)}${close}`;
}
