import assert from 'node:assert/strict';
import test from 'node:test';

import { createServiceError } from '../src/lib/errors/service.ts';

test('formats a stable service error code with its actionable hint', () => {
	const error = createServiceError(
		{
			code: 'CONNECTION_REFUSED',
			detail: 'The database refused the connection.',
			hint: 'Check that PostgreSQL is running.'
		},
		'Connection failed'
	);

	assert.equal(error.code, 'CONNECTION_REFUSED');
	assert.equal(
		error.message,
		'[CONNECTION_REFUSED] The database refused the connection. — Check that PostgreSQL is running.'
	);
});

test('uses the fallback when the backend error is unavailable', () => {
	const error = createServiceError(undefined, 'Connection failed');

	assert.equal(error.code, undefined);
	assert.equal(error.message, 'Connection failed');
});
