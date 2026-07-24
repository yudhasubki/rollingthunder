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
			csvEncoding: 'utf-16le',
			includeHeader: false,
			nullValue: 'NULL',
			prettyJSON: true,
			sqlBatchSize: 100,
			includeTransaction: true
		}),
		{
			format: 'csv',
			csv: {
				delimiter: '\t',
				encoding: 'utf-16le',
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
			csvEncoding: 'utf-8-bom',
			includeHeader: false,
			nullValue: 'NULL',
			prettyJSON: false,
			sqlBatchSize: 100,
			includeTransaction: true
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

test('builds PostgreSQL INSERT options without leaking other format settings', () => {
	assert.deepEqual(
		buildExportOptions({
			scope: 'all',
			format: 'sql',
			delimiter: ',',
			csvEncoding: 'utf-8',
			includeHeader: true,
			nullValue: '',
			prettyJSON: true,
			sqlBatchSize: 500,
			includeTransaction: false
		}),
		{
			format: 'sql',
			sql: {
				batchSize: 500,
				includeTransaction: false
			}
		}
	);
	assert.equal(getExportExtension('sql'), 'sql');
});

test('formats exported file sizes for completion feedback', () => {
	assert.equal(formatExportBytes(0), '0 B');
	assert.equal(formatExportBytes(512), '512 B');
	assert.equal(formatExportBytes(1536), '1.5 KB');
	assert.equal(formatExportBytes(2 * 1024 * 1024), '2.0 MB');
});
