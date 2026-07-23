import assert from 'node:assert/strict';
import test from 'node:test';

import {
	analyzeCompletionContext,
	getStatementAtCursor,
	isCursorInCommentOrString,
	parseCteNames,
	parseTableReferences,
	removeSqlNoise
} from '../src/lib/sql/context.ts';
import {
	getSqlDialectDefinition,
	normalizeSqlDialect,
	quoteSqlIdentifier
} from '../src/lib/sql/dialects.ts';

test('selects only the statement at the cursor', () => {
	const sql = "SELECT '⚡; thunder' AS literal;\nSELECT * FROM public.users AS u WHERE u.";
	const statement = getStatementAtCursor(sql, sql.length);

	assert.equal(statement.text.trim(), 'SELECT * FROM public.users AS u WHERE u.');
	assert.equal(statement.cursorOffset, statement.text.length);
});

test('does not split PostgreSQL dollar-quoted bodies on internal semicolons', () => {
	const sql = `DO $body$
BEGIN
	RAISE NOTICE 'still inside; the body';
END;
$body$;
SELECT * FROM audit_log AS event WHERE event.`;
	const statement = getStatementAtCursor(sql, sql.length);

	assert.equal(statement.text.trim(), 'SELECT * FROM audit_log AS event WHERE event.');
});

test('does not split quoted identifiers or comments on semicolons', () => {
	const sql =
		'SELECT * FROM "odd;table"; -- ignored ; delimiter\nSELECT * FROM `next;table`; SELECT * FROM [last;table]';
	const statement = getStatementAtCursor(sql, sql.length);

	assert.equal(statement.text.trim(), 'SELECT * FROM [last;table]');
});

test('detects SQL strings and PostgreSQL, MySQL, and block comments at the cursor', () => {
	assert.equal(isCursorInCommentOrString("SELECT 'unfinished"), true);
	assert.equal(isCursorInCommentOrString('SELECT $$unfinished'), true);
	assert.equal(isCursorInCommentOrString('SELECT 1 -- unfinished'), true);
	assert.equal(isCursorInCommentOrString('SELECT 1 # unfinished'), true);
	assert.equal(isCursorInCommentOrString('SELECT 1 /* unfinished'), true);
	assert.equal(isCursorInCommentOrString("SELECT 'finished'"), false);
	assert.equal(isCursorInCommentOrString('SELECT "quoted_identifier"'), false);
});

test('removes strings and comments before extracting table references', () => {
	const sql = `
		SELECT '-- FROM ghost_table' AS note
		FROM "sales"."Order" AS o
		JOIN customers c ON c.id = o.customer_id
		/* JOIN another_ghost g ON true */
	`;

	assert.equal(removeSqlNoise(sql).length, sql.length);
	assert.deepEqual(parseTableReferences(sql), [
		{ schema: 'sales', table: 'Order', alias: 'o' },
		{ schema: undefined, table: 'customers', alias: 'c' }
	]);
});

test('does not mistake a following clause for a table alias', () => {
	assert.deepEqual(parseTableReferences('SELECT * FROM users WHERE active = true'), [
		{ schema: undefined, table: 'users', alias: undefined }
	]);
});

test('extracts recursive and chained CTE names', () => {
	const sql = `
		WITH RECURSIVE tree AS (SELECT 1),
		totals(id) AS (SELECT id FROM tree)
		SELECT * FROM totals
	`;

	assert.deepEqual(parseCteNames(sql), ['tree', 'totals']);
});

test('recognizes table, alias-column, and insert-column completion contexts', () => {
	const tableSql = 'SELECT * FROM pub';
	const tableContext = analyzeCompletionContext({
		text: tableSql,
		cursorOffset: tableSql.length
	});
	assert.equal(tableContext.kind, 'table');

	const aliasSql = 'SELECT * FROM public.users AS u WHERE u.';
	const aliasContext = analyzeCompletionContext({
		text: aliasSql,
		cursorOffset: aliasSql.length
	});
	assert.equal(aliasContext.kind, 'column');
	assert.equal(aliasContext.qualifier, 'u');
	assert.deepEqual(aliasContext.tableReferences, [
		{ schema: 'public', table: 'users', alias: 'u' }
	]);

	const insertSql = 'INSERT INTO users (em';
	const insertContext = analyzeCompletionContext({
		text: insertSql,
		cursorOffset: insertSql.length
	});
	assert.equal(insertContext.kind, 'column');
});

test('preserves the SQL keyword casing style', () => {
	const lowercaseSql = 'select * from users where';
	const lowercaseContext = analyzeCompletionContext({
		text: lowercaseSql,
		cursorOffset: lowercaseSql.length
	});
	assert.equal(lowercaseContext.useLowercaseKeywords, true);

	const uppercaseSql = 'SELECT * FROM users WHERE';
	const uppercaseContext = analyzeCompletionContext({
		text: uppercaseSql,
		cursorOffset: uppercaseSql.length
	});
	assert.equal(uppercaseContext.useLowercaseKeywords, false);
});

test('normalizes supported database engine names', () => {
	assert.equal(normalizeSqlDialect('PostgreSQL 17'), 'postgresql');
	assert.equal(normalizeSqlDialect('CockroachDB'), 'postgresql');
	assert.equal(normalizeSqlDialect('MariaDB'), 'mysql');
	assert.equal(normalizeSqlDialect('sqlite3'), 'sqlite');
	assert.equal(normalizeSqlDialect('unknown'), 'generic');
});

test('keeps engine-specific function catalogs isolated', () => {
	const postgresFunctions = getSqlDialectDefinition('postgres').functions.map(({ name }) => name);
	const mysqlFunctions = getSqlDialectDefinition('mysql').functions.map(({ name }) => name);
	const sqliteFunctions = getSqlDialectDefinition('sqlite').functions.map(({ name }) => name);

	assert.ok(postgresFunctions.includes('DATE_TRUNC'));
	assert.ok(!postgresFunctions.includes('DATE_FORMAT'));
	assert.ok(mysqlFunctions.includes('DATE_FORMAT'));
	assert.ok(!mysqlFunctions.includes('DATE_TRUNC'));
	assert.ok(sqliteFunctions.includes('STRFTIME'));
	assert.ok(!sqliteFunctions.includes('DATE_FORMAT'));
});

test('quotes unsafe identifiers with the active engine rules', () => {
	assert.equal(quoteSqlIdentifier('safe_name', 'postgres'), 'safe_name');
	assert.equal(quoteSqlIdentifier('order item', 'postgres'), '"order item"');
	assert.equal(quoteSqlIdentifier('order`item', 'mysql'), '`order``item`');
	assert.equal(quoteSqlIdentifier('order"item', 'sqlite'), '"order""item"');
});
