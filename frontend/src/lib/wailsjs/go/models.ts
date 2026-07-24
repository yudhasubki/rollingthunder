export namespace database {
	
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
	export class SQLInsertOptions {
	    batchSize: number;
	    includeTransaction: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SQLInsertOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batchSize = source["batchSize"];
	        this.includeTransaction = source["includeTransaction"];
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
	
	export class QueryResult {
	    rows: any[];
	    truncated: boolean;
	    rowLimit: number;
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = source["rows"];
	        this.truncated = source["truncated"];
	        this.rowLimit = source["rowLimit"];
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
	export class Structure {
	    name: string;
	    data_type: string;
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
	        this.foreign_key = source["foreign_key"];
	        this.foreign_schema = source["foreign_schema"];
	        this.foreign_table = source["foreign_table"];
	        this.foreign_column = source["foreign_column"];
	        this.comment = source["comment"];
	    }
	}
	export class Table {
	    Schema: string;
	    Name: string;
	    Offset: number;
	    Limit: number;
	    Filter: string;
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
	        this.Filter = source["Filter"];
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
	
	    static createFrom(source: any = {}) {
	        return new ConnectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driver = source["driver"];
	        this.config = this.convertValues(source["config"], database.Config);
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
	    name: string;
	    database: string;
	    host: string;
	    color: string;
	    isActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.database = source["database"];
	        this.host = source["host"];
	        this.color = source["color"];
	        this.isActive = source["isActive"];
	    }
	}
	export class SavedConnection {
	    id: string;
	    config: database.Config;
	
	    static createFrom(source: any = {}) {
	        return new SavedConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.config = this.convertValues(source["config"], database.Config);
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

export namespace response {
	
	export class BaseErrorResponse {
	    title: string;
	    status: number;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new BaseErrorResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.status = source["status"];
	        this.detail = source["detail"];
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

