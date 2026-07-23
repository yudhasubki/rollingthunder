import assert from 'node:assert/strict';
import test from 'node:test';

import { findTableTabForConnection, findWorkspaceForTab } from '../src/lib/tabs/workspaces.ts';

const workspaces = {
	alpha: {
		activeTabId: 'alpha-orders',
		tabs: [
			{
				id: 'alpha-orders',
				connectionId: 'alpha',
				title: 'public.orders',
				kind: 'table',
				schema: 'public',
				table: 'orders',
				level: 'info'
			},
			{
				id: 'alpha-query',
				connectionId: 'alpha',
				title: 'SQL Query',
				kind: 'query',
				sql: 'select * from orders',
				level: 'info'
			}
		]
	},
	bravo: {
		activeTabId: 'bravo-orders',
		tabs: [
			{
				id: 'bravo-orders',
				connectionId: 'bravo',
				title: 'public.orders',
				kind: 'table',
				schema: 'public',
				table: 'orders',
				level: 'info'
			}
		]
	}
};

test('resolves the same schema and table inside its owning connection only', () => {
	assert.equal(
		findTableTabForConnection(workspaces, 'alpha', 'public', 'orders')?.id,
		'alpha-orders'
	);
	assert.equal(
		findTableTabForConnection(workspaces, 'bravo', 'public', 'orders')?.id,
		'bravo-orders'
	);
	assert.equal(findTableTabForConnection(workspaces, 'missing', 'public', 'orders'), undefined);
});

test('finds the workspace that owns a tab without relying on the active connection', () => {
	assert.equal(findWorkspaceForTab(workspaces, 'alpha-query'), workspaces.alpha);
	assert.equal(findWorkspaceForTab(workspaces, 'bravo-orders'), workspaces.bravo);
	assert.equal(findWorkspaceForTab(workspaces, 'missing'), null);
});
