import assert from 'node:assert/strict';
import test from 'node:test';

import { getForeignRelation } from '../src/lib/table/relations.ts';

test('prefers typed foreign-key metadata', () => {
	assert.deepEqual(
		getForeignRelation(
			{
				foreign_schema: 'billing',
				foreign_table: 'accounts',
				foreign_column: 'id',
				foreign_key: 'ignored(value)'
			},
			'public'
		),
		{ schema: 'billing', table: 'accounts', column: 'id' }
	);
});

test('parses schema-qualified and legacy foreign-key references', () => {
	assert.deepEqual(getForeignRelation({ foreign_key: 'public.organizations(id)' }, 'public'), {
		schema: 'public',
		table: 'organizations',
		column: 'id'
	});
	assert.deepEqual(getForeignRelation({ foreign_key: 'organizations(id)' }, 'public'), {
		schema: 'public',
		table: 'organizations',
		column: 'id'
	});
	assert.deepEqual(getForeignRelation({ foreign_key: 'organizations.id' }, 'public'), {
		schema: 'public',
		table: 'organizations',
		column: 'id'
	});
});

test('preserves quoted identifiers containing dots', () => {
	assert.deepEqual(
		getForeignRelation({ foreign_key: '"odd.schema"."Order.Items"("Primary.Id")' }, 'public'),
		{
			schema: 'odd.schema',
			table: 'Order.Items',
			column: 'Primary.Id'
		}
	);
});

test('returns null when no relation metadata exists', () => {
	assert.equal(getForeignRelation({}, 'public'), null);
});
