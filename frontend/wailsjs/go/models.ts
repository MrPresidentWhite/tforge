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

