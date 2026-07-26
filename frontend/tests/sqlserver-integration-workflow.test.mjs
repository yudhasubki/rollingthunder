import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const workflowPath = new URL('../../.github/workflows/integration.yml', import.meta.url);
const tlsSetupPath = new URL('../../.github/scripts/configure-sqlserver-tls.sh', import.meta.url);

test('SQL Server CI exercises administration and required TDS 8.0 Strict TLS', async () => {
	const workflow = await readFile(workflowPath, 'utf8');
	const tlsSetup = await readFile(tlsSetupPath, 'utf8');
	const start = workflow.indexOf('\n  sql-server:');
	const end = workflow.indexOf('\n  oracle:', start);
	const sqlServerJob = workflow.slice(start, end);

	assert.match(sqlServerJob, /version: "2022"/);
	assert.match(sqlServerJob, /image: mcr\.microsoft\.com\/mssql\/server:2022-latest/);
	assert.match(sqlServerJob, /version: "2025"/);
	assert.match(sqlServerJob, /image: mcr\.microsoft\.com\/mssql\/server:2025-CU7-ubuntu-22\.04/);
	assert.match(sqlServerJob, /image: \$\{\{ matrix\.image \}\}/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_TEST_PRIVILEGED: "1"/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP: "1"/);
	assert.match(
		sqlServerJob,
		/go test \.\/pkg\/database\/sqlserver -run TestSQLServerLiveConformance/
	);
	assert.match(sqlServerJob, /configure-sqlserver-tls\.sh/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_REQUIRE_TLS: "1"/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_TLS_WRONG_ROOT_CERT:/);
	assert.match(
		sqlServerJob,
		/go test \.\/pkg\/database\/sqlserver -run TestSQLServerTLSLiveConformance/
	);
	assert.match(sqlServerJob, /Run SQL Server driver conformance over verified TLS/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE: verify-full/);
	assert.match(sqlServerJob, /Run SQL Server TDS 8\.0 Strict TLS conformance/);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_TDS8_STRICT: "1"/);
	assert.match(sqlServerJob, /Run SQL Server driver conformance over TDS 8\.0 Strict/);
	assert.match(
		sqlServerJob,
		/if: matrix\.version == '2025'[\s\S]*ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE: strict/
	);
	assert.match(sqlServerJob, /ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE: strict/);
	assert.match(tlsSetup, /go run \.\/scripts\/testcert/);
	assert.match(tlsSetup, /chown -R mssql:mssql/);
	assert.match(tlsSetup, /chmod 0400/);
	assert.match(tlsSetup, /network\.tlsprotocols 1\.2/);
	assert.match(tlsSetup, /network\.forceencryption 1/);
	assert.match(tlsSetup, /docker restart/);
});
