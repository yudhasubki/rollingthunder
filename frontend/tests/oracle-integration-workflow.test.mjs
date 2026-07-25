import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const workflowPath = new URL('../../.github/workflows/integration.yml', import.meta.url);

test('Oracle Data Pump CI uses a pinned full image with sufficient shared memory', async () => {
	const workflow = await readFile(workflowPath, 'utf8');
	const oracleJob = workflow.slice(workflow.indexOf('\n  oracle:'));

	assert.match(
		oracleJob,
		/image: container-registry\.oracle\.com\/database\/free:\d+\.\d+\.\d+\.\d+/
	);
	assert.doesNotMatch(oracleJob, /image: .*database\/free:.*-lite/);
	assert.match(oracleJob, /--shm-size=1g/);
	assert.match(oracleJob, /ROLLINGTHUNDER_ORACLE_TEST_DATAPUMP: "1"/);
});
