export namespace database {
	
	export class ConstraintChange {
	    table: Table;
	    name: string;
	    definition?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConstraintChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = this.convertValues(source["table"], Table);
	        this.name = source["name"];
	        this.definition = source["definition"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ColumnChange {
	    table: Table;
	    name: string;
	    newName?: string;
	    dataType?: string;
	    using?: string;
	    nullable?: boolean;
	    default?: string;
	    dropDefault?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = this.convertValues(source["table"], Table);
	        this.name = source["name"];
	        this.newName = source["newName"];
	        this.dataType = source["dataType"];
	        this.using = source["using"];
	        this.nullable = source["nullable"];
	        this.default = source["default"];
	        this.dropDefault = source["dropDefault"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Sort {
	    Column: string;
	    Direction: string;
	    Nulls: string;
	
	    static createFrom(source: any = {}) {
	        return new Sort(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Column = source["Column"];
	        this.Direction = source["Direction"];
	        this.Nulls = source["Nulls"];
	    }
	}
	export class Filter {
	    Column: string;
	    Operator: string;
	    Value: any;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Column = source["Column"];
	        this.Operator = source["Operator"];
	        this.Value = source["Value"];
	    }
	}
	export class Table {
	    Schema: string;
	    Name: string;
	    Offset: number;
	    Limit: number;
	    Filters: Filter[];
	    Sorts: Sort[];
	
	    static createFrom(source: any = {}) {
	        return new Table(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Schema = source["Schema"];
	        this.Name = source["Name"];
	        this.Offset = source["Offset"];
	        this.Limit = source["Limit"];
	        this.Filters = this.convertValues(source["Filters"], Filter);
	        this.Sorts = this.convertValues(source["Sorts"], Sort);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IndexChange {
	    table: Table;
	    name: string;
	    columns: string[];
	    unique: boolean;
	    method?: string;
	    where?: string;
	
	    static createFrom(source: any = {}) {
	        return new IndexChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = this.convertValues(source["table"], Table);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	        this.method = source["method"];
	        this.where = source["where"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ObjectReference {
	    id?: string;
	    kind: string;
	    schema?: string;
	    name: string;
	    signature?: string;
	    parentSchema?: string;
	    parentName?: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.signature = source["signature"];
	        this.parentSchema = source["parentSchema"];
	        this.parentName = source["parentName"];
	    }
	}
	export class ObjectChangeRequest {
	    action: string;
	    reference: ObjectReference;
	    newName?: string;
	    definition?: string;
	    cascade?: boolean;
	    index?: IndexChange;
	    column?: ColumnChange;
	    constraint?: ConstraintChange;
	
	    static createFrom(source: any = {}) {
	        return new ObjectChangeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.reference = this.convertValues(source["reference"], ObjectReference);
	        this.newName = source["newName"];
	        this.definition = source["definition"];
	        this.cascade = source["cascade"];
	        this.index = this.convertValues(source["index"], IndexChange);
	        this.column = this.convertValues(source["column"], ColumnChange);
	        this.constraint = this.convertValues(source["constraint"], ConstraintChange);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ApplyObjectChangeRequest {
	    change: ObjectChangeRequest;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyObjectChangeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.change = this.convertValues(source["change"], ObjectChangeRequest);
	        this.fingerprint = source["fingerprint"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CSVOptions {
	    delimiter: string;
	    includeHeader: boolean;
	    nullValue: string;
	    encoding: string;
	
	    static createFrom(source: any = {}) {
	        return new CSVOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.delimiter = source["delimiter"];
	        this.includeHeader = source["includeHeader"];
	        this.nullValue = source["nullValue"];
	        this.encoding = source["encoding"];
	    }
	}
	export class Dialect {
	    name: string;
	    identifierOpen: string;
	    identifierClose: string;
	    placeholderStyle: string;
	    paginationStyle: string;
	    supportsNullOrdering: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Dialect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.identifierOpen = source["identifierOpen"];
	        this.identifierClose = source["identifierClose"];
	        this.placeholderStyle = source["placeholderStyle"];
	        this.paginationStyle = source["paginationStyle"];
	        this.supportsNullOrdering = source["supportsNullOrdering"];
	    }
	}
	export class Capabilities {
	    engine: string;
	    displayName: string;
	    dialect: Dialect;
	    schemas: boolean;
	    databases: boolean;
	    tables: boolean;
	    views: boolean;
	    materializedViews: boolean;
	    functions: boolean;
	    procedures: boolean;
	    triggers: boolean;
	    sequences: boolean;
	    customTypes: boolean;
	    domains: boolean;
	    constraints: boolean;
	    extensions: boolean;
	    objectDefinitions: boolean;
	    objectDependencies: boolean;
	    manageViews: boolean;
	    manageRoutines: boolean;
	    manageTriggers: boolean;
	    triggerToggle: boolean;
	    manageIndexes: boolean;
	    alterTableStructure: boolean;
	    explainPlans: boolean;
	    transactions: boolean;
	    transactionalDDL: boolean;
	    atomicTableChanges: boolean;
	    sqlInsertExport: boolean;
	    fileDatabase: boolean;
	    attachedDatabases: boolean;
	    generatedColumns: boolean;
	    upsert: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.displayName = source["displayName"];
	        this.dialect = this.convertValues(source["dialect"], Dialect);
	        this.schemas = source["schemas"];
	        this.databases = source["databases"];
	        this.tables = source["tables"];
	        this.views = source["views"];
	        this.materializedViews = source["materializedViews"];
	        this.functions = source["functions"];
	        this.procedures = source["procedures"];
	        this.triggers = source["triggers"];
	        this.sequences = source["sequences"];
	        this.customTypes = source["customTypes"];
	        this.domains = source["domains"];
	        this.constraints = source["constraints"];
	        this.extensions = source["extensions"];
	        this.objectDefinitions = source["objectDefinitions"];
	        this.objectDependencies = source["objectDependencies"];
	        this.manageViews = source["manageViews"];
	        this.manageRoutines = source["manageRoutines"];
	        this.manageTriggers = source["manageTriggers"];
	        this.triggerToggle = source["triggerToggle"];
	        this.manageIndexes = source["manageIndexes"];
	        this.alterTableStructure = source["alterTableStructure"];
	        this.explainPlans = source["explainPlans"];
	        this.transactions = source["transactions"];
	        this.transactionalDDL = source["transactionalDDL"];
	        this.atomicTableChanges = source["atomicTableChanges"];
	        this.sqlInsertExport = source["sqlInsertExport"];
	        this.fileDatabase = source["fileDatabase"];
	        this.attachedDatabases = source["attachedDatabases"];
	        this.generatedColumns = source["generatedColumns"];
	        this.upsert = source["upsert"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ColumnDefinition {
	    name: string;
	    type: string;
	    nullable: boolean;
	    default: string;
	    primaryKey: boolean;
	    unique: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.default = source["default"];
	        this.primaryKey = source["primaryKey"];
	        this.unique = source["unique"];
	    }
	}
	export class Config {
	    name: string;
	    color: string;
	    driver: string;
	    host: string;
	    port: string;
	    user: string;
	    password: string;
	    db: string;
	    sslMode: string;
	    sslCert: string;
	    sslKey: string;
	    sslRootCert: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	        this.driver = source["driver"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.db = source["db"];
	        this.sslMode = source["sslMode"];
	        this.sslCert = source["sslCert"];
	        this.sslKey = source["sslKey"];
	        this.sslRootCert = source["sslRootCert"];
	    }
	}
	export class ConnectionHealth {
	    connectionId: string;
	    state: string;
	    message?: string;
	    latencyMs: number;
	    failureCount: number;
	    lastChecked?: string;
	    lastHealthy?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionHealth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.latencyMs = source["latencyMs"];
	        this.failureCount = source["failureCount"];
	        this.lastChecked = source["lastChecked"];
	        this.lastHealthy = source["lastHealthy"];
	    }
	}
	
	export class DataType {
	    name: string;
	    category: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new DataType(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.category = source["category"];
	        this.description = source["description"];
	    }
	}
	export class ObjectProperty {
	    name: string;
	    value: string;
	    category?: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectProperty(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.category = source["category"];
	    }
	}
	export class DatabaseObject {
	    reference: ObjectReference;
	    displayName: string;
	    description?: string;
	    canOpenData: boolean;
	    canManage: boolean;
	    allowedActions?: string[];
	    properties?: ObjectProperty[];
	
	    static createFrom(source: any = {}) {
	        return new DatabaseObject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reference = this.convertValues(source["reference"], ObjectReference);
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	        this.canOpenData = source["canOpenData"];
	        this.canManage = source["canManage"];
	        this.allowedActions = source["allowedActions"];
	        this.properties = this.convertValues(source["properties"], ObjectProperty);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ExplainPlanNode {
	    id: string;
	    parentId?: string;
	    nodeType: string;
	    relation?: string;
	    summary: string;
	    startupCost?: number;
	    totalCost?: number;
	    estimatedRows?: number;
	    actualRows?: number;
	    details?: Record<string, string>;
	    children?: ExplainPlanNode[];
	
	    static createFrom(source: any = {}) {
	        return new ExplainPlanNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parentId = source["parentId"];
	        this.nodeType = source["nodeType"];
	        this.relation = source["relation"];
	        this.summary = source["summary"];
	        this.startupCost = source["startupCost"];
	        this.totalCost = source["totalCost"];
	        this.estimatedRows = source["estimatedRows"];
	        this.actualRows = source["actualRows"];
	        this.details = source["details"];
	        this.children = this.convertValues(source["children"], ExplainPlanNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExplainPlan {
	    engine: string;
	    summary: string;
	    roots: ExplainPlanNode[];
	    raw: string;
	
	    static createFrom(source: any = {}) {
	        return new ExplainPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.summary = source["summary"];
	        this.roots = this.convertValues(source["roots"], ExplainPlanNode);
	        this.raw = source["raw"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SQLInsertOptions {
	    batchSize: number;
	    includeTransaction: boolean;
	    upsert: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SQLInsertOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batchSize = source["batchSize"];
	        this.includeTransaction = source["includeTransaction"];
	        this.upsert = source["upsert"];
	    }
	}
	export class JSONOptions {
	    pretty: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JSONOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pretty = source["pretty"];
	    }
	}
	export class ExportOptions {
	    format: string;
	    csv: CSVOptions;
	    json: JSONOptions;
	    sql: SQLInsertOptions;
	
	    static createFrom(source: any = {}) {
	        return new ExportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.csv = this.convertValues(source["csv"], CSVOptions);
	        this.json = this.convertValues(source["json"], JSONOptions);
	        this.sql = this.convertValues(source["sql"], SQLInsertOptions);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExportProgress {
	    jobId: string;
	    status: string;
	    rows: number;
	    bytes: number;
	    totalRows: number;
	    elapsedMs: number;
	    cancellable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExportProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.status = source["status"];
	        this.rows = source["rows"];
	        this.bytes = source["bytes"];
	        this.totalRows = source["totalRows"];
	        this.elapsedMs = source["elapsedMs"];
	        this.cancellable = source["cancellable"];
	    }
	}
	export class ExportResult {
	    path: string;
	    rows: number;
	    bytes: number;
	    cancelled: boolean;
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.rows = source["rows"];
	        this.bytes = source["bytes"];
	        this.cancelled = source["cancelled"];
	        this.format = source["format"];
	    }
	}
	
	export class ImportColumn {
	    sourceName: string;
	    targetName: string;
	    inferredType: string;
	    nullable: boolean;
	    included: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceName = source["sourceName"];
	        this.targetName = source["targetName"];
	        this.inferredType = source["inferredType"];
	        this.nullable = source["nullable"];
	        this.included = source["included"];
	    }
	}
	export class ImportFileSelection {
	    token: string;
	    name: string;
	    format: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportFileSelection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.name = source["name"];
	        this.format = source["format"];
	        this.size = source["size"];
	    }
	}
	export class ImportOptions {
	    format: string;
	    delimiter?: string;
	    header: boolean;
	    emptyAsNull: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.delimiter = source["delimiter"];
	        this.header = source["header"];
	        this.emptyAsNull = source["emptyAsNull"];
	    }
	}
	export class ImportPreview {
	    file: ImportFileSelection;
	    columns: ImportColumn[];
	    rows: any[];
	    sampled: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = this.convertValues(source["file"], ImportFileSelection);
	        this.columns = this.convertValues(source["columns"], ImportColumn);
	        this.rows = source["rows"];
	        this.sampled = source["sampled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportPreviewRequest {
	    token: string;
	    options: ImportOptions;
	    limit?: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportPreviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.options = this.convertValues(source["options"], ImportOptions);
	        this.limit = source["limit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportRequest {
	    connectionId: string;
	    token: string;
	    options: ImportOptions;
	    schema: string;
	    table: string;
	    createTable: boolean;
	    columns: ImportColumn[];
	
	    static createFrom(source: any = {}) {
	        return new ImportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.token = source["token"];
	        this.options = this.convertValues(source["options"], ImportOptions);
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.createTable = source["createTable"];
	        this.columns = this.convertValues(source["columns"], ImportColumn);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportResult {
	    schema: string;
	    table: string;
	    rowsInserted: number;
	    tableCreated: boolean;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.rowsInserted = source["rowsInserted"];
	        this.tableCreated = source["tableCreated"];
	        this.warnings = source["warnings"];
	    }
	}
	export class Index {
	    name: string;
	    columns: string[];
	    is_unique: boolean;
	    is_primary: boolean;
	    algorithm: string;
	
	    static createFrom(source: any = {}) {
	        return new Index(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.is_unique = source["is_unique"];
	        this.is_primary = source["is_primary"];
	        this.algorithm = source["algorithm"];
	    }
	}
	
	export class Info {
	    engine: string;
	    version: string;
	    database: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.version = source["version"];
	        this.database = source["database"];
	    }
	}
	
	export class ObjectChangePreview {
	    summary: string;
	    sql: string;
	    statementCount: number;
	    destructive: boolean;
	    transactional: boolean;
	    warnings: string[];
	    fingerprint: string;
	    refresh: ObjectReference[];
	
	    static createFrom(source: any = {}) {
	        return new ObjectChangePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.sql = source["sql"];
	        this.statementCount = source["statementCount"];
	        this.destructive = source["destructive"];
	        this.transactional = source["transactional"];
	        this.warnings = source["warnings"];
	        this.fingerprint = source["fingerprint"];
	        this.refresh = this.convertValues(source["refresh"], ObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ObjectChangeResult {
	    applied: boolean;
	    statementCount: number;
	    fingerprint: string;
	    refresh: ObjectReference[];
	
	    static createFrom(source: any = {}) {
	        return new ObjectChangeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applied = source["applied"];
	        this.statementCount = source["statementCount"];
	        this.fingerprint = source["fingerprint"];
	        this.refresh = this.convertValues(source["refresh"], ObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ObjectDependency {
	    reference: ObjectReference;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectDependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reference = this.convertValues(source["reference"], ObjectReference);
	        this.description = source["description"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Structure {
	    name: string;
	    data_type: string;
	    native_type?: string;
	    affinity?: string;
	    type_schema?: string;
	    type_name?: string;
	    is_enum?: boolean;
	    length?: number;
	    nullable: boolean;
	    default?: string;
	    is_primary?: boolean;
	    is_primary_label?: string;
	    is_unique?: boolean;
	    is_autoinc?: boolean;
	    is_generated?: boolean;
	    generation?: string;
	    is_rowid?: boolean;
	    foreign_key?: string;
	    foreign_schema?: string;
	    foreign_table?: string;
	    foreign_column?: string;
	    comment?: string;
	
	    static createFrom(source: any = {}) {
	        return new Structure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.data_type = source["data_type"];
	        this.native_type = source["native_type"];
	        this.affinity = source["affinity"];
	        this.type_schema = source["type_schema"];
	        this.type_name = source["type_name"];
	        this.is_enum = source["is_enum"];
	        this.length = source["length"];
	        this.nullable = source["nullable"];
	        this.default = source["default"];
	        this.is_primary = source["is_primary"];
	        this.is_primary_label = source["is_primary_label"];
	        this.is_unique = source["is_unique"];
	        this.is_autoinc = source["is_autoinc"];
	        this.is_generated = source["is_generated"];
	        this.generation = source["generation"];
	        this.is_rowid = source["is_rowid"];
	        this.foreign_key = source["foreign_key"];
	        this.foreign_schema = source["foreign_schema"];
	        this.foreign_table = source["foreign_table"];
	        this.foreign_column = source["foreign_column"];
	        this.comment = source["comment"];
	    }
	}
	export class ObjectDetail {
	    object: DatabaseObject;
	    definition?: string;
	    comment?: string;
	    properties?: ObjectProperty[];
	    columns?: Structure[];
	    dependencies?: ObjectDependency[];
	    dependents?: ObjectDependency[];
	
	    static createFrom(source: any = {}) {
	        return new ObjectDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.object = this.convertValues(source["object"], DatabaseObject);
	        this.definition = source["definition"];
	        this.comment = source["comment"];
	        this.properties = this.convertValues(source["properties"], ObjectProperty);
	        this.columns = this.convertValues(source["columns"], Structure);
	        this.dependencies = this.convertValues(source["dependencies"], ObjectDependency);
	        this.dependents = this.convertValues(source["dependents"], ObjectDependency);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ObjectFilter {
	    schema?: string;
	    kinds?: string[];
	    search?: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.kinds = source["kinds"];
	        this.search = source["search"];
	    }
	}
	
	
	export class QueryVariable {
	    name: string;
	    value: any;
	    type?: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryVariable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.type = source["type"];
	    }
	}
	export class QueryRequest {
	    connectionId: string;
	    query: string;
	    attemptId: string;
	    transactionId?: string;
	    allowUnfilteredMutation: boolean;
	    variables?: QueryVariable[];
	
	    static createFrom(source: any = {}) {
	        return new QueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.query = source["query"];
	        this.attemptId = source["attemptId"];
	        this.transactionId = source["transactionId"];
	        this.allowUnfilteredMutation = source["allowUnfilteredMutation"];
	        this.variables = this.convertValues(source["variables"], QueryVariable);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QueryResultSet {
	    index: number;
	    statement: string;
	    columns: string[];
	    rows: any[];
	    truncated: boolean;
	    rowLimit: number;
	
	    static createFrom(source: any = {}) {
	        return new QueryResultSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.statement = source["statement"];
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.truncated = source["truncated"];
	        this.rowLimit = source["rowLimit"];
	    }
	}
	export class QueryResult {
	    rows: any[];
	    truncated: boolean;
	    rowLimit: number;
	    columns: string[];
	    resultSets: QueryResultSet[];
	    statementCount: number;
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = source["rows"];
	        this.truncated = source["truncated"];
	        this.rowLimit = source["rowLimit"];
	        this.columns = source["columns"];
	        this.resultSets = this.convertValues(source["resultSets"], QueryResultSet);
	        this.statementCount = source["statementCount"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class RowUpdate {
	    original: Record<string, any>;
	    values: Record<string, any>;
	    changedColumns: string[];
	
	    static createFrom(source: any = {}) {
	        return new RowUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.original = source["original"];
	        this.values = source["values"];
	        this.changedColumns = source["changedColumns"];
	    }
	}
	export class RowsExportRequest {
	    columns: string[];
	    rows: any[];
	    jobId: string;
	    expectedRows: number;
	    suggestedName: string;
	    options: ExportOptions;
	
	    static createFrom(source: any = {}) {
	        return new RowsExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.jobId = source["jobId"];
	        this.expectedRows = source["expectedRows"];
	        this.suggestedName = source["suggestedName"];
	        this.options = this.convertValues(source["options"], ExportOptions);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class TableChangeResult {
	    inserted: number;
	    updated: number;
	    deleted: number;
	
	    static createFrom(source: any = {}) {
	        return new TableChangeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inserted = source["inserted"];
	        this.updated = source["updated"];
	        this.deleted = source["deleted"];
	    }
	}
	export class TableChangeSet {
	    table: Table;
	    added: any[];
	    updated: RowUpdate[];
	    deleted: any[];
	
	    static createFrom(source: any = {}) {
	        return new TableChangeSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = this.convertValues(source["table"], Table);
	        this.added = source["added"];
	        this.updated = this.convertValues(source["updated"], RowUpdate);
	        this.deleted = source["deleted"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableData {
	    structures: Structure[];
	    data: any[];
	
	    static createFrom(source: any = {}) {
	        return new TableData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.structures = this.convertValues(source["structures"], Structure);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableExportRequest {
	    table: Table;
	    scope: string;
	    selectedRowIndexes: number[];
	    jobId: string;
	    expectedRows: number;
	    suggestedName: string;
	    options: ExportOptions;
	
	    static createFrom(source: any = {}) {
	        return new TableExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = this.convertValues(source["table"], Table);
	        this.scope = source["scope"];
	        this.selectedRowIndexes = source["selectedRowIndexes"];
	        this.jobId = source["jobId"];
	        this.expectedRows = source["expectedRows"];
	        this.suggestedName = source["suggestedName"];
	        this.options = this.convertValues(source["options"], ExportOptions);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace db {
	
	export class ConnectRequest {
	    driver: string;
	    config: database.Config;
	    attemptId?: string;
	    profileId?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driver = source["driver"];
	        this.config = this.convertValues(source["config"], database.Config);
	        this.attemptId = source["attemptId"];
	        this.profileId = source["profileId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConnectResponse {
	    connected: boolean;
	    connectionId?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.connectionId = source["connectionId"];
	    }
	}
	export class ConnectionInfo {
	    id: string;
	    profileId?: string;
	    name: string;
	    driver: string;
	    database: string;
	    host: string;
	    color: string;
	    isActive: boolean;
	    health: database.ConnectionHealth;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.profileId = source["profileId"];
	        this.name = source["name"];
	        this.driver = source["driver"];
	        this.database = source["database"];
	        this.host = source["host"];
	        this.color = source["color"];
	        this.isActive = source["isActive"];
	        this.health = this.convertValues(source["health"], database.ConnectionHealth);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SavedConnection {
	    id: string;
	    config: database.Config;
	    hasPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SavedConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.config = this.convertValues(source["config"], database.Config);
	        this.hasPassword = source["hasPassword"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TransactionInfo {
	    id: string;
	    connectionId: string;
	    state: string;
	    // Go type: time
	    startedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new TransactionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.connectionId = source["connectionId"];
	        this.state = source["state"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace diagnostics {
	
	export class ExportResult {
	    path: string;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.files = source["files"];
	    }
	}
	export class FrontendReport {
	    message: string;
	    stack?: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new FrontendReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.stack = source["stack"];
	        this.source = source["source"];
	    }
	}
	export class Settings {
	    enabled: boolean;
	    includeSystemInfo: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.includeSystemInfo = source["includeSystemInfo"];
	    }
	}

}

export namespace response {
	
	export class BaseErrorResponse {
	    title: string;
	    status: number;
	    code?: string;
	    detail: string;
	    hint?: string;
	
	    static createFrom(source: any = {}) {
	        return new BaseErrorResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.status = source["status"];
	        this.code = source["code"];
	        this.detail = source["detail"];
	        this.hint = source["hint"];
	    }
	}
	export class BaseResponse___rollingthunder_internal_db_ConnectionInfo_ {
	    errors?: BaseErrorResponse[];
	    data?: db.ConnectionInfo[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse___rollingthunder_internal_db_ConnectionInfo_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], db.ConnectionInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse___rollingthunder_internal_db_SavedConnection_ {
	    errors?: BaseErrorResponse[];
	    data?: db.SavedConnection[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse___rollingthunder_internal_db_SavedConnection_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], db.SavedConnection);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse___rollingthunder_pkg_database_ConnectionHealth_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ConnectionHealth[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse___rollingthunder_pkg_database_ConnectionHealth_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ConnectionHealth);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse___rollingthunder_pkg_database_DataType_ {
	    errors?: BaseErrorResponse[];
	    data?: database.DataType[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse___rollingthunder_pkg_database_DataType_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.DataType);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse___rollingthunder_pkg_database_DatabaseObject_ {
	    errors?: BaseErrorResponse[];
	    data?: database.DatabaseObject[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse___rollingthunder_pkg_database_DatabaseObject_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.DatabaseObject);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse___string_ {
	    errors?: BaseErrorResponse[];
	    data?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse___string_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_bool_ {
	    errors?: BaseErrorResponse[];
	    data?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_bool_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_int_ {
	    errors?: BaseErrorResponse[];
	    data?: number;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_int_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_internal_db_ConnectResponse_ {
	    errors?: BaseErrorResponse[];
	    data?: db.ConnectResponse;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_internal_db_ConnectResponse_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], db.ConnectResponse);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_internal_db_SavedConnection_ {
	    errors?: BaseErrorResponse[];
	    data?: db.SavedConnection;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_internal_db_SavedConnection_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], db.SavedConnection);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_internal_db_TransactionInfo_ {
	    errors?: BaseErrorResponse[];
	    data?: db.TransactionInfo;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_internal_db_TransactionInfo_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], db.TransactionInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_internal_diagnostics_ExportResult_ {
	    errors?: BaseErrorResponse[];
	    data?: diagnostics.ExportResult;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_internal_diagnostics_ExportResult_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], diagnostics.ExportResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_internal_diagnostics_Settings_ {
	    errors?: BaseErrorResponse[];
	    data?: diagnostics.Settings;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_internal_diagnostics_Settings_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], diagnostics.Settings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_Capabilities_ {
	    errors?: BaseErrorResponse[];
	    data?: database.Capabilities;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_Capabilities_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.Capabilities);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ConnectionHealth_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ConnectionHealth;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ConnectionHealth_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ConnectionHealth);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ExplainPlan_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ExplainPlan;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ExplainPlan_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ExplainPlan);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ExportProgress_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ExportProgress;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ExportProgress_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ExportProgress);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ExportResult_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ExportResult;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ExportResult_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ExportResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ImportFileSelection_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ImportFileSelection;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ImportFileSelection_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ImportFileSelection);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ImportPreview_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ImportPreview;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ImportPreview_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ImportPreview);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ImportResult_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ImportResult;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ImportResult_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ImportResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_Indices_ {
	    errors?: BaseErrorResponse[];
	    data?: database.Index[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_Indices_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.Index);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_Info_ {
	    errors?: BaseErrorResponse[];
	    data?: database.Info;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_Info_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.Info);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ObjectChangePreview_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ObjectChangePreview;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ObjectChangePreview_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ObjectChangePreview);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ObjectChangeResult_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ObjectChangeResult;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ObjectChangeResult_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ObjectChangeResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_ObjectDetail_ {
	    errors?: BaseErrorResponse[];
	    data?: database.ObjectDetail;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_ObjectDetail_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.ObjectDetail);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_QueryResult_ {
	    errors?: BaseErrorResponse[];
	    data?: database.QueryResult;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_QueryResult_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.QueryResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_Structures_ {
	    errors?: BaseErrorResponse[];
	    data?: database.Structure[];
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_Structures_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.Structure);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_TableChangeResult_ {
	    errors?: BaseErrorResponse[];
	    data?: database.TableChangeResult;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_TableChangeResult_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.TableChangeResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_rollingthunder_pkg_database_TableData_ {
	    errors?: BaseErrorResponse[];
	    data?: database.TableData;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_rollingthunder_pkg_database_TableData_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = this.convertValues(source["data"], database.TableData);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BaseResponse_string_ {
	    errors?: BaseErrorResponse[];
	    data?: string;
	
	    static createFrom(source: any = {}) {
	        return new BaseResponse_string_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.errors = this.convertValues(source["errors"], BaseErrorResponse);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

