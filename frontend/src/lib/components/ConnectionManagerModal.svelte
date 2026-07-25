<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Connect,
		ConnectWithProfile,
		ChooseOracleTNSFile,
		ChooseOracleWalletDirectory,
		ChooseSQLiteDatabaseFile,
		ClearConnectionOracleWalletPassword,
		ClearConnectionPassword,
		ClearConnectionSSHCredentials,
		DeleteConnection,
		GetSavedConnections,
		SaveConnection,
		UpdateConnection
	} from '$lib/wailsjs/go/db/Service';
	import {
		CONNECTION_TIMEOUT_SECONDS,
		cancelConnectionAttempt,
		createConnectionAttemptID,
		startConnectionElapsedTimer
	} from '$lib/connection/attempt';
	import { createServiceError } from '$lib/errors/service';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { database, db } from '$lib/wailsjs/go/models';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { focusTrap } from '$lib/actions/focusTrap';
	import {
		APPLICATION,
		CONNECTION_ACCESS_MODES,
		CONNECTION_DEFAULTS,
		CONNECTION_ENVIRONMENTS,
		DATABASE_PROVIDERS,
		ORACLE_CONNECTION_MODES,
		SQL_SERVER_AUTH_MODES,
		SSH_AUTH_OPTIONS,
		SSL_OPTIONS,
		connectionEnvironmentOption,
		normalizeConnectionAccessMode,
		normalizeConnectionEnvironment,
		type ConnectionAccessMode,
		type ConnectionEnvironment,
		type OracleConnectionMode,
		type ProviderId,
		type SQLServerAuthMode
	} from '$lib/config/application';
	import {
		AlertCircle,
		ArrowLeft,
		Check,
		ChevronDown,
		ChevronRight,
		Database,
		Eye,
		EyeOff,
		FilePlus2,
		FolderOpen,
		Loader2,
		Lock,
		Network,
		Play,
		Plus,
		Save,
		Search,
		Server,
		ShieldCheck,
		Trash2,
		X
	} from 'lucide-svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
		onConnected?: () => void;
		initialProfileId?: string | null;
		startNew?: boolean;
	}

	let {
		open,
		onClose,
		onConnected = () => {},
		initialProfileId = null,
		startNew = false
	}: Props = $props();

	const providers = DATABASE_PROVIDERS;
	const sshAuthOptions = SSH_AUTH_OPTIONS.map((option) => ({ ...option }));
	const environmentOptions = CONNECTION_ENVIRONMENTS.map(({ value, label }) => ({
		value,
		label
	}));
	const accessModeOptions = CONNECTION_ACCESS_MODES.map(({ value, label }) => ({
		value,
		label
	}));
	const oracleConnectionModeOptions = ORACLE_CONNECTION_MODES.map((option) => ({ ...option }));
	const sqlServerAuthModeOptions = SQL_SERVER_AUTH_MODES.map(({ value, label }) => ({
		value,
		label
	}));

	let profiles = $state<db.SavedConnection[]>([]);
	let searchQuery = $state('');
	let loadingProfiles = $state(false);
	let editingId = $state<string | null>(null);
	let action = $state<'save' | 'connect' | 'delete' | null>(null);
	let message = $state('');
	let messageLevel = $state<'info' | 'error' | 'success'>('info');
	let deleteConfirm = $state(false);
	let showPassword = $state(false);
	let hasStoredPassword = $state(false);
	let clearPasswordConfirm = $state(false);
	let sshExpanded = $state(false);
	let sshEnabled = $state(false);
	let sshHost = $state('');
	let sshPort = $state(CONNECTION_DEFAULTS.sshPort);
	let sshUser = $state('');
	let sshAuthMode = $state('agent');
	let sshPrivateKeyPath = $state('');
	let sshKnownHostsPath = $state('');
	let sshHostKeyFingerprint = $state('');
	let sshPassword = $state('');
	let sshKeyPassphrase = $state('');
	let hasStoredSSHPassword = $state(false);
	let hasStoredSSHKeyPassphrase = $state(false);
	let showSSHSecret = $state(false);
	let clearSSHCredentialsConfirm = $state(false);
	let oracleConnectionMode = $state<OracleConnectionMode>(CONNECTION_DEFAULTS.oracleConnectionMode);
	let oracleTnsConfigPath = $state('');
	let oracleTnsAlias = $state('');
	let oracleTnsAliases = $state<string[]>([]);
	let oracleWalletPath = $state('');
	let oracleWalletHasAutoLogin = $state(false);
	let oracleWalletPasswordRequired = $state(false);
	let oracleWalletPassword = $state('');
	let hasStoredOracleWalletPassword = $state(false);
	let showOracleWalletPassword = $state(false);
	let clearOracleWalletPasswordConfirm = $state(false);
	let sqlServerAuthMode = $state<SQLServerAuthMode>(CONNECTION_DEFAULTS.sqlServerAuthMode);
	let loadedSQLServerAuthMode = $state<SQLServerAuthMode>(CONNECTION_DEFAULTS.sqlServerAuthMode);
	let loadedProvider = $state<ProviderId | ''>('');
	let sqlServerEntraClientId = $state('');
	let sqlServerEntraTenantId = $state('');
	let loadedForOpen = $state(false);
	let connectionAttemptID = $state<string | null>(null);
	let connectionElapsedSeconds = $state(0);
	let cancellingConnection = $state(false);
	let stopConnectionElapsedTimer: (() => void) | null = null;

	let connectionName = $state('');
	let connectionEnvironment = $state<ConnectionEnvironment>(CONNECTION_DEFAULTS.environment);
	let accessMode = $state<ConnectionAccessMode>(CONNECTION_DEFAULTS.accessMode);
	let accessModeTouched = $state(false);
	let folder = $state('');
	let tagsText = $state('');
	let host = $state(CONNECTION_DEFAULTS.host);
	let port = $state(
		DATABASE_PROVIDERS.find((item) => item.id === CONNECTION_DEFAULTS.provider)?.defaultPort ?? ''
	);
	let username = $state('');
	let password = $state('');
	let databaseName = $state('');
	let sslMode = $state(CONNECTION_DEFAULTS.sslMode);
	let sslRootCert = $state('');
	let sslCert = $state('');
	let sslKey = $state('');
	let provider = $state<ProviderId | ''>('');

	const filteredProfiles = $derived(
		searchQuery.trim()
			? profiles.filter((profile) => {
					const config = profile.config;
					return `${config.name} ${config.driver || CONNECTION_DEFAULTS.provider} ${config.host} ${config.db} ${config.folder || ''} ${(config.tags || []).join(' ')}`
						.toLowerCase()
						.includes(searchQuery.trim().toLowerCase());
				})
			: profiles
	);
	const groupedProfiles = $derived.by(() => {
		const groups = new Map<string, db.SavedConnection[]>();
		for (const profile of filteredProfiles) {
			const group = profile.config.folder?.trim() || 'Ungrouped';
			groups.set(group, [...(groups.get(group) || []), profile]);
		}
		return [...groups.entries()]
			.sort(([left], [right]) => {
				if (left === 'Ungrouped') return 1;
				if (right === 'Ungrouped') return -1;
				return left.localeCompare(right);
			})
			.map(([name, items]) => ({ name, items }));
	});

	const oracleUsesTNS = $derived(provider === 'oracle' && oracleConnectionMode === 'tns');
	const oracleUsesWallet = $derived(provider === 'oracle' && Boolean(oracleWalletPath.trim()));
	const sqlServerIntegrated = $derived(
		provider === 'sqlserver' && sqlServerAuthMode === 'integrated'
	);
	const sqlServerUsesUsername = $derived(
		provider !== 'sqlserver' ||
			sqlServerAuthMode === 'sql' ||
			sqlServerAuthMode === 'entra-password'
	);
	const sqlServerUsesPassword = $derived(
		provider !== 'sqlserver' ||
			sqlServerAuthMode === 'sql' ||
			sqlServerAuthMode === 'entra-password' ||
			sqlServerAuthMode === 'entra-service-principal'
	);
	const sqlServerUsesClientId = $derived(
		provider === 'sqlserver' &&
			(sqlServerAuthMode === 'entra-password' ||
				sqlServerAuthMode === 'entra-service-principal' ||
				sqlServerAuthMode === 'entra-managed-identity')
	);
	const sqlServerUsesTenantId = $derived(
		provider === 'sqlserver' && sqlServerAuthMode === 'entra-service-principal'
	);
	const selectedSQLServerAuth = $derived(
		SQL_SERVER_AUTH_MODES.find((option) => option.value === sqlServerAuthMode) ??
			SQL_SERVER_AUTH_MODES[0]
	);
	const storedPasswordMatchesAuth = $derived(
		hasStoredPassword &&
			loadedProvider === provider &&
			(provider !== 'sqlserver' || loadedSQLServerAuthMode === sqlServerAuthMode)
	);
	const sslOptions = $derived(
		SSL_OPTIONS.filter(
			(option) =>
				(!oracleUsesWallet || option.value !== 'verify-ca') &&
				!(
					provider === 'sqlserver' &&
					sqlServerAuthMode.startsWith('entra-') &&
					option.value === 'disable'
				)
		).map((option) => ({ ...option }))
	);
	const oracleTnsAliasOptions = $derived(
		oracleTnsAliases.map((alias) => ({ value: alias, label: alias }))
	);
	const endpoint = $derived.by(() => {
		if (provider === 'sqlite') return databaseName || 'Choose a local database file';
		if (oracleUsesTNS) {
			const file = oracleTnsConfigPath.split(/[\\/]/).pop() || 'tnsnames.ora';
			return `${oracleTnsAlias || 'TNS alias'} · ${file}`;
		}
		return `${host || 'host'}:${port || 'port'} / ${databaseName || 'database'}`;
	});
	const selectedProvider = $derived(providers.find((item) => item.id === provider) ?? null);
	const selectedEnvironment = $derived(connectionEnvironmentOption(connectionEnvironment));

	$effect(() => {
		if (open && !loadedForOpen) {
			loadedForOpen = true;
			void loadProfiles(initialProfileId, startNew);
		} else if (!open) {
			loadedForOpen = false;
			deleteConfirm = false;
			message = '';
		}
	});

	onMount(() => {
		const handleKeydown = (event: KeyboardEvent) => {
			if (open && event.key === 'Escape' && !action) onClose();
		};
		window.addEventListener('keydown', handleKeydown);
		return () => {
			window.removeEventListener('keydown', handleKeydown);
			stopConnectionElapsedTimer?.();
			if (connectionAttemptID) {
				void cancelConnectionAttempt(connectionAttemptID).catch(() => {});
			}
		};
	});

	async function loadProfiles(selectId?: string | null, createNew = false) {
		loadingProfiles = true;
		try {
			const response = await GetSavedConnections();
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not load saved connections');
			}
			profiles = response.data || [];

			if (createNew) {
				newProfile();
				return;
			}

			const requestedProfile = profiles.find((profile) => profile.id === selectId);
			const currentProfile = profiles.find((profile) => profile.id === editingId);
			if (requestedProfile) {
				selectProfile(requestedProfile);
			} else if (currentProfile) {
				selectProfile(currentProfile);
			} else if (profiles.length > 0 && editingId === null && !connectionName) {
				selectProfile(profiles[0]);
			}
		} catch (error: any) {
			showMessage(error?.message || 'Could not load saved connections', 'error');
		} finally {
			loadingProfiles = false;
		}
	}

	function showMessage(text: string, level: 'info' | 'error' | 'success' = 'info') {
		message = text;
		messageLevel = level;
	}

	function closeModal() {
		if (!action) onClose();
	}

	function selectProfile(profile: db.SavedConnection) {
		const config = profile.config;
		editingId = profile.id;
		connectionName = config.name || '';
		connectionEnvironment = normalizeConnectionEnvironment(config.environment);
		accessMode = normalizeConnectionAccessMode(config.accessMode, config.environment);
		accessModeTouched = true;
		folder = config.folder || '';
		tagsText = (config.tags || []).join(', ');
		const profileProvider = (config.driver as ProviderId) || CONNECTION_DEFAULTS.provider;
		host = profileProvider === 'sqlite' ? '' : config.host || CONNECTION_DEFAULTS.host;
		port =
			profileProvider === 'sqlite'
				? ''
				: config.port ||
					providers.find((item) => item.id === profileProvider)?.defaultPort ||
					DATABASE_PROVIDERS.find((item) => item.id === CONNECTION_DEFAULTS.provider)
						?.defaultPort ||
					'';
		username = config.user || '';
		password = '';
		hasStoredPassword = Boolean(profile.hasPassword);
		clearPasswordConfirm = false;
		databaseName = config.db || '';
		sslMode = config.sslMode || CONNECTION_DEFAULTS.sslMode;
		sslRootCert = config.sslRootCert || '';
		sslCert = config.sslCert || '';
		sslKey = config.sslKey || '';
		sshEnabled = Boolean(config.sshEnabled);
		sshExpanded = Boolean(config.sshEnabled);
		sshHost = config.sshHost || '';
		sshPort = config.sshPort || CONNECTION_DEFAULTS.sshPort;
		sshUser = config.sshUser || '';
		sshAuthMode = config.sshAuthMode || 'agent';
		sshPrivateKeyPath = config.sshPrivateKeyPath || '';
		sshKnownHostsPath = config.sshKnownHostsPath || '';
		sshHostKeyFingerprint = config.sshHostKeyFingerprint || '';
		sshPassword = '';
		sshKeyPassphrase = '';
		hasStoredSSHPassword = Boolean(profile.hasSshPassword);
		hasStoredSSHKeyPassphrase = Boolean(profile.hasSshKeyPassphrase);
		showSSHSecret = false;
		clearSSHCredentialsConfirm = false;
		oracleConnectionMode =
			config.oracleConnectionMode === 'tns' ? 'tns' : CONNECTION_DEFAULTS.oracleConnectionMode;
		oracleTnsConfigPath = config.oracleTnsConfigPath || '';
		oracleTnsAlias = config.oracleTnsAlias || '';
		oracleTnsAliases = oracleTnsAlias ? [oracleTnsAlias] : [];
		oracleWalletPath = config.oracleWalletPath || '';
		if (oracleWalletPath && sslMode === 'verify-ca') sslMode = 'verify-full';
		hasStoredOracleWalletPassword = Boolean(profile.hasOracleWalletPassword);
		oracleWalletHasAutoLogin = Boolean(oracleWalletPath) && !hasStoredOracleWalletPassword;
		oracleWalletPasswordRequired = Boolean(oracleWalletPath) && hasStoredOracleWalletPassword;
		oracleWalletPassword = '';
		showOracleWalletPassword = false;
		clearOracleWalletPasswordConfirm = false;
		sqlServerAuthMode =
			profileProvider === 'sqlserver' &&
			SQL_SERVER_AUTH_MODES.some((option) => option.value === config.sqlServerAuthMode)
				? (config.sqlServerAuthMode as SQLServerAuthMode)
				: CONNECTION_DEFAULTS.sqlServerAuthMode;
		loadedSQLServerAuthMode = sqlServerAuthMode;
		loadedProvider = profileProvider;
		sqlServerEntraClientId = config.sqlServerEntraClientId || '';
		sqlServerEntraTenantId = config.sqlServerEntraTenantId || '';
		provider = profileProvider;
		deleteConfirm = false;
		message = '';
	}

	function newProfile() {
		editingId = null;
		connectionName = '';
		connectionEnvironment = CONNECTION_DEFAULTS.environment;
		accessMode = CONNECTION_DEFAULTS.accessMode;
		accessModeTouched = false;
		folder = '';
		tagsText = '';
		host = CONNECTION_DEFAULTS.host;
		port =
			DATABASE_PROVIDERS.find((item) => item.id === CONNECTION_DEFAULTS.provider)?.defaultPort ??
			'';
		username = '';
		password = '';
		hasStoredPassword = false;
		clearPasswordConfirm = false;
		databaseName = '';
		sslMode = CONNECTION_DEFAULTS.sslMode;
		sslRootCert = '';
		sslCert = '';
		sslKey = '';
		sshExpanded = false;
		sshEnabled = false;
		sshHost = '';
		sshPort = CONNECTION_DEFAULTS.sshPort;
		sshUser = '';
		sshAuthMode = 'agent';
		sshPrivateKeyPath = '';
		sshKnownHostsPath = '';
		sshHostKeyFingerprint = '';
		sshPassword = '';
		sshKeyPassphrase = '';
		hasStoredSSHPassword = false;
		hasStoredSSHKeyPassphrase = false;
		showSSHSecret = false;
		clearSSHCredentialsConfirm = false;
		oracleConnectionMode = CONNECTION_DEFAULTS.oracleConnectionMode;
		oracleTnsConfigPath = '';
		oracleTnsAlias = '';
		oracleTnsAliases = [];
		oracleWalletPath = '';
		oracleWalletHasAutoLogin = false;
		oracleWalletPasswordRequired = false;
		oracleWalletPassword = '';
		hasStoredOracleWalletPassword = false;
		showOracleWalletPassword = false;
		clearOracleWalletPasswordConfirm = false;
		sqlServerAuthMode = CONNECTION_DEFAULTS.sqlServerAuthMode;
		loadedSQLServerAuthMode = CONNECTION_DEFAULTS.sqlServerAuthMode;
		loadedProvider = '';
		sqlServerEntraClientId = '';
		sqlServerEntraTenantId = '';
		provider = '';
		deleteConfirm = false;
		showPassword = false;
		message = '';
	}

	function isConnected(profile: db.SavedConnection) {
		return connectionStore.connections.some(
			(connection) =>
				connection.profileId === profile.id ||
				(connection.name === profile.config.name &&
					connection.driver === (profile.config.driver || CONNECTION_DEFAULTS.provider) &&
					connection.host === profile.config.host &&
					connection.database === profile.config.db)
		);
	}

	function buildConfig() {
		const sqlite = provider === 'sqlite';
		const oracle = provider === 'oracle';
		const sqlServer = provider === 'sqlserver';
		const oracleTNS = oracle && oracleConnectionMode === 'tns';
		const wallet = oracle && Boolean(oracleWalletPath.trim());
		const allowSSH = !sqlite && !oracleTNS && !wallet && !(sqlServer && sqlServerIntegrated);
		return new database.Config({
			name: connectionName.trim(),
			environment: connectionEnvironment,
			accessMode,
			folder: folder.trim(),
			tags: tagsText
				.split(',')
				.map((tag) => tag.trim())
				.filter(Boolean),
			driver: provider || CONNECTION_DEFAULTS.provider,
			host: sqlite || oracleTNS ? '' : host.trim(),
			port: sqlite || oracleTNS ? '' : port.trim(),
			user: sqlite || (sqlServer && !sqlServerUsesUsername) ? '' : username.trim(),
			password: sqlite || (sqlServer && !sqlServerUsesPassword) ? '' : password,
			db: oracleTNS ? oracleTnsAlias.trim() : databaseName.trim(),
			sslMode: sqlite ? CONNECTION_DEFAULTS.sslMode : sslMode,
			sslRootCert: sqlite || wallet ? '' : sslRootCert.trim(),
			sslCert: sqlite || wallet ? '' : sslCert.trim(),
			sslKey: sqlite || wallet ? '' : sslKey.trim(),
			oracleConnectionMode: oracle ? oracleConnectionMode : '',
			oracleTnsConfigPath: oracleTNS ? oracleTnsConfigPath.trim() : '',
			oracleTnsAlias: oracleTNS ? oracleTnsAlias.trim() : '',
			oracleWalletPath: oracle ? oracleWalletPath.trim() : '',
			oracleWalletPassword: oracle && wallet ? oracleWalletPassword : '',
			sqlServerAuthMode: sqlServer ? sqlServerAuthMode : '',
			sqlServerEntraClientId:
				sqlServer && sqlServerUsesClientId ? sqlServerEntraClientId.trim() : '',
			sqlServerEntraTenantId:
				sqlServer && sqlServerUsesTenantId ? sqlServerEntraTenantId.trim() : '',
			sshEnabled: allowSSH && sshEnabled,
			sshHost: allowSSH && sshEnabled ? sshHost.trim() : '',
			sshPort: allowSSH && sshEnabled ? sshPort.trim() || CONNECTION_DEFAULTS.sshPort : '',
			sshUser: allowSSH && sshEnabled ? sshUser.trim() : '',
			sshAuthMode: allowSSH && sshEnabled ? sshAuthMode : '',
			sshPrivateKeyPath:
				allowSSH && sshEnabled && sshAuthMode === 'private-key' ? sshPrivateKeyPath.trim() : '',
			sshKnownHostsPath: allowSSH && sshEnabled ? sshKnownHostsPath.trim() : '',
			sshHostKeyFingerprint: allowSSH && sshEnabled ? sshHostKeyFingerprint.trim() : '',
			sshPassword: allowSSH && sshEnabled && sshAuthMode === 'password' ? sshPassword : '',
			sshKeyPassphrase:
				allowSSH && sshEnabled && sshAuthMode === 'private-key' ? sshKeyPassphrase : ''
		});
	}

	function validate(requireName = true) {
		if (requireName && !connectionName.trim()) {
			showMessage('Add a profile name before saving.', 'error');
			return false;
		}
		if (!provider) {
			showMessage('Choose a database provider first.', 'error');
			return false;
		}
		if (provider === 'sqlite' && !databaseName.trim()) {
			showMessage('Choose an existing SQLite file or a path for a new database.', 'error');
			return false;
		}
		if (
			provider === 'oracle' &&
			oracleConnectionMode === 'tns' &&
			(!oracleTnsConfigPath.trim() || !oracleTnsAlias.trim())
		) {
			showMessage('Choose a tnsnames.ora file and TNS alias.', 'error');
			return false;
		}
		if (
			provider !== 'sqlite' &&
			!(provider === 'oracle' && oracleConnectionMode === 'tns') &&
			(!host.trim() || !port.trim() || !databaseName.trim())
		) {
			showMessage(
				`Host, port, and ${selectedProvider?.databaseLabel.toLowerCase() || 'database'} are required.`,
				'error'
			);
			return false;
		}
		if (provider === 'sqlserver') {
			if (
				(sqlServerAuthMode === 'sql' || sqlServerAuthMode === 'entra-password') &&
				!username.trim()
			) {
				showMessage(
					'Enter the username required by the selected SQL Server authentication.',
					'error'
				);
				return false;
			}
			if (sqlServerUsesPassword && !password && !(editingId && storedPasswordMatchesAuth)) {
				showMessage(
					sqlServerAuthMode === 'entra-service-principal'
						? 'Enter the Microsoft Entra client secret.'
						: 'Enter the password required by the selected authentication.',
					'error'
				);
				return false;
			}
			if (
				sqlServerUsesClientId &&
				sqlServerAuthMode !== 'entra-managed-identity' &&
				!sqlServerEntraClientId.trim()
			) {
				showMessage('Enter the Microsoft Entra application client ID.', 'error');
				return false;
			}
			if (sqlServerUsesTenantId && !sqlServerEntraTenantId.trim()) {
				showMessage('Enter the Microsoft Entra tenant ID.', 'error');
				return false;
			}
			if (sqlServerAuthMode.startsWith('entra-') && sslMode === 'disable') {
				showMessage('Microsoft Entra authentication requires encrypted SQL Server TLS.', 'error');
				return false;
			}
		}
		if (
			provider === 'oracle' &&
			oracleWalletPasswordRequired &&
			!oracleWalletPassword &&
			!(editingId && hasStoredOracleWalletPassword)
		) {
			showMessage('Enter the password for the selected Oracle Wallet.', 'error');
			return false;
		}
		if (provider !== 'sqlite' && sshEnabled) {
			if (!sshHost.trim() || !sshUser.trim()) {
				showMessage('SSH host and username are required when the tunnel is enabled.', 'error');
				sshExpanded = true;
				return false;
			}
			if (sshAuthMode === 'password' && !sshPassword && !(editingId && hasStoredSSHPassword)) {
				showMessage('Enter the SSH password or choose another authentication method.', 'error');
				sshExpanded = true;
				return false;
			}
			if (sshAuthMode === 'private-key' && !sshPrivateKeyPath.trim()) {
				showMessage('Choose an SSH private key for private-key authentication.', 'error');
				sshExpanded = true;
				return false;
			}
		}
		return true;
	}

	function selectProvider(nextProvider: (typeof providers)[number]) {
		if (!nextProvider.available) return;

		const currentProvider = providers.find((item) => item.id === provider);
		if (!port || port === currentProvider?.defaultPort) {
			port = nextProvider.defaultPort;
		}
		provider = nextProvider.id;
		if (nextProvider.id !== 'oracle') {
			oracleConnectionMode = CONNECTION_DEFAULTS.oracleConnectionMode;
			oracleTnsConfigPath = '';
			oracleTnsAlias = '';
			oracleTnsAliases = [];
			oracleWalletPath = '';
			oracleWalletHasAutoLogin = false;
			oracleWalletPasswordRequired = false;
			oracleWalletPassword = '';
			hasStoredOracleWalletPassword = false;
			showOracleWalletPassword = false;
			clearOracleWalletPasswordConfirm = false;
		}
		if (nextProvider.id !== 'sqlserver') {
			sqlServerAuthMode = CONNECTION_DEFAULTS.sqlServerAuthMode;
			loadedSQLServerAuthMode = CONNECTION_DEFAULTS.sqlServerAuthMode;
			sqlServerEntraClientId = '';
			sqlServerEntraTenantId = '';
		} else if (currentProvider?.id !== 'sqlserver') {
			sqlServerAuthMode = CONNECTION_DEFAULTS.sqlServerAuthMode;
			loadedSQLServerAuthMode = CONNECTION_DEFAULTS.sqlServerAuthMode;
		}
		if (nextProvider.id === 'sqlite') {
			host = '';
			port = '';
			username = '';
			password = '';
			sslMode = CONNECTION_DEFAULTS.sslMode;
			sshEnabled = false;
			sshExpanded = false;
		} else {
			host ||= CONNECTION_DEFAULTS.host;
			if (!nextProvider.supportsClientCertificates) {
				sslCert = '';
				sslKey = '';
			}
		}
		message = '';
	}

	function selectEnvironment(value: string) {
		connectionEnvironment = normalizeConnectionEnvironment(value);
		if (!accessModeTouched) {
			accessMode = connectionEnvironment === 'production' ? 'read-only' : 'read-write';
		}
	}

	function selectSQLServerAuthMode(value: string) {
		const nextMode = value as SQLServerAuthMode;
		sqlServerAuthMode = nextMode;
		password = '';
		clearPasswordConfirm = false;
		if (nextMode !== 'sql' && nextMode !== 'entra-password') username = '';
		if (
			nextMode !== 'entra-password' &&
			nextMode !== 'entra-service-principal' &&
			nextMode !== 'entra-managed-identity'
		) {
			sqlServerEntraClientId = '';
		}
		if (nextMode !== 'entra-service-principal') sqlServerEntraTenantId = '';
		if (nextMode.startsWith('entra-') && sslMode === 'disable') {
			sslMode = 'verify-full';
		}
		if (nextMode === 'integrated') {
			sshEnabled = false;
			sshExpanded = false;
		}
		message = '';
	}

	function profileEndpoint(profile: db.SavedConnection): string {
		const config = profile.config;
		if ((config.driver || CONNECTION_DEFAULTS.provider) === 'sqlite') return config.db;
		if (
			(config.driver || CONNECTION_DEFAULTS.provider) === 'oracle' &&
			config.oracleConnectionMode === 'tns'
		) {
			const file = config.oracleTnsConfigPath?.split(/[\\/]/).pop() || 'tnsnames.ora';
			return `${config.oracleTnsAlias || config.db} · ${file}`;
		}
		return `${config.host}:${config.port}/${config.db}`;
	}

	async function chooseSQLiteFile(create: boolean) {
		if (action !== null) return;
		try {
			const response = await ChooseSQLiteDatabaseFile(create);
			if (response.errors?.length) {
				throw createServiceError(
					response.errors[0],
					create ? 'Could not choose a new SQLite file' : 'Could not open SQLite file'
				);
			}
			if (response.data) {
				databaseName = response.data;
				if (!connectionName.trim()) {
					const leaf = response.data.split(/[\\/]/).pop() || 'SQLite';
					connectionName = leaf.replace(/\.(sqlite3?|db)$/i, '') || 'SQLite';
				}
				showMessage(
					create
						? 'The database file will be created when you connect.'
						: 'SQLite database selected.',
					'info'
				);
			}
		} catch (error: any) {
			showMessage(error?.message || 'Could not choose SQLite database', 'error');
		}
	}

	async function chooseOracleTNSFile() {
		if (action !== null) return;
		try {
			const response = await ChooseOracleTNSFile();
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not choose tnsnames.ora');
			}
			if (!response.data?.path) return;
			oracleTnsConfigPath = response.data.path;
			oracleTnsAliases = response.data.aliases || [];
			if (!oracleTnsAliases.includes(oracleTnsAlias)) {
				oracleTnsAlias = oracleTnsAliases[0] || '';
			}
			sshEnabled = false;
			sshExpanded = false;
			if (!connectionName.trim() && oracleTnsAlias) connectionName = oracleTnsAlias;
			showMessage(
				`Loaded ${oracleTnsAliases.length} TNS ${oracleTnsAliases.length === 1 ? 'alias' : 'aliases'}.`,
				'info'
			);
		} catch (error: any) {
			showMessage(error?.message || 'Could not choose tnsnames.ora', 'error');
		}
	}

	async function chooseOracleWallet() {
		if (action !== null) return;
		try {
			const response = await ChooseOracleWalletDirectory();
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not choose Oracle Wallet');
			}
			if (!response.data?.path) return;
			const changed = response.data.path !== oracleWalletPath;
			oracleWalletPath = response.data.path;
			oracleWalletHasAutoLogin = Boolean(response.data.hasAutoLogin);
			oracleWalletPasswordRequired = Boolean(response.data.passwordRequired);
			if (changed) {
				oracleWalletPassword = '';
				hasStoredOracleWalletPassword = false;
			}
			if (sslMode === 'disable' || sslMode === 'verify-ca') sslMode = 'verify-full';
			sslRootCert = '';
			sslCert = '';
			sslKey = '';
			sshEnabled = false;
			sshExpanded = false;
			showMessage(
				oracleWalletHasAutoLogin
					? 'Oracle Wallet selected. Auto-login credentials are available.'
					: 'Oracle Wallet selected. Enter its password before connecting.',
				'info'
			);
		} catch (error: any) {
			showMessage(error?.message || 'Could not choose Oracle Wallet', 'error');
		}
	}

	function removeOracleWallet() {
		oracleWalletPath = '';
		oracleWalletHasAutoLogin = false;
		oracleWalletPasswordRequired = false;
		oracleWalletPassword = '';
		hasStoredOracleWalletPassword = false;
		showOracleWalletPassword = false;
		clearOracleWalletPasswordConfirm = false;
	}

	async function saveProfile() {
		if (!validate()) return;
		action = 'save';
		showMessage(editingId ? 'Updating connection profile…' : 'Saving new connection profile…');

		try {
			const response = editingId
				? await UpdateConnection(editingId, buildConfig())
				: await SaveConnection(buildConfig());
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not save connection profile');
			}

			const savedId = response.data?.id || editingId;
			await loadProfiles(savedId);
			showMessage('Connection profile saved.', 'success');
			updateStatus(`Saved connection profile “${connectionName}”`, 'success');
		} catch (error: any) {
			showMessage(error?.message || 'Could not save connection profile', 'error');
		} finally {
			action = null;
		}
	}

	async function connectProfile() {
		if (!validate(false)) return;
		const attemptID = createConnectionAttemptID();
		action = 'connect';
		connectionAttemptID = attemptID;
		connectionElapsedSeconds = 0;
		cancellingConnection = false;
		stopConnectionElapsedTimer?.();
		stopConnectionElapsedTimer = startConnectionElapsedTimer((seconds) => {
			connectionElapsedSeconds = seconds;
		});
		showMessage(
			`Connecting to ${endpoint}. Automatic timeout after ${CONNECTION_TIMEOUT_SECONDS} seconds.`
		);

		try {
			const config = buildConfig();
			const response = editingId
				? await ConnectWithProfile(editingId, config, attemptID)
				: await Connect(
						new db.ConnectRequest({
							driver: provider || CONNECTION_DEFAULTS.provider,
							config,
							attemptId: attemptID
						})
					);
			if (response.errors?.length || !response.data?.connected) {
				throw createServiceError(response.errors?.[0], 'Connection failed');
			}

			await connectionStore.refreshConnections();
			window.dispatchEvent(new CustomEvent('connection-switched'));
			updateStatus(`Connected to ${connectionName || databaseName}`, 'success');
			onConnected();
			onClose();
		} catch (error: any) {
			const detail = error?.message || 'Could not connect to the database';
			showMessage(
				detail,
				error?.code === 'CONNECTION_CANCELLED' || detail.toLowerCase().includes('cancelled')
					? 'info'
					: 'error'
			);
		} finally {
			if (connectionAttemptID === attemptID) {
				connectionAttemptID = null;
				stopConnectionElapsedTimer?.();
				stopConnectionElapsedTimer = null;
			}
			cancellingConnection = false;
			action = null;
		}
	}

	async function cancelConnection() {
		if (!connectionAttemptID || cancellingConnection) return;
		const attemptID = connectionAttemptID;
		cancellingConnection = true;
		showMessage(`Cancelling connection to ${endpoint}…`);

		try {
			await cancelConnectionAttempt(attemptID);
		} catch (error: any) {
			cancellingConnection = false;
			showMessage(error?.message || 'Could not cancel connection attempt', 'error');
		}
	}

	async function clearStoredPassword() {
		if (!editingId || !hasStoredPassword || action !== null) return;
		if (!clearPasswordConfirm) {
			clearPasswordConfirm = true;
			showMessage('Press “Remove stored password” again to confirm.', 'info');
			return;
		}
		action = 'save';
		try {
			const response = await ClearConnectionPassword(editingId);
			if (response.errors?.length || !response.data) {
				throw createServiceError(response.errors?.[0], 'Could not remove stored password');
			}
			hasStoredPassword = false;
			password = '';
			clearPasswordConfirm = false;
			await loadProfiles(editingId);
			showMessage('Stored password removed from the operating system credential store.', 'success');
		} catch (error: any) {
			showMessage(error?.message || 'Could not remove stored password', 'error');
		} finally {
			action = null;
		}
	}

	async function clearStoredSSHCredentials() {
		if (!editingId || (!hasStoredSSHPassword && !hasStoredSSHKeyPassphrase) || action !== null)
			return;
		if (!clearSSHCredentialsConfirm) {
			clearSSHCredentialsConfirm = true;
			showMessage('Press “Remove stored SSH secret” again to confirm.', 'info');
			return;
		}
		action = 'save';
		try {
			const response = await ClearConnectionSSHCredentials(editingId);
			if (response.errors?.length || !response.data) {
				throw createServiceError(response.errors?.[0], 'Could not remove stored SSH credentials');
			}
			hasStoredSSHPassword = false;
			hasStoredSSHKeyPassphrase = false;
			sshPassword = '';
			sshKeyPassphrase = '';
			clearSSHCredentialsConfirm = false;
			await loadProfiles(editingId);
			showMessage(
				'Stored SSH secret removed from the operating system credential store.',
				'success'
			);
		} catch (error: any) {
			showMessage(error?.message || 'Could not remove stored SSH credentials', 'error');
		} finally {
			action = null;
		}
	}

	async function clearStoredOracleWalletPassword() {
		if (!editingId || !hasStoredOracleWalletPassword || action !== null) return;
		if (!clearOracleWalletPasswordConfirm) {
			clearOracleWalletPasswordConfirm = true;
			showMessage('Press “Remove Wallet password” again to confirm.', 'info');
			return;
		}
		action = 'save';
		try {
			const response = await ClearConnectionOracleWalletPassword(editingId);
			if (response.errors?.length || !response.data) {
				throw createServiceError(
					response.errors?.[0],
					'Could not remove the Oracle Wallet password'
				);
			}
			hasStoredOracleWalletPassword = false;
			oracleWalletPassword = '';
			clearOracleWalletPasswordConfirm = false;
			profiles = profiles.map((profile) =>
				profile.id === editingId
					? new db.SavedConnection({
							...profile,
							hasOracleWalletPassword: false
						})
					: profile
			);
			showMessage(
				'Oracle Wallet password removed from the operating system credential store.',
				'success'
			);
		} catch (error: any) {
			showMessage(error?.message || 'Could not remove the Oracle Wallet password', 'error');
		} finally {
			action = null;
		}
	}

	async function deleteProfile() {
		if (!editingId) return;
		if (!deleteConfirm) {
			deleteConfirm = true;
			return;
		}

		action = 'delete';
		try {
			const response = await DeleteConnection(editingId);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not delete connection profile');
			}
			const deletedName = connectionName;
			newProfile();
			await loadProfiles();
			showMessage(`Deleted “${deletedName}”.`, 'success');
		} catch (error: any) {
			showMessage(error?.message || 'Could not delete connection profile', 'error');
		} finally {
			action = null;
			deleteConfirm = false;
		}
	}
