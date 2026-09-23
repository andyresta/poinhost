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

export namespace dbxfer {
	
	export class ItemResult {
	    database: string;
	    destDatabase: string;
	    status: string;
	    error?: string;
	    warning?: string;
	    tableCount: number;
	    verify?: string;
	    verifyDetail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ItemResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.destDatabase = source["destDatabase"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.warning = source["warning"];
	        this.tableCount = source["tableCount"];
	        this.verify = source["verify"];
	        this.verifyDetail = source["verifyDetail"];
	    }
	}
	export class ItemSelection {
	    database: string;
	    allTables: boolean;
	    tables?: string[];
	    destDatabase?: string;
	
	    static createFrom(source: any = {}) {
	        return new ItemSelection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.allTables = source["allTables"];
	        this.tables = source["tables"];
	        this.destDatabase = source["destDatabase"];
	    }
	}
	export class Progress {
	    jobId: string;
	    status: string;
	    sourceServerId: string;
	    destServerId: string;
	    engine: string;
	    totalBytes: number;
	    doneBytes: number;
	    percent: number;
	    currentItems: string[];
	    items: ItemResult[];
	    message: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.status = source["status"];
	        this.sourceServerId = source["sourceServerId"];
	        this.destServerId = source["destServerId"];
	        this.engine = source["engine"];
	        this.totalBytes = source["totalBytes"];
	        this.doneBytes = source["doneBytes"];
	        this.percent = source["percent"];
	        this.currentItems = source["currentItems"];
	        this.items = this.convertValues(source["items"], ItemResult);
	        this.message = source["message"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
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
	export class StartRequest {
	    sourceServerId: string;
	    destServerId: string;
	    engine: string;
	    items: ItemSelection[];
	
	    static createFrom(source: any = {}) {
	        return new StartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceServerId = source["sourceServerId"];
	        this.destServerId = source["destServerId"];
	        this.engine = source["engine"];
	        this.items = this.convertValues(source["items"], ItemSelection);
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
	export class TableListResponse {
	    tables: string[];
	
	    static createFrom(source: any = {}) {
	        return new TableListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tables = source["tables"];
	    }
	}

}

export namespace docker {
	
	export class ComposeInfo {
	    managed: boolean;
	    project: string;
	    service: string;
	    workingDir: string;
	    configFiles: string;
	    drift: boolean;
	    verdict?: string;
	    unknown: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new ComposeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.managed = source["managed"];
	        this.project = source["project"];
	        this.service = source["service"];
	        this.workingDir = source["workingDir"];
	        this.configFiles = source["configFiles"];
	        this.drift = source["drift"];
	        this.verdict = source["verdict"];
	        this.unknown = source["unknown"];
	        this.message = source["message"];
	    }
	}
	export class ComposeMigrationInfo {
	    managed: boolean;
	    project: string;
	    service: string;
	    workingDir: string;
	    workingDirExists: boolean;
	    workingDirBytes: number;
	    configFiles: string[];
	    siblings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ComposeMigrationInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.managed = source["managed"];
	        this.project = source["project"];
	        this.service = source["service"];
	        this.workingDir = source["workingDir"];
	        this.workingDirExists = source["workingDirExists"];
	        this.workingDirBytes = source["workingDirBytes"];
	        this.configFiles = source["configFiles"];
	        this.siblings = source["siblings"];
	    }
	}
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
	    scope?: string;
	
	    static createFrom(source: any = {}) {
	        return new PortMapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostPort = source["hostPort"];
	        this.containerPort = source["containerPort"];
	        this.protocol = source["protocol"];
	        this.hostIp = source["hostIp"];
	        this.scope = source["scope"];
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
	    extraHosts: string[];
	
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
	        this.extraHosts = source["extraHosts"];
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
	    extraHosts: string[];
	
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
	        this.extraHosts = source["extraHosts"];
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
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new RecreateContainerResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.container = this.convertValues(source["container"], ContainerInfo);
	        this.warning = source["warning"];
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

export namespace dockerxfer {
	
	export class ItemResult {
	    label: string;
	    status: string;
	    error?: string;
	    warning?: string;
	    verify?: string;
	    verifyDetail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ItemResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.warning = source["warning"];
	        this.verify = source["verify"];
	        this.verifyDetail = source["verifyDetail"];
	    }
	}
	export class Progress {
	    jobId: string;
	    status: string;
	    sourceServerId: string;
	    destServerId: string;
	    containerId: string;
	    containerName: string;
	    compress: boolean;
	    copyWorkdir: boolean;
	    totalBytes: number;
	    doneBytes: number;
	    percent: number;
	    currentItems: string[];
	    items: ItemResult[];
	    containerRunning?: boolean;
	    message: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.status = source["status"];
	        this.sourceServerId = source["sourceServerId"];
	        this.destServerId = source["destServerId"];
	        this.containerId = source["containerId"];
	        this.containerName = source["containerName"];
	        this.compress = source["compress"];
	        this.copyWorkdir = source["copyWorkdir"];
	        this.totalBytes = source["totalBytes"];
	        this.doneBytes = source["doneBytes"];
	        this.percent = source["percent"];
	        this.currentItems = source["currentItems"];
	        this.items = this.convertValues(source["items"], ItemResult);
	        this.containerRunning = source["containerRunning"];
	        this.message = source["message"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
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
	export class StartRequest {
	    sourceServerId: string;
	    containerId: string;
	    destServerId: string;
	    compress: boolean;
	    copyWorkdir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceServerId = source["sourceServerId"];
	        this.containerId = source["containerId"];
	        this.destServerId = source["destServerId"];
	        this.compress = source["compress"];
	        this.copyWorkdir = source["copyWorkdir"];
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

export namespace filexfer {
	
	export class DirEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new DirEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	    }
	}
	export class ListDirResponse {
	    path: string;
	    parent: string;
	    entries: DirEntry[];
	
	    static createFrom(source: any = {}) {
	        return new ListDirResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.parent = source["parent"];
	        this.entries = this.convertValues(source["entries"], DirEntry);
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
	export class PathResult {
	    path: string;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PathResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class Progress {
	    jobId: string;
	    status: string;
	    sourceServerId: string;
	    destServerId: string;
	    sourcePaths: string[];
	    destPath: string;
	    compress: boolean;
	    totalBytes: number;
	    doneBytes: number;
	    percent: number;
	    currentPaths: string[];
	    paths: PathResult[];
	    message: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.status = source["status"];
	        this.sourceServerId = source["sourceServerId"];
	        this.destServerId = source["destServerId"];
	        this.sourcePaths = source["sourcePaths"];
	        this.destPath = source["destPath"];
	        this.compress = source["compress"];
	        this.totalBytes = source["totalBytes"];
	        this.doneBytes = source["doneBytes"];
	        this.percent = source["percent"];
	        this.currentPaths = source["currentPaths"];
	        this.paths = this.convertValues(source["paths"], PathResult);
	        this.message = source["message"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
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
	export class StartRequest {
	    sourceServerId: string;
	    sourcePaths: string[];
	    destServerId: string;
	    destPath: string;
	    exclude: string[];
	    compress: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceServerId = source["sourceServerId"];
	        this.sourcePaths = source["sourcePaths"];
	        this.destServerId = source["destServerId"];
	        this.destPath = source["destPath"];
	        this.exclude = source["exclude"];
	        this.compress = source["compress"];
	    }
	}

}

export namespace firewall {
	
	export class DockerSubnet {
	    network: string;
	    subnet: string;
	    gateway: string;
	    covered: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DockerSubnet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.network = source["network"];
	        this.subnet = source["subnet"];
	        this.gateway = source["gateway"];
	        this.covered = source["covered"];
	    }
	}
	export class InstallInfo {
	    distroId: string;
	    distroName: string;
	    packageManager: string;
	    recommended: string;
	    options: string[];
	    installed: string[];
	    blocker?: string;
	    sshPort: number;
	    hasDocker: boolean;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InstallInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.distroId = source["distroId"];
	        this.distroName = source["distroName"];
	        this.packageManager = source["packageManager"];
	        this.recommended = source["recommended"];
	        this.options = source["options"];
	        this.installed = source["installed"];
	        this.blocker = source["blocker"];
	        this.sshPort = source["sshPort"];
	        this.hasDocker = source["hasDocker"];
	        this.canInstall = source["canInstall"];
	    }
	}
	export class Rule {
	    id: string;
	    port: string;
	    protocol: string;
	    source: string;
	    action: string;
	    ipv6: boolean;
	    service: string;
	    servicePorts: string;
	    owner: string;
	    comment: string;
	    raw: string;
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.source = source["source"];
	        this.action = source["action"];
	        this.ipv6 = source["ipv6"];
	        this.service = source["service"];
	        this.servicePorts = source["servicePorts"];
	        this.owner = source["owner"];
	        this.comment = source["comment"];
	        this.raw = source["raw"];
	    }
	}
	export class Status {
	    backend: string;
	    active: boolean;
	    editable: boolean;
	    installed: string[];
	    defaultIncoming: string;
	    defaultOutgoing: string;
	    zone: string;
	    zones: string[];
	    sshPort: number;
	    sshAllowed: boolean;
	    ownerDetectable: boolean;
	    dockerSubnets: DockerSubnet[];
	    dockerDbAllowed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.active = source["active"];
	        this.editable = source["editable"];
	        this.installed = source["installed"];
	        this.defaultIncoming = source["defaultIncoming"];
	        this.defaultOutgoing = source["defaultOutgoing"];
	        this.zone = source["zone"];
	        this.zones = source["zones"];
	        this.sshPort = source["sshPort"];
	        this.sshAllowed = source["sshAllowed"];
	        this.ownerDetectable = source["ownerDetectable"];
	        this.dockerSubnets = this.convertValues(source["dockerSubnets"], DockerSubnet);
	        this.dockerDbAllowed = source["dockerDbAllowed"];
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
	export class ListResponse {
	    status: Status;
	    rules: Rule[];
	
	    static createFrom(source: any = {}) {
	        return new ListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = this.convertValues(source["status"], Status);
	        this.rules = this.convertValues(source["rules"], Rule);
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
	
	export class RuleRequest {
	    serverId: string;
	    port: string;
	    protocol: string;
	    source: string;
	    action: string;
	    comment: string;
	    zone: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.source = source["source"];
	        this.action = source["action"];
	        this.comment = source["comment"];
	        this.zone = source["zone"];
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

export namespace services {
	
	export class JournalEntry {
	    timestamp: string;
	    priority: number;
	    unit: string;
	    identifier: string;
	    pid: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new JournalEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.priority = source["priority"];
	        this.unit = source["unit"];
	        this.identifier = source["identifier"];
	        this.pid = source["pid"];
	        this.message = source["message"];
	    }
	}
	export class JournalRequest {
	    serverId: string;
	    unit: string;
	    priority: string;
	    since: string;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new JournalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.unit = source["unit"];
	        this.priority = source["priority"];
	        this.since = source["since"];
	        this.lines = source["lines"];
	    }
	}
	export class JournalResponse {
	    entries: JournalEntry[];
	    systemdAvailable: boolean;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JournalResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], JournalEntry);
	        this.systemdAvailable = source["systemdAvailable"];
	        this.truncated = source["truncated"];
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
	export class ServiceInfo {
	    name: string;
	    description: string;
	    activeState: string;
	    subState: string;
	    unitFileState: string;
	    running: boolean;
	    enabled: boolean;
	    canEnable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.activeState = source["activeState"];
	        this.subState = source["subState"];
	        this.unitFileState = source["unitFileState"];
	        this.running = source["running"];
	        this.enabled = source["enabled"];
	        this.canEnable = source["canEnable"];
	    }
	}
	export class ListResponse {
	    services: ServiceInfo[];
	    systemdAvailable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.services = this.convertValues(source["services"], ServiceInfo);
	        this.systemdAvailable = source["systemdAvailable"];
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
	    kind: string;
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
	        this.kind = source["kind"];
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

export namespace sitexfer {
	
	export class ItemResult {
	    key: string;
	    kind: string;
	    label: string;
	    status: string;
	    error?: string;
	    warning?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ItemResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.warning = source["warning"];
	        this.detail = source["detail"];
	    }
	}
	export class Problem {
	    blocking: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocking = source["blocking"];
	        this.message = source["message"];
	    }
	}
	export class PlanSFTP {
	    username: string;
	    domain: string;
	    enabled: boolean;
	    hashScheme: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanSFTP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.domain = source["domain"];
	        this.enabled = source["enabled"];
	        this.hashScheme = source["hashScheme"];
	        this.note = source["note"];
	    }
	}
	export class PlanDBUser {
	    engine: string;
	    username: string;
	    host?: string;
	    databases: string[];
	    action: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanDBUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.databases = source["databases"];
	        this.action = source["action"];
	        this.note = source["note"];
	    }
	}
	export class PlanDatabase {
	    engine: string;
	    name: string;
	    domains: string[];
	    owner?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanDatabase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.name = source["name"];
	        this.domains = source["domains"];
	        this.owner = source["owner"];
	    }
	}
	export class PlanPath {
	    path: string;
	    excludes: string[];
	    files: number;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new PlanPath(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.excludes = source["excludes"];
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	    }
	}
	export class PlanDomain {
	    domain: string;
	    parent?: string;
	    isSubdomain: boolean;
	    root: string;
	    enabled: boolean;
	    phpVersion?: string;
	    sslEnabled: boolean;
	    proxy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PlanDomain(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.parent = source["parent"];
	        this.isSubdomain = source["isSubdomain"];
	        this.root = source["root"];
	        this.enabled = source["enabled"];
	        this.phpVersion = source["phpVersion"];
	        this.sslEnabled = source["sslEnabled"];
	        this.proxy = source["proxy"];
	    }
	}
	export class Plan {
	    domains: PlanDomain[];
	    paths: PlanPath[];
	    databases: PlanDatabase[];
	    dbUsers: PlanDBUser[];
	    sftp: PlanSFTP[];
	    cronJobs: number;
	    totalBytes: number;
	    problems: Problem[];
	    canStart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domains = this.convertValues(source["domains"], PlanDomain);
	        this.paths = this.convertValues(source["paths"], PlanPath);
	        this.databases = this.convertValues(source["databases"], PlanDatabase);
	        this.dbUsers = this.convertValues(source["dbUsers"], PlanDBUser);
	        this.sftp = this.convertValues(source["sftp"], PlanSFTP);
	        this.cronJobs = source["cronJobs"];
	        this.totalBytes = source["totalBytes"];
	        this.problems = this.convertValues(source["problems"], Problem);
	        this.canStart = source["canStart"];
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
	
	
	
	
	
	
	export class Progress {
	    jobId: string;
	    status: string;
	    sourceServerId: string;
	    destServerId: string;
	    domains: string[];
	    totalBytes: number;
	    doneBytes: number;
	    percent: number;
	    items: ItemResult[];
	    report: string[];
	    message: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.status = source["status"];
	        this.sourceServerId = source["sourceServerId"];
	        this.destServerId = source["destServerId"];
	        this.domains = source["domains"];
	        this.totalBytes = source["totalBytes"];
	        this.doneBytes = source["doneBytes"];
	        this.percent = source["percent"];
	        this.items = this.convertValues(source["items"], ItemResult);
	        this.report = source["report"];
	        this.message = source["message"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
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
	export class RepairResult {
	    items: ItemResult[];
	
	    static createFrom(source: any = {}) {
	        return new RepairResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ItemResult);
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
	export class StartRequest {
	    sourceServerId: string;
	    destServerId: string;
	    domains: string[];
	    includeDatabases: boolean;
	    includeSftp: boolean;
	    includeCron: boolean;
	    compress: boolean;
	    dbUserPasswords?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new StartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceServerId = source["sourceServerId"];
	        this.destServerId = source["destServerId"];
	        this.domains = source["domains"];
	        this.includeDatabases = source["includeDatabases"];
	        this.includeSftp = source["includeSftp"];
	        this.includeCron = source["includeCron"];
	        this.compress = source["compress"];
	        this.dbUserPasswords = source["dbUserPasswords"];
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
	    owner?: string;
	    ownerHost?: string;
	
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
	        this.owner = source["owner"];
	        this.ownerHost = source["ownerHost"];
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
	export class DBDatabaseUserRequest {
	    serverId: string;
	    engine: string;
	    database: string;
	    username: string;
	    host?: string;
	    privileges?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DBDatabaseUserRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.engine = source["engine"];
	        this.database = source["database"];
	        this.username = source["username"];
	        this.host = source["host"];
	        this.privileges = source["privileges"];
	    }
	}
	export class DBDockerAccessStatus {
	    engine: string;
	    bindAllInterfaces: boolean;
	    firewallDetected?: string;
	    firewallRuleActive: boolean;
	    enabled: boolean;
	    message?: string;
	    localOnlyUsers?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DBDockerAccessStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.bindAllInterfaces = source["bindAllInterfaces"];
	        this.firewallDetected = source["firewallDetected"];
	        this.firewallRuleActive = source["firewallRuleActive"];
	        this.enabled = source["enabled"];
	        this.message = source["message"];
	        this.localOnlyUsers = source["localOnlyUsers"];
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
	    repoConfigured: boolean;
	
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
	        this.repoConfigured = source["repoConfigured"];
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
	    hosts?: string[];
	    databases?: string[];
	    allDatabases?: boolean;
	    privileges?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DBUserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.host = source["host"];
	        this.hosts = source["hosts"];
	        this.databases = source["databases"];
	        this.allDatabases = source["allDatabases"];
	        this.privileges = source["privileges"];
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
	    limit?: number;
	    offset?: number;
	
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
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class MySQLQueryResult {
	    columns?: string[];
	    rows?: string[][];
	    rowsAffected: number;
	    isSelect: boolean;
	    paginated: boolean;
	    hasMore: boolean;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new MySQLQueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowsAffected = source["rowsAffected"];
	        this.isSelect = source["isSelect"];
	        this.paginated = source["paginated"];
	        this.hasMore = source["hasMore"];
	        this.offset = source["offset"];
	    }
	}
	export class MySQLRowMutateRequest {
	    serverId: string;
	    username: string;
	    host?: string;
	    database: string;
	    table: string;
	    values?: Record<string, any>;
	    where?: Record<string, any>;
	
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
	
	export class PGColumnInfo {
	    name: string;
	    type: string;
	    nullable: boolean;
	    key?: string;
	    default?: string;
	
	    static createFrom(source: any = {}) {
	        return new PGColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.key = source["key"];
	        this.default = source["default"];
	    }
	}
	export class PGExploreRequest {
	    serverId: string;
	    username: string;
	
	    static createFrom(source: any = {}) {
	        return new PGExploreRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	    }
	}
	export class PGQueryRequest {
	    serverId: string;
	    username: string;
	    database: string;
	    schema?: string;
	    sql: string;
	    limit?: number;
	    offset?: number;
	
	    static createFrom(source: any = {}) {
	        return new PGQueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.database = source["database"];
	        this.schema = source["schema"];
	        this.sql = source["sql"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class PGQueryResult {
	    columns?: string[];
	    rows?: string[][];
	    rowsAffected: number;
	    isSelect: boolean;
	    paginated: boolean;
	    hasMore: boolean;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new PGQueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowsAffected = source["rowsAffected"];
	        this.isSelect = source["isSelect"];
	        this.paginated = source["paginated"];
	        this.hasMore = source["hasMore"];
	        this.offset = source["offset"];
	    }
	}
	export class PGRowMutateRequest {
	    serverId: string;
	    username: string;
	    database: string;
	    schema?: string;
	    table: string;
	    values?: Record<string, any>;
	    where?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new PGRowMutateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.database = source["database"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.values = source["values"];
	        this.where = source["where"];
	    }
	}
	export class PGTableInfo {
	    name: string;
	    schema: string;
	    approxRows: number;
	
	    static createFrom(source: any = {}) {
	        return new PGTableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.schema = source["schema"];
	        this.approxRows = source["approxRows"];
	    }
	}
	export class PGTableRequest {
	    serverId: string;
	    username: string;
	    database: string;
	    schema?: string;
	
	    static createFrom(source: any = {}) {
	        return new PGTableRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.database = source["database"];
	        this.schema = source["schema"];
	    }
	}
	export class PGTableRowsRequest {
	    serverId: string;
	    username: string;
	    database: string;
	    schema?: string;
	    table: string;
	    limit: number;
	    offset: number;
	    orderBy?: string;
	    orderDir?: string;
	    skipTotal?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PGTableRowsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.username = source["username"];
	        this.database = source["database"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.orderBy = source["orderBy"];
	        this.orderDir = source["orderDir"];
	        this.skipTotal = source["skipTotal"];
	    }
	}
	export class PGTableRowsResult {
	    columns: string[];
	    rows: string[][];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PGTableRowsResult(source);
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
	    missingSans?: string[];
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
	        this.missingSans = source["missingSans"];
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

