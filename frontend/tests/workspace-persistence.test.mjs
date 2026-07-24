import assert from 'node:assert/strict';
import test from 'node:test';

import {
	getConnectionWorkspaceKey,
	parseWorkspaceEnvelope,
	persistableWorkspace,
	restoreQueryTabs
} from '../src/lib/tabs/persistence.ts';

test('builds a stable workspace key without credentials', () => {
	const key = getConnectionWorkspaceKey({
		name: 'Production',
		driver: 'postgres',
		host: 'db.example',
		database: 'app'
	});
	assert.equal(key, 'connection:postgres:db.example:app:production');
	assert.equal(
		getConnectionWorkspaceKey({
			profileId: 'profile-1',
			name: 'Changed name',
			driver: 'postgres',
			host: 'other',
			database: 'other'
		}),
		'profile:profile-1'
	);
});

test('persists and restores only query tabs', () => {
	const workspace = persistableWorkspace(
		[
			{
				id: 'table',
				connectionId: 'runtime-a',
				title: 'public.users',
				kind: 'table'
			},
			{
				id: 'query',
				connectionId: 'runtime-a',
				title: 'Active customers',
				kind: 'query',
				sql: 'SELECT * FROM customers',
				savedQueryId: 'saved-1'
			}
		],
		'query'
	);
	assert.equal(workspace.tabs.length, 1);
	assert.equal(workspace.activeTabId, 'query');

	const restored = restoreQueryTabs('runtime-b', workspace);
	assert.equal(restored.length, 1);
	assert.equal(restored[0].connectionId, 'runtime-b');
	assert.equal(restored[0].savedQueryId, 'saved-1');
});

test('rejects unknown workspace storage versions and malformed tabs', () => {
	assert.deepEqual(parseWorkspaceEnvelope('{"version":99,"workspaces":{}}').workspaces, {});
	const parsed = parseWorkspaceEnvelope(
		JSON.stringify({
			version: 1,
			workspaces: {
				main: {
					activeTabId: 'bad',
					tabs: [
						{ id: 'bad', kind: 'table', sql: '' },
						{ id: 'good', kind: 'query', title: 'Query', sql: 'SELECT 1' }
					]
				}
			}
		})
	);
	assert.equal(parsed.workspaces.main.tabs.length, 1);
	assert.equal(parsed.workspaces.main.activeTabId, 'good');
});
