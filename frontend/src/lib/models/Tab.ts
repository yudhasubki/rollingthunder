export type TabKind = 'query' | 'table' | 'createTable' | 'schemaDiagram' | 'databaseObject';

export interface Tab {
	id: string;
	connectionId: string;
	title: string;
	kind: TabKind;
	schema?: string;
	table?: string;
	objectId?: string;
	objectKind?: string;
	objectName?: string;
	objectSignature?: string;
	parentSchema?: string;
	parentName?: string;
	sql?: string;
	sqlFileToken?: string;
	sqlFilePath?: string;
	sqlFileName?: string;
	sqlFileSavedContent?: string;
	savedQueryId?: string;
	status?: string;
	level?: 'info' | 'warn' | 'error';
	activeSubTab?: 'structure' | 'data' | 'ddl';
	focusColumn?: string;
	focusRequest?: number;
	revision?: number;
}
