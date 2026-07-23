import assert from 'node:assert/strict';
import test from 'node:test';

import {
	getCellPresentation,
	getColumnTypeLabel,
	getDefaultColumnWidth
} from '../src/lib/table/cells.ts';

test('presents null and boolean values explicitly', () => {
	assert.deepEqual(getCellPresentation(null, 'text'), {
		kind: 'null',
		text: 'NULL',
		title: 'NULL'
	});
	assert.deepEqual(getCellPresentation('t', 'boolean'), {
		kind: 'boolean',
		text: 'TRUE',
		title: 't',
		booleanValue: true
	});
	assert.deepEqual(getCellPresentation(false, 'bool'), {
		kind: 'boolean',
		text: 'FALSE',
		title: 'false',
		booleanValue: false
	});
});

test('summarizes JSON without rendering object coercion text', () => {
	assert.deepEqual(getCellPresentation({ plan: 'team', active: true }, 'jsonb'), {
		kind: 'json',
		text: '{ 2 fields }',
		title: '{"plan":"team","active":true}'
	});
	assert.equal(getCellPresentation([1, 2, 3], 'json').text, '[ 3 items ]');
});

test('formats timestamp previews without changing their instant', () => {
	assert.deepEqual(getCellPresentation('2026-07-18T09:14:22.481Z', 'timestamptz'), {
		kind: 'datetime',
		text: '2026-07-18 09:14:22.481 UTC',
		title: '2026-07-18T09:14:22.481Z'
	});
});

test('collapses multiline text only in the cell preview', () => {
	assert.deepEqual(getCellPresentation('first line\nsecond line', 'text'), {
		kind: 'text',
		text: 'first line second line',
		title: 'first line\nsecond line'
	});
});

test('chooses compact but readable default column widths', () => {
	assert.equal(getDefaultColumnWidth('active', 'boolean'), 116);
	assert.equal(getDefaultColumnWidth('created_at', 'timestamptz'), 190);
	assert.ok(getDefaultColumnWidth('an_exceptionally_long_column_name', 'text') > 200);
});

test('labels PostgreSQL enums with their declared type', () => {
	assert.equal(
		getColumnTypeLabel({
			data_type: 'enum',
			is_enum: true,
			type_schema: 'public',
			type_name: 'order_status'
		}),
		'enum · public.order_status'
	);
	assert.equal(getColumnTypeLabel({ data_type: 'varchar', length: 255 }), 'varchar(255)');
});
