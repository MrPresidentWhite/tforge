export namespace main {
	
	export class ImportEnvReport {
	    provided: boolean;
	    values: Record<string, string>;
	    missing: string[];
	    unknown: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportEnvReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provided = source["provided"];
	        this.values = source["values"];
	        this.missing = source["missing"];
	        this.unknown = source["unknown"];
	    }
	}
	export class ImportGroup {
	    prefix: string;
	    keys: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prefix = source["prefix"];
	        this.keys = source["keys"];
	    }
	}
	export class ImportAnalysis {
	    keys: string[];
	    groups: ImportGroup[];
	    ungrouped: string[];
	    envs: Record<string, ImportEnvReport>;
	
	    static createFrom(source: any = {}) {
	        return new ImportAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keys = source["keys"];
	        this.groups = this.convertValues(source["groups"], ImportGroup);
	        this.ungrouped = source["ungrouped"];
	        this.envs = this.convertValues(source["envs"], ImportEnvReport, true);
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
	export class ImportEntry {
	    key: string;
	    groupPrefix: string;
	    valueDev: string;
	    valueStage: string;
	    valueProd: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.groupPrefix = source["groupPrefix"];
	        this.valueDev = source["valueDev"];
	        this.valueStage = source["valueStage"];
	        this.valueProd = source["valueProd"];
	        this.type = source["type"];
	    }
	}
	
	
	export class PickedFile {
	    path: string;
	    name: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new PickedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.content = source["content"];
	    }
	}
	export class RestoreResult {
	    added: string[];
	    skipped: string[];
	
	    static createFrom(source: any = {}) {
	        return new RestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.skipped = source["skipped"];
	    }
	}
	export class VaultEnvAnalysis {
	    values: Record<string, string>;
	    missing: string[];
	    unknown: string[];
	    overwrite: string[];
	
	    static createFrom(source: any = {}) {
	        return new VaultEnvAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.values = source["values"];
	        this.missing = source["missing"];
	        this.unknown = source["unknown"];
	        this.overwrite = source["overwrite"];
	    }
	}

}

export namespace vault {
	
	export class Entry {
	    key: string;
	    valueDev?: string;
	    valueStage?: string;
	    valueProd?: string;
	    type: string;
	    groupPrefix?: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.valueDev = source["valueDev"];
	        this.valueStage = source["valueStage"];
	        this.valueProd = source["valueProd"];
	        this.type = source["type"];
	        this.groupPrefix = source["groupPrefix"];
	    }
	}
	export class Vault {
	    id: string;
	    name: string;
	    icon?: string;
	    description?: string;
	    entries: Entry[];
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Vault(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.description = source["description"];
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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

