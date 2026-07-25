import assert from 'node:assert/strict';
import test from 'node:test';
import {
	createObjectDefinitionTemplate,
	defaultObjectName,
	structuralIntentLabel
} from '../src/lib/database/changeTemplates.ts';

const postgres = {
	engine: 'postgres',
	dialect: { identifierOpen: '"', identifierClose: '"' }
};
const mysql = {
	engine: 'mysql',
	dialect: { identifierOpen: '`', identifierClose: '`' }
};
const oracle = {
	engine: 'oracle',
	dialect: { identifierOpen: '"', identifierClose: '"' }
};
const sqlserver = {
	engine: 'sqlserver',
	dialect: { identifierOpen: '[', identifierClose: ']' }
};

test('builds dialect-aware routine templates', () => {
	const pg = createObjectDefinitionTemplate(
		'create-function',
		postgres,
		'public',
		'refresh_report'
	);
	assert.match(pg, /CREATE OR REPLACE FUNCTION "public"\."refresh_report"\(\)/);
	assert.match(pg, /\$function\$/);

	const my = createObjectDefinitionTemplate('create-procedure', mysql, 'analytics', 'refresh');
	assert.match(my, /CREATE PROCEDURE `analytics`\.`refresh`\(\)/);

	const ora = createObjectDefinitionTemplate('create-function', oracle, 'APP', 'ANSWER');
	assert.match(ora, /CREATE FUNCTION "APP"\."ANSWER"/);
	assert.match(ora, /RETURN NUMBER/);

	const mssql = createObjectDefinitionTemplate(
		'create-procedure',
		sqlserver,
		'dbo',
		'refresh_report'
	);
	assert.match(mssql, /CREATE PROCEDURE \[dbo\]\.\[refresh_report\]/);
	assert.match(mssql, /SET NOCOUNT ON/);
});

test('builds a trigger template against the selected parent table', () => {
	const template = createObjectDefinitionTemplate(
		'create-trigger',
		postgres,
		'audit',
		'capture_insert',
		'orders'
	);
	assert.match(template, /AFTER INSERT ON "audit"\."orders"/);

	const mssql = createObjectDefinitionTemplate(
		'create-trigger',
		sqlserver,
		'audit',
		'capture_insert',
		'orders'
	);
	assert.match(mssql, /CREATE TRIGGER \[audit\]\.\[capture_insert\]/);
	assert.match(mssql, /ON \[audit\]\.\[orders\]/);
});

test('provides concise labels and deterministic starter names', () => {
	assert.equal(structuralIntentLabel('alter-column'), 'Alter column');
	assert.equal(defaultObjectName('create-materialized-view'), 'new_materialized_view');
});
