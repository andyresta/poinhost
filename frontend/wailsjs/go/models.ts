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

