export namespace main {
	
	export class ConnectionCommand {
	    protocol: string;
	    command: string;
	    target: string;
	    user: string;
	    address: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.command = source["command"];
	        this.target = source["target"];
	        this.user = source["user"];
	        this.address = source["address"];
	        this.port = source["port"];
	    }
	}
	export class Host {
	    id: string;
	    name: string;
	    target: string;
	    user: string;
	    address: string;
	    port: number;
	    command: string;
	    favorite: boolean;
	    hasPassword: boolean;
	    lastConnected: string;
	
	    static createFrom(source: any = {}) {
	        return new Host(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.user = source["user"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.command = source["command"];
	        this.favorite = source["favorite"];
	        this.hasPassword = source["hasPassword"];
	        this.lastConnected = source["lastConnected"];
	    }
	}
	export class RemoteFile {
	    name: string;
	    path: string;
	    size: number;
	    mode: string;
	    isDir: boolean;
	    modTime: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.isDir = source["isDir"];
	        this.modTime = source["modTime"];
	    }
	}
	export class SFTPListing {
	    path: string;
	    files: RemoteFile[];
	
	    static createFrom(source: any = {}) {
	        return new SFTPListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.files = this.convertValues(source["files"], RemoteFile);
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

