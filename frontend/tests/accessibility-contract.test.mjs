import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

async function source(path) {
	return readFile(new URL(`../${path}`, import.meta.url), 'utf8');
}

test('application shell exposes a skip target and polite status region', async () => {
	const [layout, landing, workspace, status] = await Promise.all([
		source('src/routes/+layout.svelte'),
		source('src/routes/+page.svelte'),
		source('src/routes/workspace/+page.svelte'),
		source('src/lib/components/layout/AppStatusBar.svelte')
	]);

	assert.match(layout, /href="#main-content"/);
	assert.match(landing, /id="main-content"/);
	assert.match(workspace, /id="main-content"/);
	assert.match(status, /role="status"/);
	assert.match(status, /aria-live="polite"/);
});

test('blocking application dialogs use the shared focus trap', async () => {
	const dialogs = [
		'src/lib/components/CommandPalette.svelte',
		'src/lib/components/ConnectionManagerModal.svelte',
		'src/lib/components/DiagnosticsDialog.svelte',
		'src/lib/components/database/ExportDialog.svelte',
		'src/lib/components/database/ImportDataDialog.svelte',
		'src/lib/components/database/ObjectChangeDialog.svelte',
		'src/lib/components/database/RowDetailDrawer.svelte',
		'src/lib/components/query/QueryToolingSettings.svelte',
		'src/lib/components/query/QueryVariablesDialog.svelte',
		'src/lib/components/query/SavedQueriesDrawer.svelte',
		'src/routes/workspace/+page.svelte'
	];

	for (const path of dialogs) {
		const content = await source(path);
		assert.match(content, /use:focusTrap/, `${path} must trap keyboard focus`);
		assert.match(content, /aria-modal="true"/, `${path} must identify itself as modal`);
	}
});

test('global styles preserve visible focus and reduced-motion preferences', async () => {
	const styles = await source('src/app.css');
	assert.match(styles, /:focus-visible/);
	assert.match(styles, /prefers-reduced-motion:\s*reduce/);
	assert.match(styles, /\.rt-skip-link/);
});
