import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const workflowPath = new URL('../../.github/workflows/release.yml', import.meta.url);

test('published and manually retried GitHub releases receive native artifacts', async () => {
	const workflow = await readFile(workflowPath, 'utf8');

	assert.match(
		workflow,
		/on:\n  release:\n    types: \[published\]\n  workflow_dispatch:\n    inputs:\n      release_tag:/
	);
	assert.match(
		workflow,
		/RELEASE_TAG: \$\{\{ github\.event\.release\.tag_name \|\| inputs\.release_tag \|\| '' \}\}/
	);
	assert.match(
		workflow,
		/SOURCE_REF: \$\{\{ github\.event\.release\.tag_name \|\| inputs\.release_tag \|\| github\.ref \}\}/
	);
	assert.equal(
		[
			...workflow.matchAll(
				/uses: actions\/checkout@v6\n        with:\n          ref: \$\{\{ env\.SOURCE_REF \}\}/g
			)
		].length,
		4
	);
	assert.match(workflow, /Verify manual release target/);
	assert.match(workflow, /gh release view "\$RELEASE_TAG" --repo "\$GITHUB_REPOSITORY"/);
	assert.match(workflow, /if: env\.RELEASE_TAG != ''/);
	assert.match(
		workflow,
		/gh release upload "\$RELEASE_TAG" dist\/\* --clobber --repo "\$GITHUB_REPOSITORY"/
	);
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
