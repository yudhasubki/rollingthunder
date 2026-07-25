import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

import { extractQueryVariableNames, coerceQueryVariable } from '../src/lib/query/variables.ts';
import { parseSavedQueries, upsertSavedQuery } from '../src/lib/query/snippets.ts';
import { formatSql, lintSql } from '../src/lib/sql/tooling.ts';
import { getSqlIdentifierAtOffset, resolveSqlObjectTarget } from '../src/lib/sql/navigation.ts';
import {
	matchesShortcut,
	normalizeShortcut,
	parseShortcuts,
	shortcutFromKeyboardEvent
} from '../src/lib/commands/shortcuts.ts';

async function componentSource(path) {
	return readFile(new URL(`../${path}`, import.meta.url), 'utf8');
}

test('extracts unique query variables outside strings and comments', () => {
	const names = extractQueryVariableNames(`
		SELECT {{tenant_id}}, '{{ignored}}'
		FROM members
		WHERE id = {{member_id}} OR parent_id = {{member_id}}
		-- {{commented}}
	`);
	assert.deepEqual(names, ['tenant_id', 'member_id']);
	assert.equal(coerceQueryVariable({ name: 'limit', value: '25', type: 'number' }).value, 25);
});

test('upserts named queries with stable identity and tags', () => {
	const first = upsertSavedQuery(
		[],
		{ name: ' Active ', query: 'SELECT 1', engine: 'postgres', tags: ['daily', 'daily'] },
		new Date('2026-01-01T00:00:00Z')
	);
	const updated = upsertSavedQuery(
		first,
		{
			id: first[0].id,
			name: 'Active customers',
			query: 'SELECT * FROM customers',
			engine: 'postgres',
			tags: ['daily']
		},
		new Date('2026-01-02T00:00:00Z')
	);
	assert.equal(updated.length, 1);
	assert.equal(updated[0].createdAt, '2026-01-01T00:00:00.000Z');
	assert.equal(updated[0].updatedAt, '2026-01-02T00:00:00.000Z');
	assert.deepEqual(updated[0].tags, ['daily']);
	assert.deepEqual(parseSavedQueries('invalid').queries, []);
});

test('formats common SQL clauses without modifying string literals', () => {
	const formatted = formatSql(
		"select id, name from users where note = 'from here' and active = true;",
		'postgres',
		{ keywordCase: 'upper', indentSize: 2 }
	);
	assert.match(formatted, /^SELECT/);
	assert.match(formatted, /\nFROM users/);
	assert.match(formatted, /\nWHERE note = 'from here'/);
	assert.match(formatted, /\nAND active = TRUE;/);
});

test('reports configurable SQL safety and style issues', () => {
	const issues = lintSql('UPDATE users SET active = false\nSELECT * FROM users', {
		requireWhereForMutations: true,
		disallowSelectStar: true,
		requireSemicolon: true,
		maxLineLength: 200
	});
	assert.deepEqual(
		issues.map((issue) => issue.rule),
		['mutation-where', 'select-star', 'terminal-semicolon']
	);
});

test('resolves alias columns and table names to schema objects', () => {
	const sql = 'SELECT u.email FROM public.users AS u WHERE u.id = 1';
	const identifier = getSqlIdentifierAtOffset(sql, sql.indexOf('u.email') + 3);
	assert.ok(identifier);
	const target = resolveSqlObjectTarget(sql, identifier, {
		engine: 'PostgreSQL',
		dialect: 'postgres',
		database: 'app',
		schemas: ['public'],
		tables: [
			{
				schema: 'public',
				name: 'users',
				columnsLoaded: true,
				columns: [{ name: 'id' }, { name: 'email' }]
			}
		],
		isLoading: false,
		error: ''
	});
	assert.deepEqual(target, { schema: 'public', table: 'users', column: 'email' });
});

test('normalizes and matches cross-platform keyboard shortcuts', () => {
	const event = {
		key: 'k',
		ctrlKey: false,
		metaKey: true,
		altKey: false,
		shiftKey: false
	};
	assert.equal(shortcutFromKeyboardEvent(event), 'Mod+K');
	assert.equal(matchesShortcut(event, 'Mod+K'), true);
	assert.equal(normalizeShortcut('shift + mod + e'), 'Mod+Shift+E');
	assert.equal(parseShortcuts('broken').commandPalette, 'Mod+K');
});

test('surfaces SQL tooling behavior and object targets in the workspace', async () => {
	const [queryEditor, tableContent, toolingSettings] = await Promise.all([
		componentSource('src/lib/components/QueryEditorContent.svelte'),
		componentSource('src/lib/components/TableContent.svelte'),
		componentSource('src/lib/components/query/QueryToolingSettings.svelte')
	]);

	assert.doesNotMatch(queryEditor, /findIdentifierReferences/);
	assert.match(queryEditor, />\s*Open object\s*</);
	assert.match(queryEditor, /navigationNotice/);
	assert.match(queryEditor, /focusColumn:\s*target\.column/);
	assert.match(queryEditor, /container-type:\s*inline-size/);
	assert.match(queryEditor, /@container\s+\(max-width:\s*1040px\)/);
	assert.match(queryEditor, /\.query-editor-actions[\s\S]*flex-wrap:\s*wrap/);

	assert.match(tableContent, /data-column-name=/);
	assert.match(tableContent, /scrollIntoView/);
	assert.match(tableContent, />Target</);

	assert.match(toolingSettings, /Runs only when you choose Format/);
	assert.match(toolingSettings, /Runs automatically while you type/);
	assert.match(toolingSettings, /issues\.slice\(0,\s*5\)/);
});
