import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { productVersion, updateWailsVersion } from '../../.github/scripts/set-release-version.mjs';

test('normalizes release tags to the numeric Wails package version', () => {
	assert.equal(productVersion('v1.4.2'), '1.4.2');
	assert.equal(productVersion('v2.0.0-rc.1'), '2.0.0');
	assert.throws(() => productVersion('release/latest'), /Invalid release version/);
});

test('updates only Wails product version metadata', async () => {
	const directory = await mkdtemp(join(tmpdir(), 'rollingthunder-release-'));
	const path = join(directory, 'wails.json');
	await writeFile(
		path,
		JSON.stringify({
			name: 'rollingthunder',
			info: { productName: 'Rolling Thunder', productVersion: '0.0.1' }
		}),
		'utf8'
	);

	await updateWailsVersion('v3.1.4', path);
	const updated = JSON.parse(await readFile(path, 'utf8'));
	assert.equal(updated.info.productName, 'Rolling Thunder');
	assert.equal(updated.info.productVersion, '3.1.4');
});
