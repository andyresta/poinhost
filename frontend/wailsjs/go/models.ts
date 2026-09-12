export namespace backup {
	
	export class ImportSummary {
	    serversImported: number;
	    dbCredentialsImported: number;
	    domainLinksImported: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serversImported = source["serversImported"];
	        this.dbCredentialsImported = source["dbCredentialsImported"];
	        this.domainLinksImported = source["domainLinksImported"];
	    }
	}

}

export namespace docker {
	
	export class ContainerInfo {
	    id: string;
	    name: string;
	    image: string;
	    status: string;
	    state: string;
	    ports: string;
	    created: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.image = source["image"];
	        this.status = source["status"];
	        this.state = source["state"];
	        this.ports = source["ports"];
	        this.created = source["created"];
	    }
	}
	export class VolumeMount {
	    hostPath: string;
	    containerPath: string;
	    readOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VolumeMount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostPath = source["hostPath"];
	        this.containerPath = source["containerPath"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class PortMapping {
	    hostPort: number;
	    containerPort: number;
	    protocol: string;
	    hostIp?: string;
	
	    static createFrom(source: any = {}) {
	        return new PortMapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostPort = source["hostPort"];
	        this.containerPort = source["containerPort"];
	        this.protocol = source["protocol"];
	        this.hostIp = source["hostIp"];
	    }
	}
	export class EnvVar {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvVar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class ContainerInspectResponse {
	    containerId: string;
	    name: string;
	    image: string;
	    state: string;
	    env: EnvVar[];
	    ports: PortMapping[];
	    volumes: VolumeMount[];
	    memoryBytes: number;
	    restartPolicy?: string;
	    networkMode?: string;
	    cmd?: string[];
	    workingDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerInspectResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containerId = source["containerId"];
	        this.name = source["name"];
	        this.image = source["image"];
	        this.state = source["state"];
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.ports = this.convertValues(source["ports"], PortMapping);
	        this.volumes = this.convertValues(source["volumes"], VolumeMount);
	        this.memoryBytes = source["memoryBytes"];
	        this.restartPolicy = source["restartPolicy"];
	        this.networkMode = source["networkMode"];
	        this.cmd = source["cmd"];
	        this.workingDir = source["workingDir"];
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
	export class EngineStatus {
	    installed: boolean;
	    version?: string;
	    active: boolean;
	    enabled: boolean;
	    distroId?: string;
	    distroName?: string;
	    packageManager?: string;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EngineStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.active = source["active"];
	        this.enabled = source["enabled"];
	        this.distroId = source["distroId"];
	        this.distroName = source["distroName"];
	        this.packageManager = source["packageManager"];
	        this.canInstall = source["canInstall"];
	    }
	}
	
	export class ListResponse {
	    containers: ContainerInfo[];
	    dockerOk: boolean;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containers = this.convertValues(source["containers"], ContainerInfo);
	        this.dockerOk = source["dockerOk"];
	        this.version = source["version"];
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
	export class LogsRequest {
	    serverId: string;
	    containerId: string;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new LogsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.containerId = source["containerId"];
	        this.lines = source["lines"];
	    }
	}
	export class LogsResponse {
	    containerId: string;
	    name?: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new LogsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containerId = source["containerId"];
	        this.name = source["name"];
	        this.content = source["content"];
	    }
	}
	export class NetworkCreateRequest {
	    serverId: string;
	    name: string;
	    driver?: string;
	    subnet?: string;
	    gateway?: string;
	    internal: boolean;
	    attachable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NetworkCreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.name = source["name"];
	        this.driver = source["driver"];
	        this.subnet = source["subnet"];
	        this.gateway = source["gateway"];
	        this.internal = source["internal"];
	        this.attachable = source["attachable"];
	    }
	}
	export class NetworkInfo {
	    id: string;
	    name: string;
	    driver: string;
	    scope: string;
	    subnet?: string;
	    gateway?: string;
	    internal: boolean;
	    attachable: boolean;
	    containerCount: number;
	    labels?: Record<string, string>;
	    created?: string;
	    builtin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NetworkInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.driver = source["driver"];
	        this.scope = source["scope"];
	        this.subnet = source["subnet"];
	        this.gateway = source["gateway"];
	        this.internal = source["internal"];
	        this.attachable = source["attachable"];
	        this.containerCount = source["containerCount"];
	        this.labels = source["labels"];
	        this.created = source["created"];
	        this.builtin = source["builtin"];
	    }
	}
	export class NetworkListResponse {
	    networks: NetworkInfo[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.networks = this.convertValues(source["networks"], NetworkInfo);
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
	
	export class RecreateContainerRequest {
	    serverId: string;
	    containerId: string;
	    env: EnvVar[];
	    ports: PortMapping[];
	    volumes: VolumeMount[];
	    memoryBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new RecreateContainerRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.containerId = source["containerId"];
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.ports = this.convertValues(source["ports"], PortMapping);
	        this.volumes = this.convertValues(source["volumes"], VolumeMount);
	        this.memoryBytes = source["memoryBytes"];
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
	export class RecreateContainerResponse {
	    container: ContainerInfo;
	
	    static createFrom(source: any = {}) {
	        return new RecreateContainerResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.container = this.convertValues(source["container"], ContainerInfo);
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
	export class StatsResponse {
	    containerId: string;
	    name: string;
	    cpuPerc: string;
	    memUsage: string;
	    memPerc: string;
	    netIO: string;
	    blockIO: string;
	    pids: string;
	
	    static createFrom(source: any = {}) {
	        return new StatsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containerId = source["containerId"];
	        this.name = source["name"];
	        this.cpuPerc = source["cpuPerc"];
	        this.memUsage = source["memUsage"];
	        this.memPerc = source["memPerc"];
	        this.netIO = source["netIO"];
	        this.blockIO = source["blockIO"];
	        this.pids = source["pids"];
	    }
	}

}

export namespace files {
	
	export class ChmodRequest {
	    serverId: string;
	    path: string;
	    mode: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChmodRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.mode = source["mode"];
	        this.asUser = source["asUser"];
	    }
	}
	export class CompressRequest {
	    serverId: string;
	    sources: string[];
	    archivePath: string;
	    format: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new CompressRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.sources = source["sources"];
	        this.archivePath = source["archivePath"];
	        this.format = source["format"];
	        this.asUser = source["asUser"];
	    }
	}
	export class CopyRequest {
	    serverId: string;
	    sources: string[];
	    destPath: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new CopyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.sources = source["sources"];
	        this.destPath = source["destPath"];
	        this.asUser = source["asUser"];
	    }
	}
	export class CreateFileRequest {
	    serverId: string;
	    path: string;
	    name: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.asUser = source["asUser"];
	    }
	}
	export class DeleteRequest {
	    serverId: string;
	    paths: string[];
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.paths = source["paths"];
	        this.asUser = source["asUser"];
	    }
	}
	export class ExtractRequest {
	    serverId: string;
	    archivePath: string;
	    destPath: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtractRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.archivePath = source["archivePath"];
	        this.destPath = source["destPath"];
	        this.asUser = source["asUser"];
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
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new MkdirRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.asUser = source["asUser"];
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
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new RenameRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.oldPath = source["oldPath"];
	        this.newPath = source["newPath"];
	        this.asUser = source["asUser"];
	    }
	}
	export class SearchHit {
	    path: string;
	    name: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.isDir = source["isDir"];
	    }
	}
	export class SearchRequest {
	    serverId: string;
	    path: string;
	    query: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.query = source["query"];
	        this.asUser = source["asUser"];
	    }
	}
	export class SearchResult {
	    query: string;
	    hits: SearchHit[];
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.hits = this.convertValues(source["hits"], SearchHit);
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
	export class SystemUser {
	    username: string;
	    home: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.home = source["home"];
	    }
	}
	export class WriteFileRequest {
	    serverId: string;
	    path: string;
	    content: string;
	    asUser?: string;
	
	    static createFrom(source: any = {}) {
	        return new WriteFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.path = source["path"];
	        this.content = source["content"];
	        this.asUser = source["asUser"];
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

export namespace website {
	
	export class CreateSubdomainRequest {
	    serverId: string;
	    parent: string;
	    subdomain: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateSubdomainRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.parent = source["parent"];
	        this.subdomain = source["subdomain"];
	    }
	}
	export class CreateWebsiteRequest {
	    serverId: string;
	    domain: string;
	    phpVersion?: string;
	    enableSsl: boolean;
	    sslEmail?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateWebsiteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.phpVersion = source["phpVersion"];
	        this.enableSsl = source["enableSsl"];
	        this.sslEmail = source["sslEmail"];
	    }
	}
	export class ProxyRule {
	    path: string;
	    target: string;
	    webSocket?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProxyRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.target = source["target"];
	        this.webSocket = source["webSocket"];
	    }
	}
	export class DomainInfo {
	    domain: string;
	    parent?: string;
	    isSubdomain: boolean;
	    serverNames: string;
	    root: string;
	    configPath: string;
	    enabled: boolean;
	    phpVersion?: string;
	    phpEnabled: boolean;
	    proxyTarget?: string;
	    proxyWebSocket?: boolean;
	    proxyRules?: ProxyRule[];
	    sslEnabled: boolean;
	    sslCertificate?: string;
	    sslCertificateKey?: string;
	
	    static createFrom(source: any = {}) {
	        return new DomainInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.parent = source["parent"];
	        this.isSubdomain = source["isSubdomain"];
	        this.serverNames = source["serverNames"];
	        this.root = source["root"];
	        this.configPath = source["configPath"];
	        this.enabled = source["enabled"];
	        this.phpVersion = source["phpVersion"];
	        this.phpEnabled = source["phpEnabled"];
	        this.proxyTarget = source["proxyTarget"];
	        this.proxyWebSocket = source["proxyWebSocket"];
	        this.proxyRules = this.convertValues(source["proxyRules"], ProxyRule);
	        this.sslEnabled = source["sslEnabled"];
	        this.sslCertificate = source["sslCertificate"];
	        this.sslCertificateKey = source["sslCertificateKey"];
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
	export class CreateWebsiteResult {
	    domain: DomainInfo;
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CreateWebsiteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = this.convertValues(source["domain"], DomainInfo);
	        this.warnings = source["warnings"];
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
	export class CronHeader {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new CronHeader(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class CronJobInfo {
	    id: string;
	    domain: string;
	    taskType: string;
	    schedule: string;
	    scheduleText: string;
	    command?: string;
	    url?: string;
	    method?: string;
	    payload?: string;
	    headers?: CronHeader[];
	    description?: string;
	    enabled: boolean;
	    actionSummary: string;
	    logPath: string;
	
	    static createFrom(source: any = {}) {
	        return new CronJobInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.domain = source["domain"];
	        this.taskType = source["taskType"];
	        this.schedule = source["schedule"];
	        this.scheduleText = source["scheduleText"];
	        this.command = source["command"];
	        this.url = source["url"];
	        this.method = source["method"];
	        this.payload = source["payload"];
	        this.headers = this.convertValues(source["headers"], CronHeader);
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	        this.actionSummary = source["actionSummary"];
	        this.logPath = source["logPath"];
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
	export class CronJobRequest {
	    serverId: string;
	    domain: string;
	    id?: string;
	    taskType: string;
	    schedule: string;
	    command?: string;
	    url?: string;
	    method?: string;
	    payload?: string;
	    headers?: CronHeader[];
	    description?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CronJobRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.id = source["id"];
	        this.taskType = source["taskType"];
	        this.schedule = source["schedule"];
	        this.command = source["command"];
	        this.url = source["url"];
	        this.method = source["method"];
	        this.payload = source["payload"];
	        this.headers = this.convertValues(source["headers"], CronHeader);
	        this.description = source["description"];
	        this.enabled = source["enabled"];
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
	export class CronListResponse {
	    domain: string;
	    domainRoot: string;
	    jobs: CronJobInfo[];
	    active: number;
	    inactive: number;
	
	    static createFrom(source: any = {}) {
	        return new CronListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.domainRoot = source["domainRoot"];
	        this.jobs = this.convertValues(source["jobs"], CronJobInfo);
	        this.active = source["active"];
	        this.inactive = source["inactive"];
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
	export class CronLogRequest {
	    serverId: string;
	    domain: string;
	    id: string;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new CronLogRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.id = source["id"];
	        this.lines = source["lines"];
	    }
	}
	export class CronLogResponse {
	    id: string;
	    domain: string;
	    path: string;
	    content: string;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CronLogResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.domain = source["domain"];
	        this.path = source["path"];
	        this.content = source["content"];
	        this.exists = source["exists"];
	    }
	}
	export class CronToggleRequest {
	    serverId: string;
	    domain: string;
	    id: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CronToggleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.id = source["id"];
	        this.enabled = source["enabled"];
	    }
	}
	export class DBCreateDatabaseRequest {
	    serverId: string;
	    engine: string;
	    domain?: string;
	    name: string;
	    encoding?: string;
	
	    static createFrom(source: any = {}) {
	        return new DBCreateDatabaseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.engine = source["engine"];
	        this.domain = source["domain"];
	        this.name = source["name"];
	        this.encoding = source["encoding"];
	    }
	}
	export class DBCreateUserRequest {
	    serverId: string;
	    engine: string;
	    domain?: string;
	    username: string;
	    password: string;
	    host?: string;
	    allDbs: boolean;
	    databases?: string[];
	    privileges: string[];
	    saveCredential?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DBCreateUserRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.engine = source["engine"];
	        this.domain = source["domain"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.host = source["host"];
	        this.allDbs = source["allDbs"];
	        this.databases = source["databases"];
	        this.privileges = source["privileges"];
	        this.saveCredential = source["saveCredential"];
	    }
	}
	export class DBCredentialInfo {
	    serverId: string;
	    engine: string;
	    username: string;
	    host: string;
	    verifiedAt?: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DBCredentialInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.engine = source["engine"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.verifiedAt = source["verifiedAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class DBDatabaseInfo {
	    name: string;
	    charset?: string;
	
	    static createFrom(source: any = {}) {
	        return new DBDatabaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.charset = source["charset"];
	    }
	}
	export class DBEngineStatus {
	    engine: string;
	    installed: boolean;
	    active: boolean;
	    enabled: boolean;
	    version?: string;
	    distroId?: string;
	    distroName?: string;
	    packageManager?: string;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DBEngineStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.installed = source["installed"];
	        this.active = source["active"];
	        this.enabled = source["enabled"];
	        this.version = source["version"];
	        this.distroId = source["distroId"];
	        this.distroName = source["distroName"];
	        this.packageManager = source["packageManager"];
	        this.canInstall = source["canInstall"];
	    }
	}
	export class DBGrantsRequest {
	    serverId: string;
	    engine: string;
	    username: string;
	    host?: string;
	    allDbs: boolean;
	    databases?: string[];
	    privileges: string[];
	
	    static createFrom(source: any = {}) {
	        return new DBGrantsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.engine = source["engine"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.allDbs = source["allDbs"];
	        this.databases = source["databases"];
	        this.privileges = source["privileges"];
	    }
	}
	export class DBPrivilegeOption {
	    key: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new DBPrivilegeOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	    }
	}
	export class DBUserInfo {
	    username: string;
	    host?: string;
	
	    static createFrom(source: any = {}) {
	        return new DBUserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.host = source["host"];
	    }
	}
	export class DNSRecord {
	    type: string;
	    name: string;
	    content: string;
	    ttl: number;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new DNSRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.ttl = source["ttl"];
	        this.note = source["note"];
	    }
	}
	export class DNSPreviewResponse {
	    domain: string;
	    serverHost: string;
	    hostValid: boolean;
	    ipKind: string;
	    ttl: number;
	    records: DNSRecord[];
	    zoneText: string;
	    filename: string;
	
	    static createFrom(source: any = {}) {
	        return new DNSPreviewResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.serverHost = source["serverHost"];
	        this.hostValid = source["hostValid"];
	        this.ipKind = source["ipKind"];
	        this.ttl = source["ttl"];
	        this.records = this.convertValues(source["records"], DNSRecord);
	        this.zoneText = source["zoneText"];
	        this.filename = source["filename"];
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
	
	export class DeleteDomainRequest {
	    serverId: string;
	    domain: string;
	    removeRoot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeleteDomainRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.removeRoot = source["removeRoot"];
	    }
	}
	export class DomainDatabaseLink {
	    serverId: string;
	    domain: string;
	    engine: string;
	    database: string;
	
	    static createFrom(source: any = {}) {
	        return new DomainDatabaseLink(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.engine = source["engine"];
	        this.database = source["database"];
	    }
	}
	
	export class NginxStatus {
	    installed: boolean;
	    version?: string;
	    active: boolean;
	    enabled: boolean;
	    distroId?: string;
	    distroName?: string;
	    packageManager?: string;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NginxStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.active = source["active"];
	        this.enabled = source["enabled"];
	        this.distroId = source["distroId"];
	        this.distroName = source["distroName"];
	        this.packageManager = source["packageManager"];
	        this.canInstall = source["canInstall"];
	    }
	}
	export class ListResponse {
	    nginx: NginxStatus;
	    domains: DomainInfo[];
	
	    static createFrom(source: any = {}) {
	        return new ListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nginx = this.convertValues(source["nginx"], NginxStatus);
	        this.domains = this.convertValues(source["domains"], DomainInfo);
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
	export class LogReadRequest {
	    serverId: string;
	    domain: string;
	    logType: string;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new LogReadRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.logType = source["logType"];
	        this.lines = source["lines"];
	    }
	}
	export class LogReadResponse {
	    domain: string;
	    logType: string;
	    path: string;
	    content: string;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LogReadResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.logType = source["logType"];
	        this.path = source["path"];
	        this.content = source["content"];
	        this.exists = source["exists"];
	    }
	}
	export class MySQLColumnInfo {
	    name: string;
	    type: string;
	    nullable: boolean;
	    key?: string;
	    default?: string;
	    extra?: string;
	
	    static createFrom(source: any = {}) {
	        return new MySQLColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.key = source["key"];
	        this.default = source["default"];
	        this.extra = source["extra"];
	    }
	}
	export class MySQLExploreRequest {
	    serverId: string;
	    username: string;
	    host?: string;
	
	    static createFrom(source: any = {}) {
	        return new MySQLExploreRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.host = source["host"];
	    }
	}
	export class MySQLQueryRequest {
	    serverId: string;
	    username: string;
	    host?: string;
	    database: string;
	    sql: string;
	
	    static createFrom(source: any = {}) {
	        return new MySQLQueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.database = source["database"];
	        this.sql = source["sql"];
	    }
	}
	export class MySQLQueryResult {
	    columns?: string[];
	    rows?: string[][];
	    rowsAffected: number;
	    isSelect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MySQLQueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowsAffected = source["rowsAffected"];
	        this.isSelect = source["isSelect"];
	    }
	}
	export class MySQLRowMutateRequest {
	    serverId: string;
	    username: string;
	    host?: string;
	    database: string;
	    table: string;
	    values?: Record<string, string>;
	    where?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MySQLRowMutateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.database = source["database"];
	        this.table = source["table"];
	        this.values = source["values"];
	        this.where = source["where"];
	    }
	}
	export class MySQLTableInfo {
	    name: string;
	    approxRows: number;
	    engine?: string;
	
	    static createFrom(source: any = {}) {
	        return new MySQLTableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.approxRows = source["approxRows"];
	        this.engine = source["engine"];
	    }
	}
	export class MySQLTableRowsRequest {
	    serverId: string;
	    username: string;
	    host?: string;
	    database: string;
	    table: string;
	    limit: number;
	    offset: number;
	    orderBy?: string;
	    orderDir?: string;
	    skipTotal?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MySQLTableRowsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.database = source["database"];
	        this.table = source["table"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.orderBy = source["orderBy"];
	        this.orderDir = source["orderDir"];
	        this.skipTotal = source["skipTotal"];
	    }
	}
	export class MySQLTableRowsResult {
	    columns: string[];
	    rows: string[][];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new MySQLTableRowsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.total = source["total"];
	    }
	}
	
	export class PHPSetDomainRequest {
	    serverId: string;
	    domain: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new PHPSetDomainRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.version = source["version"];
	    }
	}
	export class PHPVersionInfo {
	    version: string;
	    socket: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PHPVersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.socket = source["socket"];
	        this.active = source["active"];
	    }
	}
	export class PHPStatus {
	    installed: boolean;
	    repoConfigured: boolean;
	    distroId?: string;
	    distroName?: string;
	    packageManager?: string;
	    canInstall: boolean;
	    versions: PHPVersionInfo[];
	    available: string[];
	    domainVersion?: string;
	    domainEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PHPStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.repoConfigured = source["repoConfigured"];
	        this.distroId = source["distroId"];
	        this.distroName = source["distroName"];
	        this.packageManager = source["packageManager"];
	        this.canInstall = source["canInstall"];
	        this.versions = this.convertValues(source["versions"], PHPVersionInfo);
	        this.available = source["available"];
	        this.domainVersion = source["domainVersion"];
	        this.domainEnabled = source["domainEnabled"];
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
	
	
	export class ProxyRuleRequest {
	    serverId: string;
	    domain: string;
	    path: string;
	    target: string;
	    webSocket: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProxyRuleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.path = source["path"];
	        this.target = source["target"];
	        this.webSocket = source["webSocket"];
	    }
	}
	export class ProxySetDomainRequest {
	    serverId: string;
	    domain: string;
	    target: string;
	    webSocket: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProxySetDomainRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.target = source["target"];
	        this.webSocket = source["webSocket"];
	    }
	}
	export class ProxyStatus {
	    domain: string;
	    mode: string;
	    proxyTarget?: string;
	    proxyRules: ProxyRule[];
	    phpEnabled: boolean;
	    phpVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.mode = source["mode"];
	        this.proxyTarget = source["proxyTarget"];
	        this.proxyRules = this.convertValues(source["proxyRules"], ProxyRule);
	        this.phpEnabled = source["phpEnabled"];
	        this.phpVersion = source["phpVersion"];
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
	export class SFTPAccountInfo {
	    username: string;
	    domain: string;
	    chroot: string;
	    homeDir: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SFTPAccountInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.domain = source["domain"];
	        this.chroot = source["chroot"];
	        this.homeDir = source["homeDir"];
	        this.enabled = source["enabled"];
	    }
	}
	export class SFTPCreateAccountRequest {
	    serverId: string;
	    domain: string;
	    username: string;
	    password: string;
	    chroot?: string;
	    homeDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new SFTPCreateAccountRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.chroot = source["chroot"];
	        this.homeDir = source["homeDir"];
	    }
	}
	export class SFTPListResponse {
	    domain: string;
	    domainRoot: string;
	    accounts: SFTPAccountInfo[];
	
	    static createFrom(source: any = {}) {
	        return new SFTPListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.domainRoot = source["domainRoot"];
	        this.accounts = this.convertValues(source["accounts"], SFTPAccountInfo);
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
	export class SSLCertificateInfo {
	    domains?: string[];
	    notAfter?: string;
	    certPath?: string;
	    keyPath?: string;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SSLCertificateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domains = source["domains"];
	        this.notAfter = source["notAfter"];
	        this.certPath = source["certPath"];
	        this.keyPath = source["keyPath"];
	        this.exists = source["exists"];
	    }
	}
	export class SSLIssueRequest {
	    serverId: string;
	    domain: string;
	    email: string;
	
	    static createFrom(source: any = {}) {
	        return new SSLIssueRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.email = source["email"];
	    }
	}
	export class SSLStatus {
	    domain: string;
	    isParent: boolean;
	    certbotInstalled: boolean;
	    canInstall: boolean;
	    distroId?: string;
	    distroName?: string;
	    packageManager?: string;
	    sslEnabled: boolean;
	    canIssue: boolean;
	    canEnable: boolean;
	    sans?: string[];
	    certificate: SSLCertificateInfo;
	    nginxInstalled: boolean;
	    autoRenewEnabled: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new SSLStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.isParent = source["isParent"];
	        this.certbotInstalled = source["certbotInstalled"];
	        this.canInstall = source["canInstall"];
	        this.distroId = source["distroId"];
	        this.distroName = source["distroName"];
	        this.packageManager = source["packageManager"];
	        this.sslEnabled = source["sslEnabled"];
	        this.canIssue = source["canIssue"];
	        this.canEnable = source["canEnable"];
	        this.sans = source["sans"];
	        this.certificate = this.convertValues(source["certificate"], SSLCertificateInfo);
	        this.nginxInstalled = source["nginxInstalled"];
	        this.autoRenewEnabled = source["autoRenewEnabled"];
	        this.message = source["message"];
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
	export class SaveDBCredentialRequest {
	    serverId: string;
	    engine: string;
	    username: string;
	    host?: string;
	    password: string;
	    verify: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SaveDBCredentialRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.engine = source["engine"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.password = source["password"];
	        this.verify = source["verify"];
	    }
	}
	export class SetEnabledRequest {
	    serverId: string;
	    domain: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetEnabledRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.domain = source["domain"];
	        this.enabled = source["enabled"];
	    }
	}

}

