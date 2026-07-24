import assert from 'node:assert/strict';
import test from 'node:test';

import {
	UPDATE_REMINDER_DELAY_MS,
	createUpdateSnooze,
	displayVersion,
	isUpdateSnoozed
} from '../src/lib/update/notification.ts';

test('snoozes only the matching release until its reminder expires', () => {
	const now = Date.UTC(2026, 6, 25);
	const serialized = createUpdateSnooze('1.2.0', UPDATE_REMINDER_DELAY_MS, now);

	assert.equal(isUpdateSnoozed(serialized, '1.2.0', now + 1), true);
	assert.equal(isUpdateSnoozed(serialized, '1.3.0', now + 1), false);
	assert.equal(isUpdateSnoozed(serialized, '1.2.0', now + UPDATE_REMINDER_DELAY_MS), false);
});

test('ignores malformed snooze state and formats versions consistently', () => {
	assert.equal(isUpdateSnoozed('{', '1.2.0'), false);
	assert.equal(isUpdateSnoozed('{"version":"1.2.0"}', '1.2.0'), false);
	assert.equal(displayVersion('v1.2.0'), 'v1.2.0');
	assert.equal(displayVersion(' 1.3.0 '), 'v1.3.0');
	assert.equal(displayVersion(''), 'Unknown');
});
