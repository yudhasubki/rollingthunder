import assert from 'node:assert/strict';
import test from 'node:test';

import { getNextSortingState } from '../src/lib/table/sorting.ts';

test('cycles a single-column sort through ascending, descending, and clear', () => {
	const ascending = getNextSortingState([], 'created_at', false);
	assert.deepEqual(ascending, [{ id: 'created_at', desc: false }]);

	const descending = getNextSortingState(ascending, 'created_at', false);
	assert.deepEqual(descending, [{ id: 'created_at', desc: true }]);

	const cleared = getNextSortingState(descending, 'created_at', false);
	assert.deepEqual(cleared, []);
});

test('a regular click replaces an existing sort with the selected column', () => {
	const current = [
		{ id: 'tenant_id', desc: false },
		{ id: 'created_at', desc: true }
	];

	assert.deepEqual(getNextSortingState(current, 'name', false), [{ id: 'name', desc: false }]);
	assert.deepEqual(getNextSortingState(current, 'tenant_id', false), [
		{ id: 'tenant_id', desc: true }
	]);
});

test('shift-click appends and cycles a secondary sort without changing priority', () => {
	const primary = [{ id: 'tenant_id', desc: false }];
	const appended = getNextSortingState(primary, 'created_at', true);
	assert.deepEqual(appended, [
		{ id: 'tenant_id', desc: false },
		{ id: 'created_at', desc: false }
	]);

	const descending = getNextSortingState(appended, 'created_at', true);
	assert.deepEqual(descending, [
		{ id: 'tenant_id', desc: false },
		{ id: 'created_at', desc: true }
	]);

	const removed = getNextSortingState(descending, 'created_at', true);
	assert.deepEqual(removed, primary);
});
