import assert from 'node:assert/strict';
import test from 'node:test';
import {
	countGroupedObjects,
	databaseObjectKey,
	databaseObjectQualifiedName,
	groupDatabaseObjects
} from '../src/lib/database/objects.ts';

function object(kind, name, extras = {}) {
	return {
		displayName: extras.displayName || name,
		description: extras.description || '',
		reference: {
			id: extras.id || '',
			kind,
			schema: extras.schema || 'public',
			name,
			signature: extras.signature || '',
			parentSchema: extras.parentSchema || '',
			parentName: extras.parentName || ''
		}
	};
}

test('groups normalized database objects without mixing kinds', () => {
	const groups = groupDatabaseObjects(
		[
			object('view', 'active_users'),
			object('table', 'users'),
			object('enum', 'account_state'),
			object('domain', 'email_address')
		],
		''
	);

	assert.deepEqual(
		groups.map((group) => [group.id, group.objects.map((item) => item.reference.name)]),
		[
			['tables', ['users']],
			['views', ['active_users']],
			['types', ['account_state', 'email_address']]
		]
	);
	assert.equal(countGroupedObjects(groups), 4);
});

test('search includes signatures and parent relations', () => {
	const objects = [
		object('function', 'refresh', { signature: 'account_id uuid' }),
		object('trigger', 'audit_insert', { parentName: 'accounts' })
	];

	assert.equal(groupDatabaseObjects(objects, 'uuid')[0].objects[0].reference.name, 'refresh');
	assert.equal(
		groupDatabaseObjects(objects, 'accounts')[0].objects[0].reference.name,
		'audit_insert'
	);
});

test('uses opaque IDs for stable keys and formats qualified routines', () => {
	const reference = object('function', 'refresh', {
		id: 'pg_proc:42',
		signature: 'integer'
	}).reference;
	assert.equal(databaseObjectKey(reference), 'pg_proc:42');
	assert.equal(databaseObjectQualifiedName(reference), 'public.refresh(integer)');
});
