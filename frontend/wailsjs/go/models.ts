export namespace main {
	
	export class Activity {
	    state: string;
	    source: string;
	    changedAt: string;
	    eventPath?: string;
	    eventName?: string;
	
	    static createFrom(source: any = {}) {
	        return new Activity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.source = source["source"];
	        this.changedAt = source["changedAt"];
	        this.eventPath = source["eventPath"];
	        this.eventName = source["eventName"];
	    }
	}
	export class WindowConfig {
	    alwaysOnTop: boolean;
	    configured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WindowConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.configured = source["configured"];
	    }
	}
	export class ProjectRef {
	    id: string;
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class Config {
	    projects: ProjectRef[];
	    worktreeNames: Record<string, string>;
	    window: WindowConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projects = this.convertValues(source["projects"], ProjectRef);
	        this.worktreeNames = source["worktreeNames"];
	        this.window = this.convertValues(source["window"], WindowConfig);
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
	export class GitState {
	    dirtyFiles: number;
	    ahead: number;
	    behind: number;
	    clean: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dirtyFiles = source["dirtyFiles"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.clean = source["clean"];
	    }
	}
	export class HookStatus {
	    enabled: boolean;
	    port: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new HookStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.error = source["error"];
	    }
	}
	
	export class Worktree {
	    path: string;
	    name: string;
	    displayName: string;
	    branch: string;
	    head: string;
	    detached: boolean;
	    bare: boolean;
	    status: GitState;
	    activity: Activity;
	
	    static createFrom(source: any = {}) {
	        return new Worktree(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.branch = source["branch"];
	        this.head = source["head"];
	        this.detached = source["detached"];
	        this.bare = source["bare"];
	        this.status = this.convertValues(source["status"], GitState);
	        this.activity = this.convertValues(source["activity"], Activity);
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
	export class ProjectVM {
	    id: string;
	    name: string;
	    root: string;
	    worktrees: Worktree[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectVM(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.root = source["root"];
	        this.worktrees = this.convertValues(source["worktrees"], Worktree);
	        this.error = source["error"];
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
	export class State {
	    config: Config;
	    projects: ProjectVM[];
	    scanning: boolean;
	    lastUpdated: string;
	    hook: HookStatus;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], Config);
	        this.projects = this.convertValues(source["projects"], ProjectVM);
	        this.scanning = source["scanning"];
	        this.lastUpdated = source["lastUpdated"];
	        this.hook = this.convertValues(source["hook"], HookStatus);
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

