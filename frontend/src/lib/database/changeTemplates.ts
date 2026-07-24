import type { database } from '$lib/wailsjs/go/models';

export type StructuralChangeIntent =
	| 'create-view'
	| 'create-materialized-view'
	| 'create-function'
	| 'create-procedure'
	| 'create-trigger'
	| 'edit'
	| 'rename'
	| 'drop'
	| 'enable'
	| 'disable'
	| 'create-index'
	| 'alter-column'
	| 'add-constraint'
	| 'drop-constraint';

function quoteIdentifier(identifier: string, capabilities: database.Capabilities | null): string {
	const open = capabilities?.dialect?.identifierOpen || '"';
	const close = capabilities?.dialect?.identifierClose || open;
	return `${open}${identifier.replaceAll(close, close + close)}${close}`;
}

function qualifiedName(
	schema: string,
	name: string,
	capabilities: database.Capabilities | null
): string {
	return schema
		? `${quoteIdentifier(schema, capabilities)}.${quoteIdentifier(name, capabilities)}`
		: quoteIdentifier(name, capabilities);
}

export function structuralIntentLabel(intent: StructuralChangeIntent): string {
	switch (intent) {
		case 'create-view':
			return 'Create view';
		case 'create-materialized-view':
			return 'Create materialized view';
		case 'create-function':
			return 'Create function';
		case 'create-procedure':
			return 'Create procedure';
		case 'create-trigger':
			return 'Create trigger';
		case 'edit':
			return 'Edit definition';
		case 'rename':
			return 'Rename object';
		case 'drop':
			return 'Drop object';
		case 'enable':
			return 'Enable trigger';
		case 'disable':
			return 'Disable trigger';
		case 'create-index':
			return 'Create index';
		case 'alter-column':
			return 'Alter column';
		case 'add-constraint':
			return 'Add constraint';
		case 'drop-constraint':
			return 'Drop constraint';
	}
}

export function createObjectDefinitionTemplate(
	intent: StructuralChangeIntent,
	capabilities: database.Capabilities | null,
	schema: string,
	name: string,
	parentTable = ''
): string {
	const engine = capabilities?.engine || 'postgres';
	const object = qualifiedName(schema, name, capabilities);
	const table = qualifiedName(schema, parentTable || 'table_name', capabilities);

	switch (intent) {
		case 'create-view':
		case 'create-materialized-view':
			return `SELECT\n  *\nFROM ${qualifiedName(schema, 'table_name', capabilities)}\nWHERE true`;
		case 'create-function':
			if (engine === 'mysql') {
				return `CREATE FUNCTION ${object}()\nRETURNS INTEGER\nDETERMINISTIC\nBEGIN\n  RETURN 1;\nEND;`;
			}
			return `CREATE OR REPLACE FUNCTION ${object}()\nRETURNS void\nLANGUAGE plpgsql\nAS $function$\nBEGIN\n  -- function body\nEND;\n$function$;`;
		case 'create-procedure':
			if (engine === 'mysql') {
				return `CREATE PROCEDURE ${object}()\nBEGIN\n  -- procedure body\nEND;`;
			}
			return `CREATE OR REPLACE PROCEDURE ${object}()\nLANGUAGE plpgsql\nAS $procedure$\nBEGIN\n  -- procedure body\nEND;\n$procedure$;`;
		case 'create-trigger':
			if (engine === 'sqlite') {
				return `CREATE TRIGGER ${object}\nAFTER INSERT ON ${table}\nFOR EACH ROW\nBEGIN\n  -- trigger statements\nEND;`;
			}
			if (engine === 'mysql') {
				return `CREATE TRIGGER ${object}\nAFTER INSERT ON ${table}\nFOR EACH ROW\nBEGIN\n  -- trigger statements\nEND;`;
			}
			return `CREATE OR REPLACE TRIGGER ${quoteIdentifier(name, capabilities)}\nAFTER INSERT ON ${table}\nFOR EACH ROW\nEXECUTE FUNCTION ${qualifiedName(schema, 'trigger_function', capabilities)}();`;
		default:
			return '';
	}
}

export function defaultObjectName(intent: StructuralChangeIntent): string {
	switch (intent) {
		case 'create-view':
			return 'new_view';
		case 'create-materialized-view':
			return 'new_materialized_view';
		case 'create-function':
			return 'new_function';
		case 'create-procedure':
			return 'new_procedure';
		case 'create-trigger':
			return 'new_trigger';
		case 'create-index':
			return 'new_index';
		case 'add-constraint':
			return 'new_constraint';
		default:
			return '';
	}
}
