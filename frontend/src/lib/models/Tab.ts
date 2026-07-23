export type TabKind = 'query' | 'table' | 'createTable' | 'schemaDiagram';

export interface Tab {
	id: string;
	title: string;
	kind: TabKind;
	schema?: string;
	table?: string;
	sql?: string;
	status?: string;
	level?: 'info' | 'warn' | 'error';
	activeSubTab?: 'structure' | 'data';
}
