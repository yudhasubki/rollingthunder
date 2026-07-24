import assert from 'node:assert/strict';
import test from 'node:test';

import { buildDatabaseFilters, filterNeedsValue } from '../src/lib/table/filters.ts';

test('builds typed filters without assembling SQL in the frontend', () => {
	const filters = buildDatabaseFilters([
		{
			id: 'status',
			column: 'status',
			operator: 'eq',
			value: "open' OR true --",
			enabled: true
		},
		{
			id: 'deleted',
			column: 'deleted_at',
			operator: 'is_null',
			value: 'ignored',
			enabled: true
		}
	]);

	assert.deepEqual(filters, [
		{
			Column: 'status',
			Operator: 'eq',
			Value: "open' OR true --"
		},
		{
			Column: 'deleted_at',
			Operator: 'is_null',
			Value: null
		}
	]);
});

test('drops disabled and incomplete filters', () => {
	assert.deepEqual(
		buildDatabaseFilters([
			{
				id: 'disabled',
				column: 'id',
				operator: 'eq',
				value: '1',
				enabled: false
			},
			{
				id: 'missing-value',
				column: 'id',
				operator: 'eq',
				value: '',
				enabled: true
			}
		]),
		[]
	);
});

test('null operators do not require a value', () => {
	assert.equal(filterNeedsValue('eq'), true);
	assert.equal(filterNeedsValue('contains'), true);
	assert.equal(filterNeedsValue('is_null'), false);
	assert.equal(filterNeedsValue('is_not_null'), false);
});
