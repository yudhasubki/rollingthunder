import assert from 'node:assert/strict';
import test from 'node:test';

import {
	BACKEND_RESTART_MESSAGE,
	hasBackendMethod,
	isBackendVersionMismatch
} from '../src/lib/wails/backendCompatibility.ts';

test('detects whether the native Wails backend exposes a required method', () => {
	assert.equal(hasBackendMethod('GetDiagnosticsSettings'), false);

	globalThis.window = {
		go: {
			db: {
				Service: {
					GetDiagnosticsSettings() {}
				}
			}
		}
	};
	assert.equal(hasBackendMethod('GetDiagnosticsSettings'), true);
	assert.equal(hasBackendMethod('ReconnectConnection'), false);
	delete globalThis.window;
});

test('recognizes stale backend errors and provides an actionable restart message', () => {
	assert.match(BACKEND_RESTART_MESSAGE, /stop the old Wails process/i);
	assert.equal(
		isBackendVersionMismatch(
			new Error("window['go']['db']['Service']['GetDiagnosticsSettings'] is not a function")
		),
		true
	);
	assert.equal(
		isBackendVersionMismatch(
			new Error('json: cannot unmarshal object into Go value of type []db.SavedConnection')
		),
		true
	);
	assert.equal(isBackendVersionMismatch(new Error('connection refused')), false);
});
