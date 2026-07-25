import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import test from 'node:test';

const sourceRoot = new URL('../src/', import.meta.url);
const rawPalette =
	/(?:^|[\s"'`])(?:hover:|dark:|focus:|group-hover:)?(?:(?:bg|text|border|ring|from|to|via)-(?:red|rose|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|slate|gray|zinc|neutral|stone|black|white)-\d{2,3}(?:\/\d+)?|bg-black(?:\/\d+)?|text-white|border-white(?:\/\d+)?|ring-white(?:\/\d+)?)/;
const literalColor = /#[0-9a-f]{6,8}\b/i;

async function sourceFiles(directory = sourceRoot) {
	const entries = await readdir(directory, { withFileTypes: true });
	const files = [];
	for (const entry of entries) {
		const child = new URL(`${entry.name}${entry.isDirectory() ? '/' : ''}`, directory);
		if (entry.isDirectory()) {
			if (child.pathname.includes('/lib/wailsjs/')) continue;
			files.push(...(await sourceFiles(child)));
		} else if (entry.name.endsWith('.svelte') || entry.name.endsWith('.ts')) {
			files.push(child);
		}
	}
	return files;
}

test('components consume semantic colors instead of raw palette values', async () => {
	for (const file of await sourceFiles()) {
		const content = await readFile(file, 'utf8');
		assert.doesNotMatch(content, rawPalette, `${file.pathname} contains a raw utility color`);
		assert.doesNotMatch(content, literalColor, `${file.pathname} contains a literal color`);
	}
});

test('components use the shared dropdown instead of native select styling', async () => {
	for (const file of await sourceFiles()) {
		if (!file.pathname.endsWith('.svelte')) continue;
		const content = await readFile(file, 'utf8');
		assert.doesNotMatch(content, /<select\b/, `${file.pathname} contains a native select`);
	}
});

test('the theme defines the complete semantic feedback palette', async () => {
	const styles = await readFile(new URL('../src/app.css', import.meta.url), 'utf8');
	for (const token of ['info', 'success', 'warning', 'danger']) {
		assert.match(styles, new RegExp(`--${token}:`), `missing --${token}`);
		assert.match(styles, new RegExp(`--${token}-soft:`), `missing --${token}-soft`);
		assert.match(styles, new RegExp(`--${token}-border:`), `missing --${token}-border`);
	}
});

test('connection profiles use risk environments, not arbitrary presentation colors', async () => {
	const manager = await readFile(
		new URL('../src/lib/components/ConnectionManagerModal.svelte', import.meta.url),
		'utf8'
	);
	assert.match(manager, /connectionEnvironment/);
	assert.match(manager, /Environment/);
	assert.doesNotMatch(manager, /profileColors|connectionColor|style="background-color/);
});
