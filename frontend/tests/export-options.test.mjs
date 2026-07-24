import assert from 'node:assert/strict';
import test from 'node:test';

import {
	buildExportOptions,
	formatExportBytes,
	getExportExtension
} from '../src/lib/export/options.ts';

test('builds explicit CSV options for the backend contract', () => {
	assert.deepEqual(
		buildExportOptions({
			scope: 'all',
			format: 'csv',
			delimiter: '\t',
			includeHeader: false,
			nullValue: 'NULL',
			prettyJSON: true
		}),
		{
			format: 'csv',
			csv: {
				delimiter: '\t',
				includeHeader: false,
				nullValue: 'NULL'
			}
		}
	);
});

test('builds JSON options without leaking CSV settings', () => {
	assert.deepEqual(
		buildExportOptions({
			scope: 'loaded',
			format: 'json',
			delimiter: ';',
			includeHeader: false,
			nullValue: 'NULL',
			prettyJSON: false
		}),
		{
			format: 'json',
			json: {
				pretty: false
			}
		}
	);
	assert.equal(getExportExtension('csv'), 'csv');
	assert.equal(getExportExtension('json'), 'json');
});

test('formats exported file sizes for completion feedback', () => {
	assert.equal(formatExportBytes(0), '0 B');
	assert.equal(formatExportBytes(512), '512 B');
	assert.equal(formatExportBytes(1536), '1.5 KB');
	assert.equal(formatExportBytes(2 * 1024 * 1024), '2.0 MB');
});
