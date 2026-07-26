import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const workflowPath = new URL('../../.github/workflows/integration.yml', import.meta.url);
const postgresSetupPath = new URL(
	'../../.github/scripts/configure-postgres-tls.sh',
	import.meta.url
);
const mysqlSetupPath = new URL('../../.github/scripts/configure-mysql-tls.sh', import.meta.url);

test('PostgreSQL matrix requires live server and client TLS conformance', async () => {
	const workflow = await readFile(workflowPath, 'utf8');
	const start = workflow.indexOf('\n  postgres:');
	const end = workflow.indexOf('\n  mysql-compatible:', start);
	const postgresJob = workflow.slice(start, end);
	const setup = await readFile(postgresSetupPath, 'utf8');

	assert.match(postgresJob, /version: \["14", "15", "16", "17", "18"\]/);
	assert.match(postgresJob, /configure-postgres-tls\.sh/);
	assert.match(postgresJob, /ROLLINGTHUNDER_POSTGRES_TEST_REQUIRE_TLS: "1"/);
	assert.match(postgresJob, /ROLLINGTHUNDER_POSTGRES_TEST_TLS_WRONG_ROOT_CERT:/);
	assert.match(postgresJob, /ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_CERT:/);
	assert.match(
		postgresJob,
		/go test \.\/pkg\/database\/postgres -run TestPostgresTLSLiveConformance/
	);
	assert.match(postgresJob, /Run PostgreSQL driver conformance over verified TLS/);
	assert.match(postgresJob, /ROLLINGTHUNDER_POSTGRES_TEST_SSL_MODE: verify-full/);
	assert.match(setup, /go run \.\/scripts\/testcert/);
	assert.match(setup, /hostssl all \$\{client_role\} .* cert/);
	assert.match(setup, /hostnossl all all .* reject/);
});

test('every MySQL and MariaDB matrix entry requires live TLS conformance', async () => {
	const workflow = await readFile(workflowPath, 'utf8');
	const start = workflow.indexOf('\n  mysql-compatible:');
	const end = workflow.indexOf('\n  sql-server:', start);
	const mysqlJob = workflow.slice(start, end);
	const setup = await readFile(mysqlSetupPath, 'utf8');

	for (const image of [
		'mysql:8.0',
		'mysql:8.4',
		'mysql:9.7',
		'mariadb:10.11',
		'mariadb:11.4',
		'mariadb:11.8',
		'mariadb:12.3'
	]) {
		assert.match(mysqlJob, new RegExp(`image: ${image.replace('.', '\\.')}`));
	}
	assert.match(mysqlJob, /configure-mysql-tls\.sh/);
	assert.match(mysqlJob, /ROLLINGTHUNDER_MYSQL_TEST_REQUIRE_TLS: "1"/);
	assert.match(mysqlJob, /ROLLINGTHUNDER_MYSQL_TEST_TLS_WRONG_ROOT_CERT:/);
	assert.match(mysqlJob, /ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_CERT:/);
	assert.match(mysqlJob, /go test \.\/pkg\/database\/mysql -run TestMySQLTLSLiveConformance/);
	assert.match(mysqlJob, /Run MySQL-compatible driver conformance over verified TLS/);
	assert.match(mysqlJob, /ROLLINGTHUNDER_MYSQL_TEST_SSL_MODE: verify-full/);
	assert.match(setup, /go run \.\/scripts\/testcert/);
	assert.match(setup, /require_secure_transport=ON/);
});
