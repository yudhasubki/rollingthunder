import assert from 'node:assert/strict';
import test from 'node:test';

import { getQueryResultPage, QUERY_RESULT_PAGE_SIZE } from '../src/lib/query/results.ts';

test('renders query results in bounded client-side pages', () => {
	const rows = Array.from({ length: 1000 }, (_, index) => ({ id: index + 1 }));

	const firstPage = getQueryResultPage(rows, 0);
	const lastPage = getQueryResultPage(rows, 9);

	assert.equal(QUERY_RESULT_PAGE_SIZE, 100);
	assert.equal(firstPage.length, 100);
	assert.equal(firstPage[0].id, 1);
	assert.equal(firstPage.at(-1).id, 100);
	assert.equal(lastPage.length, 100);
	assert.equal(lastPage[0].id, 901);
	assert.equal(lastPage.at(-1).id, 1000);
	assert.equal(rows.length, 1000);
});

test('normalizes invalid page and page-size input', () => {
	const rows = [1, 2, 3, 4];

	assert.deepEqual(getQueryResultPage(rows, -2, 2), [1, 2]);
	assert.deepEqual(getQueryResultPage(rows, 1.9, 2.8), [3, 4]);
	assert.deepEqual(getQueryResultPage(rows, 0, 0), [1]);
});
