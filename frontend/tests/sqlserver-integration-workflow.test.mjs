import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const workflowPath = new URL('../../.github/workflows/integration.yml', import.meta.url);

test('SQL Server CI exercises privileged security and native backup restore', async () => {
	const workflow = await readFile(workflowPath, 'utf8');
	const start = workflow.indexOf('\n  sql-server:');
	const end = workflow.indexOf('\n  oracle:', start);
	const sqlServerJob = workflow.slice(start, end);

	assert.match(sqlServerJob, /matrix:\s*\n\s*version: \["2022", "2025"\]/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_TEST_PRIVILEGED: "1"/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP: "1"/);
	assert.match(
		sqlServerJob,
		/go test \.\/pkg\/database\/sqlserver -run TestSQLServerLiveConformance/
	);
});
