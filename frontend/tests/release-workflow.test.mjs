import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const workflowPath = new URL('../../.github/workflows/release.yml', import.meta.url);

test('a published GitHub release builds and receives native artifacts', async () => {
	const workflow = await readFile(workflowPath, 'utf8');

	assert.match(workflow, /on:\n  release:\n    types: \[published\]\n  workflow_dispatch:/);
	assert.match(workflow, /if: github\.event_name == 'release'/);
	assert.match(workflow, /gh release upload "\$RELEASE_TAG" dist\/\* --clobber/);
	assert.doesNotMatch(workflow, /github\.event_name == 'push'/);
	assert.doesNotMatch(workflow, /^\s+push:\s*$/m);
});

test('platform signing is optional but partial configuration fails closed', async () => {
	const workflow = await readFile(workflowPath, 'utf8');

	assert.match(workflow, /Validate optional Windows signing configuration/);
	assert.match(workflow, /Validate optional Linux signing configuration/);
	assert.match(
		workflow,
		/if: env\.WINDOWS_CERTIFICATE_BASE64 != '' \|\| env\.WINDOWS_CERTIFICATE_PASSWORD != ''/
	);
	assert.match(
		workflow,
		/if: env\.LINUX_GPG_PRIVATE_KEY_BASE64 != '' \|\| env\.LINUX_GPG_PASSPHRASE != ''/
	);
	assert.doesNotMatch(workflow, /Require signing credentials/);
});
