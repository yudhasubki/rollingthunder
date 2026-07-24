import assert from 'node:assert/strict';
import test from 'node:test';

import { buildCSVExportOptions, formatExportBytes } from '../src/lib/export/csv.ts';

test('builds explicit CSV options for the backend contract', () => {
	assert.deepEqual(
		buildCSVExportOptions({
			scope: 'all',
			delimiter: '\t',
			includeHeader: false,
			nullValue: 'NULL'
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

test('formats exported file sizes for completion feedback', () => {
	assert.equal(formatExportBytes(0), '0 B');
	assert.equal(formatExportBytes(512), '512 B');
	assert.equal(formatExportBytes(1536), '1.5 KB');
	assert.equal(formatExportBytes(2 * 1024 * 1024), '2.0 MB');
});
