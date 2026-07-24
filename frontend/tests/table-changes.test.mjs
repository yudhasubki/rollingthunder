import assert from 'node:assert/strict';
import test from 'node:test';

import {
	findChangedColumns,
	getChangedColumns,
	getOriginalRow,
	getRowIdentity,
	STAGED_CHANGED_COLUMNS,
	STAGED_ORIGINAL,
	stripInternalRowFields
} from '../src/lib/table/changes.ts';

test('tracks only columns whose values actually changed', () => {
	const original = {
		tenant_id: 4,
		user_id: 9,
		role: 'viewer',
		settings: { alerts: true }
	};
	const current = {
		...original,
		role: 'admin',
		settings: { alerts: true }
	};

	assert.deepEqual(findChangedColumns(original, current), ['role']);
});

test('keeps original composite identity after primary-key edits', () => {
	const staged = {
		tenant_id: 5,
		user_id: 9,
		role: 'admin',
		[STAGED_ORIGINAL]: {
			tenant_id: 4,
			user_id: 9,
			role: 'viewer'
		},
		[STAGED_CHANGED_COLUMNS]: ['tenant_id', 'role']
	};

	assert.equal(getRowIdentity(staged, ['tenant_id', 'user_id']), 'primary:[4,9]');
	assert.equal(getOriginalRow(staged).tenant_id, 4);
	assert.deepEqual(getChangedColumns(staged), ['tenant_id', 'role']);
});

test('removes staging metadata before sending rows to the backend', () => {
	const cleaned = stripInternalRowFields({
		id: 7,
		label: 'ready',
		_isNew: true,
		_rtStageId: 'local',
		_rtOriginal: { id: 7, label: 'before' },
		temp_preview: true
	});

	assert.deepEqual(cleaned, { id: 7, label: 'ready' });
});
