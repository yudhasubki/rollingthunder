import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const headerPath = new URL('../src/lib/components/layout/AppHeader.svelte', import.meta.url);
const workspacePath = new URL('../src/routes/workspace/+page.svelte', import.meta.url);

test('the header opens the command palette through a direct workspace callback', async () => {
	const [header, workspace] = await Promise.all([
		readFile(headerPath, 'utf8'),
		readFile(workspacePath, 'utf8')
	]);

	assert.match(header, /onOpenCommandPalette:\s*\(\)\s*=>\s*void/);
	assert.match(header, /function openCommandPalette\(\)\s*\{\s*onOpenCommandPalette\(\);\s*\}/);
	assert.doesNotMatch(header, /CustomEvent\(['"]open-command-palette['"]\)/);
	assert.match(
		workspace,
		/<AppHeader onOpenCommandPalette=\{\(\) => \(commandPaletteOpen = true\)\} \/>/
	);
});
