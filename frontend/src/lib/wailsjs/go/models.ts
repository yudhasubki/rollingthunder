export namespace database {
	
	export class Config {
	    host: string;
	    port: string;
	    user: string;
	    password: string;
	    db: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.db = source["db"];
	    }
	}
	export class Structure {
	    name: string;
	    data_type: string;
	    length?: number;
	    nullable: boolean;
	    default?: string;
	    is_primary?: boolean;
	    is_unique?: boolean;
	    is_autoinc?: boolean;
	    foreign_key?: string;
	    comment?: string;
	
	    static createFrom(source: any = {}) {
	        return new Structure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.data_type = source["data_type"];
	        this.length = source["length"];
	        this.nullable = source["nullable"];
	        this.default = source["default"];
	        this.is_primary = source["is_primary"];
	        this.is_unique = source["is_unique"];
	        this.is_autoinc = source["is_autoinc"];
	        this.foreign_key = source["foreign_key"];
	        this.comment = source["comment"];
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
	
	    static createFrom(source: any = {}) {
	        return new ConnectResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
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

}

