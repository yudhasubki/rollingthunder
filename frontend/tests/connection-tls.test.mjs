import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

import {
	SSL_OPTIONS,
	normalizeTLSModeForProvider,
	tlsModeAvailableForProvider,
	tlsModeVerifiesServerCertificate
} from '../src/lib/config/application.ts';

test('TDS 8.0 Strict is a SQL Server-only verified TLS mode', () => {
	assert.deepEqual(
		SSL_OPTIONS.find((option) => option.value === 'strict'),
		{ value: 'strict', label: 'Strict (TDS 8.0)' }
	);
	assert.equal(tlsModeAvailableForProvider('strict', 'sqlserver'), true);
	assert.equal(tlsModeAvailableForProvider('strict', 'postgres'), false);
	assert.equal(tlsModeAvailableForProvider('verify-full', 'postgres'), true);
	assert.equal(tlsModeVerifiesServerCertificate('strict'), true);
	assert.equal(normalizeTLSModeForProvider('STRICT', 'sqlserver'), 'strict');
	assert.equal(normalizeTLSModeForProvider('strict', 'mysql'), 'verify-full');
	assert.equal(normalizeTLSModeForProvider('unsupported', 'sqlserver'), 'disable');
});

test('the connection manager filters, explains, and safely normalizes Strict mode', async () => {
	const manager = await readFile(
		new URL('../src/lib/components/ConnectionManagerModal.svelte', import.meta.url),
		'utf8'
	);

	assert.match(manager, /tlsModeAvailableForProvider\(option\.value, provider\)/);
	assert.match(manager, /normalizeTLSModeForProvider\(sslMode, nextProvider\.id\)/);
	assert.match(manager, /tlsModeVerifiesServerCertificate\(sslMode\)/);
	assert.match(manager, /Use\s+SQL Server 2025\+ on Linux/);
	assert.match(manager, /SQL Server 2022 Linux accepts Verify full/);
	assert.match(manager, /issuer is not in the system trust store/);
});
