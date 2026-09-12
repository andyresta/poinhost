export namespace files {
	
	export class ChmodRequest {
	    serverId: string;
	    path: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new ChmodRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.mode = source["mode"];
	    }
	}
	export class CompressRequest {
	    serverId: string;
	    sources: string[];
	    archivePath: string;
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new CompressRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.sources = source["sources"];
	        this.archivePath = source["archivePath"];
	        this.format = source["format"];
	    }
	}
	export class CreateFileRequest {
	    serverId: string;
	    path: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}
	export class DeleteRequest {
	    serverId: string;
	    paths: string[];
	
	    static createFrom(source: any = {}) {
	        return new DeleteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.paths = source["paths"];
	    }
	}
	export class ExtractRequest {
	    serverId: string;
	    archivePath: string;
	    destPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtractRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.archivePath = source["archivePath"];
	        this.destPath = source["destPath"];
	    }
	}
	export class ListResult {
	    path: string;
	    parent: string;
	    entries: sshpool.FileEntry[];
	
	    static createFrom(source: any = {}) {
	        return new ListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.parent = source["parent"];
	        this.entries = this.convertValues(source["entries"], sshpool.FileEntry);
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
	export class MkdirRequest {
	    serverId: string;
	    path: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new MkdirRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}
	export class ReadResult {
	    path: string;
	    content: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.size = source["size"];
	    }
	}
	export class RenameRequest {
	    serverId: string;
	    oldPath: string;
	    newPath: string;
	
	    static createFrom(source: any = {}) {
	        return new RenameRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.oldPath = source["oldPath"];
	        this.newPath = source["newPath"];
	    }
	}
	export class WriteFileRequest {
	    serverId: string;
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new WriteFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}

}

export namespace servers {
	
	export class ConnectionTestResult {
	    status: string;
	    latency?: string;
	    fingerprint?: string;
	    oldFingerprint?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.latency = source["latency"];
	        this.fingerprint = source["fingerprint"];
	        this.oldFingerprint = source["oldFingerprint"];
	        this.message = source["message"];
	    }
	}
	export class SaveServerRequest {
	    id?: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    authType: string;
	    keyPath?: string;
	    password?: string;
	    tags: string[];
	    color: string;
	    notes: string;
	    useSudo: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SaveServerRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authType = source["authType"];
	        this.keyPath = source["keyPath"];
	        this.password = source["password"];
	        this.tags = source["tags"];
	        this.color = source["color"];
	        this.notes = source["notes"];
	        this.useSudo = source["useSudo"];
	    }
	}
	export class Server {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    authType: string;
	    keyPath?: string;
	    tags: string[];
	    color: string;
	    notes: string;
	    useSudo: boolean;
	    isActive: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authType = source["authType"];
	        this.keyPath = source["keyPath"];
	        this.tags = source["tags"];
	        this.color = source["color"];
	        this.notes = source["notes"];
	        this.useSudo = source["useSudo"];
	        this.isActive = source["isActive"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class ServerStatus {
	    id: string;
	    connection: string;
	    osName?: string;
	    hostname?: string;
	    uptimeSeconds?: number;
	    cpuCores?: number;
	    cpuUsedPct?: number;
	    memUsedMb?: number;
	    memTotalMb?: number;
	    diskUsedGb?: number;
	    diskTotalGb?: number;
	    // Go type: time
	    metricsCheckedAt?: any;
	    metricsStale: boolean;
	    metricsError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.connection = source["connection"];
	        this.osName = source["osName"];
	        this.hostname = source["hostname"];
	        this.uptimeSeconds = source["uptimeSeconds"];
	        this.cpuCores = source["cpuCores"];
	        this.cpuUsedPct = source["cpuUsedPct"];
	        this.memUsedMb = source["memUsedMb"];
	        this.memTotalMb = source["memTotalMb"];
	        this.diskUsedGb = source["diskUsedGb"];
	        this.diskTotalGb = source["diskTotalGb"];
	        this.metricsCheckedAt = this.convertValues(source["metricsCheckedAt"], null);
	        this.metricsStale = source["metricsStale"];
	        this.metricsError = source["metricsError"];
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

export namespace session {
	
	export class Tab {
	    id: string;
	    serverId: string;
	    title: string;
	    activeModule: string;
	    position: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    lastActiveAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Tab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.title = source["title"];
	        this.activeModule = source["activeModule"];
	        this.position = source["position"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.lastActiveAt = this.convertValues(source["lastActiveAt"], null);
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

export namespace sshpool {
	
	export class FileEntry {
	    name: string;
	    path: string;
	    size: number;
	    isDir: boolean;
	    mode: string;
	    modTime: string;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.isDir = source["isDir"];
	        this.mode = source["mode"];
	        this.modTime = source["modTime"];
	    }
	}

}