</script>

{#if open}
	<div class="fixed inset-0 z-[100] flex items-center justify-center p-6">
		<button
			type="button"
			class="bg-overlay/45 absolute inset-0 cursor-default backdrop-blur-[2px]"
			aria-label="Close connection manager"
			onclick={closeModal}
		></button>

		<div
			use:focusTrap
			class="rt-popover relative flex h-[min(640px,calc(100vh-48px))] w-[min(960px,calc(100vw-48px))] flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="connection-manager-title"
		>
			<header class="flex h-14 shrink-0 items-center justify-between border-b px-4">
				<div class="flex min-w-0 items-center gap-2.5">
					<img src="/logo.png" alt="" class="rt-brand-logo h-8 w-8 rounded-lg" />
					<div class="min-w-0">
						<h2 id="connection-manager-title" class="text-[13px] font-bold">Manage connections</h2>
						<p class="text-muted-foreground mt-0.5 text-[9px]">
							Add, edit, or connect a database profile in one place.
						</p>
					</div>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-8 w-8"
					onclick={closeModal}
					disabled={action !== null}
					aria-label="Close connection manager"
				>
					<X class="h-4 w-4" />
				</button>
			</header>

			<div class="grid min-h-0 flex-1 grid-cols-[286px_minmax(0,1fr)]">
				<aside class="flex min-h-0 flex-col border-r bg-[var(--surface-sunken)]">
					<div class="border-b p-3">
						<div class="relative">
							<Search
								class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2"
							/>
							<input
								type="search"
								class="rt-input h-8 w-full pr-3 pl-8 text-[10px]"
								placeholder="Search profiles"
								bind:value={searchQuery}
							/>
						</div>
						<button
							type="button"
							class="mt-2 flex h-8 w-full items-center justify-center gap-2 rounded-md border bg-[var(--surface-raised)] text-[10px] font-bold hover:bg-[var(--surface-hover)]"
							onclick={newProfile}
						>
							<Plus class="h-3.5 w-3.5" />
							New connection
						</button>
					</div>

					<div class="min-h-0 flex-1 overflow-auto p-2">
						<div
							class="text-muted-foreground flex items-center justify-between px-2 py-1.5 text-[8px] font-bold tracking-[0.12em] uppercase"
						>
							<span>Saved profiles</span>
							<span>{filteredProfiles.length}</span>
						</div>

						{#if loadingProfiles}
							<div
								class="text-muted-foreground flex items-center justify-center gap-2 py-10 text-[10px]"
							>
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
								Loading profiles
							</div>
						{:else if filteredProfiles.length === 0}
							<div class="text-muted-foreground px-5 py-10 text-center">
								<Database class="mx-auto h-5 w-5 opacity-50" />
								<p class="mt-2 text-[10px] font-semibold">
									{searchQuery ? 'No matching profiles' : 'No saved profiles'}
								</p>
							</div>
						{:else}
							<div class="space-y-2">
								{#each groupedProfiles as group (group.name)}
									<div>
										<div
											class="text-muted-foreground flex items-center gap-2 px-2 py-1 text-[7px] font-bold tracking-[0.1em] uppercase"
										>
											<FolderOpen class="h-3 w-3" />
											<span class="min-w-0 flex-1 truncate">{group.name}</span>
											<span>{group.items.length}</span>
										</div>
										<div class="space-y-0.5">
											{#each group.items as profile (profile.id)}
												{@const profileProvider =
													providers.find(
														(item) =>
															item.id === (profile.config.driver || CONNECTION_DEFAULTS.provider)
													) ?? providers[0]}
												{@const profileEnvironment = connectionEnvironmentOption(
													profile.config.environment
												)}
												<button
													type="button"
													class="group relative flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left transition-colors {editingId ===
													profile.id
														? 'text-foreground bg-[var(--surface-raised)] shadow-sm'
														: 'text-muted-foreground hover:text-foreground hover:bg-[var(--surface-hover)]'}"
													onclick={() => selectProfile(profile)}
												>
													{#if editingId === profile.id}
														<span
															class="bg-foreground absolute top-2 bottom-2 left-0 w-0.5 rounded-r"
														></span>
													{/if}
													<span
														class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border font-mono text-[8px] font-bold"
													>
														{profileProvider.mark}
													</span>
													<span class="min-w-0 flex-1">
														<span class="block truncate text-[10px] font-bold">
															{profile.config.name || 'Unnamed profile'}
														</span>
														<span class="mt-0.5 block truncate font-mono text-[8px]">
															{profile.config.tags?.[0]
																? `${profile.config.tags[0]} · ${profileEndpoint(profile)}`
																: profileEndpoint(profile)}
														</span>
													</span>
													<span
														class="h-1.5 w-1.5 shrink-0 rounded-full {profileEnvironment.dotClass}"
														title={profileEnvironment.label}
													></span>
													{#if isConnected(profile)}
														<span
															class="bg-success h-1.5 w-1.5 shrink-0 rounded-full"
															title="Connected"
														></span>
													{/if}
												</button>
											{/each}
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</aside>

				<form
					class="rt-connection-form flex min-h-0 min-w-0 flex-col bg-[var(--surface-raised)]"
					onsubmit={(event) => {
						event.preventDefault();
						void connectProfile();
					}}
				>
					<div class="flex h-[66px] shrink-0 items-center justify-between border-b px-5">
						<div class="flex min-w-0 items-center gap-3">
							{#if selectedProvider}
								<button
									type="button"
									class="rt-toolbar-button h-8 shrink-0 gap-1.5 px-2.5 text-[9px] font-semibold"
									onclick={() => {
										provider = '';
										message = '';
									}}
									disabled={action !== null}
									aria-label="Back to database providers"
								>
									<ArrowLeft class="h-3 w-3" />
									Back
								</button>
								<span class="h-8 border-l"></span>
							{/if}
							<div class="min-w-0">
								<p class="text-muted-foreground text-[8px] font-bold tracking-[0.12em] uppercase">
									{selectedProvider
										? editingId
											? 'Saved profile'
											: 'New profile'
										: 'New connection'}
								</p>
								<h3 class="mt-1 truncate text-[13px] font-bold">
									{selectedProvider
										? connectionName || 'Untitled connection'
										: 'Choose a database provider'}
								</h3>
							</div>
						</div>
						{#if selectedProvider}
							<div class="text-right">
								<p class="text-muted-foreground font-mono text-[8px]">{endpoint}</p>
								<p class="mt-1 text-[8px] font-semibold">{selectedProvider.name}</p>
							</div>
						{:else}
							<span class="text-muted-foreground text-[9px] font-semibold">Step 1 of 2</span>
						{/if}
					</div>

					<div class="min-h-0 flex-1 overflow-auto p-5">
						{#if message}
							<div
								class="mb-4 flex items-center gap-2 rounded-md border px-3 py-2 text-[9px] font-semibold {messageLevel ===
								'error'
									? 'border-danger-border bg-danger-soft text-danger'
									: messageLevel === 'success'
										? 'border-success-border bg-success-soft text-success'
										: 'text-muted-foreground bg-[var(--surface-sunken)]'}"
							>
								{#if action}
									<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
								{:else if messageLevel === 'success'}
									<Check class="h-3.5 w-3.5 shrink-0" />
								{:else}
									<AlertCircle class="h-3.5 w-3.5 shrink-0" />
								{/if}
								<span class="min-w-0 flex-1">{message}</span>
								{#if action === 'connect'}
									<span class="shrink-0 font-mono text-[8px] tabular-nums">
										{connectionElapsedSeconds}s / {CONNECTION_TIMEOUT_SECONDS}s
									</span>
								{/if}
							</div>
						{/if}

						{#if !selectedProvider}
							<div class="mx-auto max-w-[520px] py-2">
								<p class="text-muted-foreground text-[9px] font-bold tracking-[0.12em] uppercase">
									Database engine
								</p>
								<h4 class="mt-2 text-base font-bold tracking-[-0.02em]">Choose your provider</h4>
								<p class="text-muted-foreground mt-1 text-[10px] leading-relaxed">
									The connection form will adapt to the selected database engine.
								</p>

								<div class="mt-5 space-y-2">
									{#each providers as item (item.id)}
										<button
											type="button"
											class="group flex w-full items-center gap-3 rounded-lg border px-3.5 py-3 text-left transition-colors {item.available
												? 'cursor-pointer bg-[var(--surface-raised)] hover:border-[var(--brand-border)] hover:bg-[var(--brand-soft)]'
												: 'cursor-not-allowed bg-[var(--surface-sunken)] opacity-55'}"
											onclick={() => selectProvider(item)}
											disabled={!item.available}
										>
											<span
												class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-sunken)] font-mono text-[10px] font-bold"
											>
												{item.mark}
											</span>
											<span class="min-w-0 flex-1">
												<span class="block text-[11px] font-bold">{item.name}</span>
												<span class="text-muted-foreground mt-0.5 block text-[9px]">
													{item.description}
												</span>
											</span>
											{#if item.available}
												<span class="text-primary text-[8px] font-bold tracking-wide uppercase">
													Available
												</span>
												<ChevronRight class="text-muted-foreground h-3.5 w-3.5" />
											{:else}
												<span
													class="text-muted-foreground rounded-full border px-2 py-1 text-[8px] font-semibold"
												>
													Coming soon
												</span>
											{/if}
										</button>
									{/each}
								</div>
							</div>
						{:else}
							<div class="grid grid-cols-2 gap-x-4 gap-y-4">
								<div class="col-span-2 grid grid-cols-[minmax(0,1fr)_220px] gap-4">
									<div>
										<label for="modal-connection-name">Profile name</label>
										<input
											id="modal-connection-name"
											bind:value={connectionName}
											placeholder="Analytics workspace"
											disabled={action !== null}
										/>
									</div>
									<div>
										<label for="modal-environment">Environment</label>
										<FilterCombobox
											id="modal-environment"
											options={environmentOptions}
											value={connectionEnvironment}
											onChange={selectEnvironment}
											searchable={false}
											disabled={action !== null}
											triggerClass="h-9 px-3 text-xs"
										/>
										<p class="text-muted-foreground mt-1 flex items-center gap-1.5 text-[7px]">
											<span class="h-1.5 w-1.5 rounded-full {selectedEnvironment.dotClass}"></span>
											{selectedEnvironment.description}
										</p>
									</div>
								</div>
								<div class="col-span-2 grid grid-cols-2 gap-4">
									<div>
										<label for="modal-folder">Folder</label>
										<input
											id="modal-folder"
											bind:value={folder}
											placeholder="Team or project (optional)"
											disabled={action !== null}
										/>
									</div>
									<div>
										<label for="modal-tags">Tags</label>
										<input
											id="modal-tags"
											bind:value={tagsText}
											placeholder="billing, analytics"
											disabled={action !== null}
										/>
									</div>
								</div>
								<div
									class="col-span-2 grid grid-cols-[minmax(0,1fr)_220px] items-end gap-4 rounded-lg border bg-[var(--surface-sunken)] p-3"
								>
									<div>
										<div class="flex items-center gap-2 text-[10px] font-bold">
											<ShieldCheck class="h-3.5 w-3.5" />
											Write protection
										</div>
										<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
											{CONNECTION_ACCESS_MODES.find((option) => option.value === accessMode)
												?.description}
											Temporary unlocks reset when the connection closes.
										</p>
									</div>
									<div>
										<label for="modal-access-mode">Access mode</label>
										<FilterCombobox
											id="modal-access-mode"
											options={accessModeOptions}
											value={accessMode}
											onChange={(value) => {
												accessMode = normalizeConnectionAccessMode(value, connectionEnvironment);
												accessModeTouched = true;
											}}
											searchable={false}
											disabled={action !== null}
											triggerClass="h-9 px-3 text-xs"
										/>
									</div>
								</div>

								{#if provider === 'sqlite'}
									<div class="col-span-2">
										<div class="mt-1 flex items-center gap-2 border-b pb-2">
											<Database class="text-muted-foreground h-3.5 w-3.5" />
											<span class="text-[10px] font-bold">Local database file</span>
										</div>
									</div>
									<div class="col-span-2">
										<label for="modal-database">SQLite file path</label>
										<input
											id="modal-database"
											bind:value={databaseName}
											placeholder="/path/to/database.sqlite3"
											disabled={action !== null}
										/>
									</div>
									<div class="col-span-2 grid grid-cols-2 gap-3">
										<button
											type="button"
											class="rt-toolbar-button h-9 gap-2 px-3 text-[10px] font-bold"
											onclick={() => void chooseSQLiteFile(false)}
											disabled={action !== null}
										>
											<FolderOpen class="h-3.5 w-3.5" />
											Open existing file
										</button>
										<button
											type="button"
											class="rt-toolbar-button h-9 gap-2 px-3 text-[10px] font-bold"
											onclick={() => void chooseSQLiteFile(true)}
											disabled={action !== null}
										>
											<FilePlus2 class="h-3.5 w-3.5" />
											Create new file
										</button>
									</div>
									<div class="col-span-2 rounded-lg border bg-[var(--surface-sunken)] px-3.5 py-3">
										<div class="text-[9px] font-bold">Safe local defaults</div>
										<div class="text-muted-foreground mt-2 grid gap-1.5 text-[8px]">
											<span>• Foreign-key enforcement is enabled for every session.</span>
											<span>• WAL mode improves reader/writer concurrency.</span>
											<span
												>• Locked files wait up to five seconds and return an actionable error.</span
											>
										</div>
									</div>
								{:else}
									<div class="col-span-2 mt-1 flex items-center gap-2 border-b pb-2">
										<Server class="text-muted-foreground h-3.5 w-3.5" />
										<span class="text-[10px] font-bold">{selectedProvider.name} connection</span>
									</div>
									{#if provider === 'oracle'}
										<div class="col-span-2">
											<label for="modal-oracle-connection-mode">Connection method</label>
											<FilterCombobox
												id="modal-oracle-connection-mode"
												options={oracleConnectionModeOptions}
												value={oracleConnectionMode}
												onChange={(value) => {
													oracleConnectionMode = value as OracleConnectionMode;
													if (value === 'tns') {
														sshEnabled = false;
														sshExpanded = false;
													}
												}}
												searchable={false}
												disabled={action !== null}
												triggerClass="h-9 px-3 text-xs"
											/>
										</div>
									{/if}
									{#if oracleUsesTNS}
										<div class="col-span-2">
											<label for="modal-oracle-tns-path">tnsnames.ora</label>
											<div class="flex gap-2">
												<input
													id="modal-oracle-tns-path"
													value={oracleTnsConfigPath}
													placeholder="Choose an Oracle Net configuration file"
													readonly
													disabled={action !== null}
												/>
												<button
													type="button"
													class="rt-toolbar-button h-9 shrink-0 gap-1.5 px-3 text-[9px] font-bold"
													onclick={() => void chooseOracleTNSFile()}
													disabled={action !== null}
												>
													<FolderOpen class="h-3.5 w-3.5" />
													Choose file
												</button>
											</div>
										</div>
										<div class="col-span-2">
											<label for="modal-oracle-tns-alias">TNS alias</label>
											<FilterCombobox
												id="modal-oracle-tns-alias"
												options={oracleTnsAliasOptions}
												value={oracleTnsAlias}
												onChange={(value) => (oracleTnsAlias = value)}
												searchable={oracleTnsAliasOptions.length > 6}
												disabled={action !== null || oracleTnsAliasOptions.length === 0}
												triggerClass="h-9 px-3 text-xs"
												placeholder="Choose a TNS alias"
											/>
											<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
												The reviewed descriptor supplies host, port, protocol, and service name.
												Included files are not followed automatically.
											</p>
										</div>
									{:else}
										<div>
											<label for="modal-host">Host</label>
											<input
												id="modal-host"
												bind:value={host}
												placeholder={CONNECTION_DEFAULTS.host}
												disabled={action !== null}
											/>
										</div>
										<div class="grid grid-cols-[112px_minmax(0,1fr)] gap-3">
											<div>
												<label for="modal-port">Port</label>
												<input
													id="modal-port"
													bind:value={port}
													placeholder={selectedProvider.defaultPort}
													disabled={action !== null}
												/>
											</div>
											<div>
												<label for="modal-database">{selectedProvider.databaseLabel}</label>
												<input
													id="modal-database"
													bind:value={databaseName}
													placeholder={selectedProvider.defaultDatabase}
													disabled={action !== null}
												/>
											</div>
										</div>
									{/if}

									<div class="col-span-2 mt-1 flex items-center gap-2 border-b pb-2">
										<Lock class="text-muted-foreground h-3.5 w-3.5" />
										<span class="text-[10px] font-bold">Authentication & TLS</span>
									</div>
									{#if provider === 'sqlserver'}
										<div class="col-span-2">
											<label for="modal-sqlserver-auth">Authentication method</label>
											<FilterCombobox
												id="modal-sqlserver-auth"
												options={sqlServerAuthModeOptions}
												value={sqlServerAuthMode}
												onChange={selectSQLServerAuthMode}
												searchable={false}
												disabled={action !== null}
												triggerClass="h-9 px-3 text-xs"
											/>
											<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
												{selectedSQLServerAuth.description}
												{#if sqlServerAuthMode === 'integrated'}
													This mode is available in the Windows desktop build.
												{/if}
												{#if hasStoredPassword && !storedPasswordMatchesAuth}
													The previous authentication secret will be removed when this profile is
													saved.
												{/if}
											</p>
										</div>
									{/if}
									{#if sqlServerUsesUsername}
										<div class={sqlServerUsesPassword ? '' : 'col-span-2'}>
											<label for="modal-username">
												{provider === 'sqlserver' && sqlServerAuthMode === 'entra-password'
													? 'Microsoft Entra username'
													: 'Username'}
											</label>
											<input
												id="modal-username"
												bind:value={username}
												placeholder={provider === 'sqlserver' &&
												sqlServerAuthMode === 'entra-password'
													? 'user@example.com'
													: selectedProvider.defaultUser}
												disabled={action !== null}
											/>
										</div>
									{/if}
									{#if sqlServerUsesPassword}
										<div class={!sqlServerUsesUsername ? 'col-span-2' : ''}>
											<div class="mb-1.5 flex items-center justify-between gap-2">
												<label for="modal-password" class="!mb-0">
													{provider === 'sqlserver' &&
													sqlServerAuthMode === 'entra-service-principal'
														? 'Client secret'
														: 'Password'}
												</label>
												{#if hasStoredPassword && storedPasswordMatchesAuth}
													<button
														type="button"
														class="text-muted-foreground hover:text-destructive cursor-pointer text-[8px] font-semibold"
														onclick={clearStoredPassword}
														disabled={action !== null}
													>
														{clearPasswordConfirm
															? 'Remove stored password'
															: 'Stored securely · remove'}
													</button>
												{/if}
											</div>
											<div class="relative">
												<input
													id="modal-password"
													type={showPassword ? 'text' : 'password'}
													bind:value={password}
													placeholder={storedPasswordMatchesAuth
														? 'Stored by the operating system - leave blank to keep'
														: provider === 'sqlserver' &&
															  sqlServerAuthMode === 'entra-service-principal'
															? 'Enter client secret'
															: 'Enter password'}
													class="!pr-10"
													disabled={action !== null}
												/>
												<button
													type="button"
													class="rt-toolbar-button absolute top-1/2 right-1.5 h-7 w-7 -translate-y-1/2"
													onclick={() => (showPassword = !showPassword)}
													aria-label={showPassword ? 'Hide password' : 'Show password'}
												>
													{#if showPassword}
														<EyeOff class="h-3.5 w-3.5" />
													{:else}
														<Eye class="h-3.5 w-3.5" />
													{/if}
												</button>
											</div>
										</div>
									{/if}
									{#if sqlServerUsesClientId}
										<div class={sqlServerUsesTenantId ? '' : 'col-span-2'}>
											<label for="modal-sqlserver-client-id">
												{sqlServerAuthMode === 'entra-managed-identity'
													? 'User-assigned client ID (optional)'
													: 'Application client ID'}
											</label>
											<input
												id="modal-sqlserver-client-id"
												bind:value={sqlServerEntraClientId}
												placeholder={sqlServerAuthMode === 'entra-managed-identity'
													? 'Empty uses system-assigned identity'
													: '00000000-0000-0000-0000-000000000000'}
												disabled={action !== null}
											/>
										</div>
									{/if}
									{#if sqlServerUsesTenantId}
										<div>
											<label for="modal-sqlserver-tenant-id">Tenant ID</label>
											<input
												id="modal-sqlserver-tenant-id"
												bind:value={sqlServerEntraTenantId}
												placeholder="00000000-0000-0000-0000-000000000000"
												disabled={action !== null}
											/>
										</div>
									{/if}
									<div class="col-span-2">
										<label for="modal-ssl">TLS mode</label>
										<FilterCombobox
											id="modal-ssl"
											options={sslOptions}
											value={sslMode}
											onChange={(value) => (sslMode = value)}
											searchable={false}
											disabled={action !== null}
											triggerClass="h-9 px-3 text-xs"
											placeholder="Select TLS mode"
										/>
									</div>

									{#if provider === 'oracle'}
										<div
											class="col-span-2 overflow-hidden rounded-lg border bg-[var(--surface-sunken)]"
										>
											<div class="flex items-center gap-3 px-3.5 py-3">
												<span
													class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-[var(--surface-raised)]"
												>
													<ShieldCheck class="text-muted-foreground h-3.5 w-3.5" />
												</span>
												<div class="min-w-0 flex-1">
													<p class="text-[10px] font-bold">Oracle Wallet</p>
													<p class="text-muted-foreground mt-0.5 text-[8px] leading-relaxed">
														Optional TCPS trust material from
														<code>ewallet.p12</code> or <code>cwallet.sso</code>.
													</p>
												</div>
												<button
													type="button"
													class="rt-toolbar-button h-8 shrink-0 gap-1.5 px-2.5 text-[9px] font-bold"
													onclick={() => void chooseOracleWallet()}
													disabled={action !== null}
												>
													<FolderOpen class="h-3.5 w-3.5" />
													{oracleWalletPath ? 'Change' : 'Choose directory'}
												</button>
											</div>
											{#if oracleWalletPath}
												<div
													class="grid grid-cols-2 gap-3 border-t bg-[var(--surface-raised)] p-3.5"
												>
													<div class="col-span-2">
														<div class="mb-1.5 flex items-center justify-between gap-2">
															<label for="modal-oracle-wallet-path" class="!mb-0">
																Wallet directory
															</label>
															<button
																type="button"
																class="text-muted-foreground hover:text-destructive text-[8px] font-semibold"
																onclick={removeOracleWallet}
																disabled={action !== null}
															>
																Remove
															</button>
														</div>
														<input
															id="modal-oracle-wallet-path"
															value={oracleWalletPath}
															readonly
															disabled={action !== null}
														/>
														<p class="text-muted-foreground mt-1 text-[8px]">
															{oracleWalletHasAutoLogin
																? 'Auto-login wallet detected; a password is not required.'
																: 'Password-protected wallet detected.'}
														</p>
													</div>
													<div class="col-span-2">
														<div class="mb-1.5 flex items-center justify-between gap-2">
															<label for="modal-oracle-wallet-password" class="!mb-0">
																Wallet password
															</label>
															{#if hasStoredOracleWalletPassword}
																<button
																	type="button"
																	class="text-muted-foreground hover:text-destructive text-[8px] font-semibold"
																	onclick={clearStoredOracleWalletPassword}
																	disabled={action !== null}
																>
																	{clearOracleWalletPasswordConfirm
																		? 'Remove Wallet password'
																		: 'Stored securely · remove'}
																</button>
															{:else if oracleWalletHasAutoLogin}
																<span class="text-muted-foreground text-[8px] font-semibold">
																	Optional
																</span>
															{/if}
														</div>
														<div class="relative">
															<input
																id="modal-oracle-wallet-password"
																type={showOracleWalletPassword ? 'text' : 'password'}
																bind:value={oracleWalletPassword}
																placeholder={hasStoredOracleWalletPassword
																	? 'Stored by the operating system - leave blank to keep'
																	: oracleWalletPasswordRequired
																		? 'Enter Wallet password'
																		: 'Not required for auto-login'}
																class="!pr-10"
																disabled={action !== null}
															/>
															<button
																type="button"
																class="rt-toolbar-button absolute top-1/2 right-1.5 h-7 w-7 -translate-y-1/2"
																onclick={() =>
																	(showOracleWalletPassword = !showOracleWalletPassword)}
																aria-label={showOracleWalletPassword
																	? 'Hide Oracle Wallet password'
																	: 'Show Oracle Wallet password'}
															>
																{#if showOracleWalletPassword}
																	<EyeOff class="h-3.5 w-3.5" />
																{:else}
																	<Eye class="h-3.5 w-3.5" />
																{/if}
															</button>
														</div>
													</div>
													<p class="text-muted-foreground col-span-2 text-[8px] leading-relaxed">
														Wallet passwords are stored only in the operating system credential
														store. Selecting a Wallet disables separate certificate paths and SSH
														tunnelling.
													</p>
												</div>
											{/if}
										</div>
									{/if}

									{#if !oracleUsesWallet && (sslMode === 'verify-ca' || sslMode === 'verify-full')}
										<div class="col-span-2">
											<label for="modal-root-cert">CA certificate path</label>
											<input
												id="modal-root-cert"
												bind:value={sslRootCert}
												placeholder="/path/to/root.crt"
												disabled={action !== null}
											/>
										</div>
										{#if selectedProvider.supportsClientCertificates}
											<div>
												<label for="modal-client-cert">Client certificate path</label>
												<input
													id="modal-client-cert"
													bind:value={sslCert}
													placeholder="/path/to/client.crt"
													disabled={action !== null}
												/>
											</div>
											<div>
												<label for="modal-client-key">Client key path</label>
												<input
													id="modal-client-key"
													bind:value={sslKey}
													placeholder="/path/to/client.key"
													disabled={action !== null}
												/>
											</div>
										{/if}
									{/if}

									{#if (provider === 'oracle' && (oracleUsesTNS || oracleUsesWallet)) || sqlServerIntegrated}
										<div
											class="col-span-2 flex items-start gap-3 rounded-lg border bg-[var(--surface-sunken)] px-3.5 py-3"
										>
											<Network class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
											<div>
												<p class="text-[9px] font-bold">SSH tunnel unavailable</p>
												<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
													{sqlServerIntegrated
														? 'Windows Integrated authentication must preserve the reviewed SQL Server identity for SSPI and cannot be routed through an SSH endpoint.'
														: oracleUsesTNS
															? 'TNS descriptors control their own endpoints. Use Direct endpoint mode when Oracle must be reached through SSH.'
															: 'Wallet certificate verification must target the reviewed Oracle endpoint directly.'}
												</p>
											</div>
										</div>
									{:else}
										<div
											class="col-span-2 overflow-hidden rounded-lg border bg-[var(--surface-sunken)]"
										>
											<div class="flex min-h-12 items-center">
												<button
													type="button"
													class="flex min-w-0 flex-1 items-center gap-3 px-3.5 py-3 text-left"
													onclick={() => (sshExpanded = !sshExpanded)}
													disabled={action !== null}
													aria-expanded={sshExpanded}
												>
													<span
														class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-[var(--surface-raised)]"
													>
														<Network class="text-muted-foreground h-3.5 w-3.5" />
													</span>
													<span class="min-w-0 flex-1">
														<span class="block text-[10px] font-bold">SSH tunnel</span>
														<span class="text-muted-foreground mt-0.5 block text-[8px]">
															{sshEnabled
																? `Via ${sshHost || 'SSH host'}:${sshPort || CONNECTION_DEFAULTS.sshPort}`
																: 'Optional secure forwarding through a bastion host'}
														</span>
													</span>
													<ChevronDown
														class="text-muted-foreground h-3.5 w-3.5 transition-transform {sshExpanded
															? 'rotate-180'
															: ''}"
													/>
												</button>
												<button
													type="button"
													class="mr-3 inline-flex h-7 min-w-12 items-center justify-center rounded-full border px-2.5 text-[8px] font-bold transition-colors {sshEnabled
														? 'border-success-border bg-success-soft text-success'
														: 'text-muted-foreground bg-[var(--surface-raised)]'}"
													onclick={() => {
														sshEnabled = !sshEnabled;
														if (sshEnabled) sshExpanded = true;
													}}
													disabled={action !== null}
													aria-pressed={sshEnabled}
												>
													{sshEnabled ? 'On' : 'Off'}
												</button>
											</div>

											{#if sshExpanded}
												<div
													class="grid grid-cols-2 gap-3 border-t bg-[var(--surface-raised)] p-3.5"
												>
													<div>
														<label for="modal-ssh-host">SSH host</label>
														<input
															id="modal-ssh-host"
															bind:value={sshHost}
															placeholder="bastion.example.com"
															disabled={action !== null || !sshEnabled}
														/>
													</div>
													<div class="grid grid-cols-[96px_minmax(0,1fr)] gap-3">
														<div>
															<label for="modal-ssh-port">Port</label>
															<input
																id="modal-ssh-port"
																bind:value={sshPort}
																placeholder={CONNECTION_DEFAULTS.sshPort}
																disabled={action !== null || !sshEnabled}
															/>
														</div>
														<div>
															<label for="modal-ssh-user">Username</label>
															<input
																id="modal-ssh-user"
																bind:value={sshUser}
																placeholder="deploy"
																disabled={action !== null || !sshEnabled}
															/>
														</div>
													</div>

													<div class="col-span-2">
														<label for="modal-ssh-auth">Authentication</label>
														<FilterCombobox
															id="modal-ssh-auth"
															options={sshAuthOptions}
															value={sshAuthMode}
															onChange={(value) => {
																sshAuthMode = value;
																clearSSHCredentialsConfirm = false;
															}}
															searchable={false}
															disabled={action !== null || !sshEnabled}
															triggerClass="h-9 px-3 text-xs"
															placeholder="Select SSH authentication"
														/>
													</div>

													{#if sshAuthMode === 'private-key'}
														<div class="col-span-2">
															<label for="modal-ssh-key">Private key path</label>
															<input
																id="modal-ssh-key"
																bind:value={sshPrivateKeyPath}
																placeholder="~/.ssh/id_ed25519"
																disabled={action !== null || !sshEnabled}
															/>
														</div>
														<div class="col-span-2">
															<div class="mb-1.5 flex items-center justify-between gap-2">
																<label for="modal-ssh-passphrase" class="!mb-0">
																	Key passphrase
																</label>
																{#if hasStoredSSHKeyPassphrase}
																	<span class="text-muted-foreground text-[8px] font-semibold">
																		Stored securely
																	</span>
																{/if}
															</div>
															<div class="relative">
																<input
																	id="modal-ssh-passphrase"
																	type={showSSHSecret ? 'text' : 'password'}
																	bind:value={sshKeyPassphrase}
																	placeholder={hasStoredSSHKeyPassphrase
																		? 'Stored by the operating system - leave blank to keep'
																		: 'Optional for encrypted keys'}
																	class="!pr-10"
																	disabled={action !== null || !sshEnabled}
																/>
																<button
																	type="button"
																	class="rt-toolbar-button absolute top-1/2 right-1.5 h-7 w-7 -translate-y-1/2"
																	onclick={() => (showSSHSecret = !showSSHSecret)}
																	aria-label={showSSHSecret
																		? 'Hide SSH key passphrase'
																		: 'Show SSH key passphrase'}
																>
																	{#if showSSHSecret}
																		<EyeOff class="h-3.5 w-3.5" />
																	{:else}
																		<Eye class="h-3.5 w-3.5" />
																	{/if}
																</button>
															</div>
														</div>
													{:else if sshAuthMode === 'password'}
														<div class="col-span-2">
															<div class="mb-1.5 flex items-center justify-between gap-2">
																<label for="modal-ssh-password" class="!mb-0">SSH password</label>
																{#if hasStoredSSHPassword}
																	<span class="text-muted-foreground text-[8px] font-semibold">
																		Stored securely
																	</span>
																{/if}
															</div>
															<div class="relative">
																<input
																	id="modal-ssh-password"
																	type={showSSHSecret ? 'text' : 'password'}
																	bind:value={sshPassword}
																	placeholder={hasStoredSSHPassword
																		? 'Stored by the operating system - leave blank to keep'
																		: 'Enter SSH password'}
																	class="!pr-10"
																	disabled={action !== null || !sshEnabled}
																/>
																<button
																	type="button"
																	class="rt-toolbar-button absolute top-1/2 right-1.5 h-7 w-7 -translate-y-1/2"
																	onclick={() => (showSSHSecret = !showSSHSecret)}
																	aria-label={showSSHSecret
																		? 'Hide SSH password'
																		: 'Show SSH password'}
																>
																	{#if showSSHSecret}
																		<EyeOff class="h-3.5 w-3.5" />
																	{:else}
																		<Eye class="h-3.5 w-3.5" />
																	{/if}
																</button>
															</div>
														</div>
													{:else}
														<div
															class="border-success-border bg-success-soft text-success col-span-2 rounded-md border px-3 py-2 text-[8px] leading-relaxed"
														>
															{APPLICATION.name} uses the agent exposed by
															<code>SSH_AUTH_SOCK</code>. No private key or SSH password is copied
															into the profile.
														</div>
													{/if}

													<div>
														<label for="modal-known-hosts">Known hosts file</label>
														<input
															id="modal-known-hosts"
															bind:value={sshKnownHostsPath}
															placeholder="~/.ssh/known_hosts (default)"
															disabled={action !== null || !sshEnabled}
														/>
													</div>
													<div>
														<label for="modal-host-fingerprint">Pinned host fingerprint</label>
														<input
															id="modal-host-fingerprint"
															bind:value={sshHostKeyFingerprint}
															placeholder="SHA256:… (optional)"
															disabled={action !== null || !sshEnabled}
														/>
													</div>

													<div
														class="col-span-2 flex items-start gap-2 rounded-md border px-3 py-2.5"
													>
														<ShieldCheck class="text-success mt-0.5 h-3.5 w-3.5 shrink-0" />
														<div class="min-w-0 flex-1">
															<p class="text-[8px] font-bold">Strict host verification</p>
															<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
																The pinned SHA256 fingerprint takes priority. Otherwise the selected
																or default known_hosts file must already trust this server.
															</p>
														</div>
														{#if editingId && (hasStoredSSHPassword || hasStoredSSHKeyPassphrase)}
															<button
																type="button"
																class="text-muted-foreground hover:text-destructive shrink-0 text-[8px] font-semibold"
																onclick={clearStoredSSHCredentials}
																disabled={action !== null}
															>
																{clearSSHCredentialsConfirm
																	? 'Remove stored SSH secret'
																	: 'Remove secret'}
															</button>
														{/if}
													</div>
													<p class="text-muted-foreground col-span-2 text-[8px] leading-relaxed">
														The database host above is resolved from the SSH server. Use
														<code>{CONNECTION_DEFAULTS.host}</code> when the database only listens on
														that server.
													</p>
												</div>
											{/if}
										</div>
									{/if}
								{/if}
							</div>
						{/if}
					</div>

					<footer class="flex h-[58px] shrink-0 items-center justify-between border-t px-5">
						<div>
							{#if editingId}
								<button
									type="button"
									class="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[10px] font-semibold {deleteConfirm
										? 'bg-danger text-on-solid'
										: 'text-muted-foreground hover:bg-danger-soft hover:text-danger'}"
									onclick={deleteProfile}
									disabled={action !== null}
								>
									{#if action === 'delete'}
										<Loader2 class="h-3 w-3 animate-spin" />
									{:else}
										<Trash2 class="h-3 w-3" />
									{/if}
									{deleteConfirm ? 'Confirm delete' : 'Delete'}
								</button>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							{#if selectedProvider}
								<button
									type="button"
									class="rt-toolbar-button border-border h-8 gap-1.5 px-3 text-[10px] font-bold"
									onclick={saveProfile}
									disabled={action !== null}
								>
									{#if action === 'save'}
										<Loader2 class="h-3 w-3 animate-spin" />
									{:else}
										<Save class="h-3 w-3" />
									{/if}
									Save profile
								</button>
								{#if action === 'connect'}
									<button
										type="button"
										class="border-danger-border bg-danger-soft text-danger hover:bg-danger-soft inline-flex h-8 items-center gap-1.5 rounded-md border px-3 text-[10px] font-bold transition-colors disabled:opacity-50"
										onclick={cancelConnection}
										disabled={cancellingConnection}
									>
										{#if cancellingConnection}
											<Loader2 class="h-3 w-3 animate-spin" />
											Cancelling
										{:else}
											<X class="h-3 w-3" />
											Cancel · {connectionElapsedSeconds}s
										{/if}
									</button>
								{:else}
									<button
										type="submit"
										class="rt-primary-button inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-[10px] font-bold disabled:opacity-50"
										disabled={action !== null}
									>
										<Play class="h-3 w-3" fill="currentColor" />
										Connect
									</button>
								{/if}
							{:else}
								<span class="text-muted-foreground text-[9px] font-semibold">
									Select a provider to continue
								</span>
							{/if}
						</div>
					</footer>
				</form>
			</div>
		</div>
	</div>
{/if}
