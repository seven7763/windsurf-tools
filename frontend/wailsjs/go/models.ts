export namespace main {
	
	export class APIKeyItem {
	    api_key: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new APIKeyItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.remark = source["remark"];
	    }
	}
	export class BackgroundServiceStatus {
	    name: string;
	    platform: string;
	    supported: boolean;
	    installed: boolean;
	    running: boolean;
	    status: string;
	    detail: string;
	    autostart_mitm: boolean;
	    log_path: string;
	    recent_logs: string[];
	    last_log_at: string;
	    last_log_line: string;
	    last_log_tone: string;
	    last_error_at: string;
	    last_error_line: string;
	    recent_error_count: number;
	
	    static createFrom(source: any = {}) {
	        return new BackgroundServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.platform = source["platform"];
	        this.supported = source["supported"];
	        this.installed = source["installed"];
	        this.running = source["running"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.autostart_mitm = source["autostart_mitm"];
	        this.log_path = source["log_path"];
	        this.recent_logs = source["recent_logs"];
	        this.last_log_at = source["last_log_at"];
	        this.last_log_line = source["last_log_line"];
	        this.last_log_tone = source["last_log_tone"];
	        this.last_error_at = source["last_error_at"];
	        this.last_error_line = source["last_error_line"];
	        this.recent_error_count = source["recent_error_count"];
	    }
	}
	export class DesktopRuntimeStatus {
	    status: string;
	    detail: string;
	    log_path: string;
	    recent_logs: string[];
	    last_log_at: string;
	    last_log_line: string;
	    last_log_tone: string;
	    last_error_at: string;
	    last_error_line: string;
	    recent_error_count: number;
	
	    static createFrom(source: any = {}) {
	        return new DesktopRuntimeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.log_path = source["log_path"];
	        this.recent_logs = source["recent_logs"];
	        this.last_log_at = source["last_log_at"];
	        this.last_log_line = source["last_log_line"];
	        this.last_log_tone = source["last_log_tone"];
	        this.last_error_at = source["last_error_at"];
	        this.last_error_line = source["last_error_line"];
	        this.recent_error_count = source["recent_error_count"];
	    }
	}
	export class EmailPasswordItem {
	    email: string;
	    password: string;
	    alt_password?: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new EmailPasswordItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.password = source["password"];
	        this.alt_password = source["alt_password"];
	        this.remark = source["remark"];
	    }
	}
	export class ImportResult {
	    email: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class JWTItem {
	    jwt: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new JWTItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jwt = source["jwt"];
	        this.remark = source["remark"];
	    }
	}
	export class TokenItem {
	    token: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new TokenItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.remark = source["remark"];
	    }
	}

}

export namespace models {
	
	export class Account {
	    id: string;
	    email: string;
	    password?: string;
	    nickname: string;
	    token?: string;
	    refresh_token?: string;
	    windsurf_api_key?: string;
	    plan_name: string;
	    used_quota: number;
	    total_quota: number;
	    daily_remaining: string;
	    weekly_remaining: string;
	    daily_reset_at: string;
	    weekly_reset_at: string;
	    subscription_expires_at: string;
	    token_expires_at: string;
	    status: string;
	    tags: string;
	    remark: string;
	    last_login_at: string;
	    last_quota_update: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.password = source["password"];
	        this.nickname = source["nickname"];
	        this.token = source["token"];
	        this.refresh_token = source["refresh_token"];
	        this.windsurf_api_key = source["windsurf_api_key"];
	        this.plan_name = source["plan_name"];
	        this.used_quota = source["used_quota"];
	        this.total_quota = source["total_quota"];
	        this.daily_remaining = source["daily_remaining"];
	        this.weekly_remaining = source["weekly_remaining"];
	        this.daily_reset_at = source["daily_reset_at"];
	        this.weekly_reset_at = source["weekly_reset_at"];
	        this.subscription_expires_at = source["subscription_expires_at"];
	        this.token_expires_at = source["token_expires_at"];
	        this.status = source["status"];
	        this.tags = source["tags"];
	        this.remark = source["remark"];
	        this.last_login_at = source["last_login_at"];
	        this.last_quota_update = source["last_quota_update"];
	        this.created_at = source["created_at"];
	    }
	}
	export class Settings {
	    proxy_enabled: boolean;
	    proxy_url: string;
	    windsurf_path: string;
	    concurrent_limit: number;
	    auto_refresh_tokens: boolean;
	    auto_refresh_quotas: boolean;
	    quota_refresh_policy: string;
	    quota_custom_interval_minutes: number;
	    auto_switch_plan_filter: string;
	    auto_switch_on_quota_exhausted: boolean;
	    quota_hot_poll_seconds: number;
	    restart_windsurf_after_switch: boolean;
	    minimize_to_tray: boolean;
	    show_desktop_toolbar: boolean;
	    silent_start: boolean;
	    mitm_only: boolean;
	    mitm_tun_mode: boolean;
	    mitm_proxy_enabled: boolean;
	    mitm_debug_dump: boolean;
	    openai_relay_enabled: boolean;
	    openai_relay_port: number;
	    openai_relay_secret: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proxy_enabled = source["proxy_enabled"];
	        this.proxy_url = source["proxy_url"];
	        this.windsurf_path = source["windsurf_path"];
	        this.concurrent_limit = source["concurrent_limit"];
	        this.auto_refresh_tokens = source["auto_refresh_tokens"];
	        this.auto_refresh_quotas = source["auto_refresh_quotas"];
	        this.quota_refresh_policy = source["quota_refresh_policy"];
	        this.quota_custom_interval_minutes = source["quota_custom_interval_minutes"];
	        this.auto_switch_plan_filter = source["auto_switch_plan_filter"];
	        this.auto_switch_on_quota_exhausted = source["auto_switch_on_quota_exhausted"];
	        this.quota_hot_poll_seconds = source["quota_hot_poll_seconds"];
	        this.restart_windsurf_after_switch = source["restart_windsurf_after_switch"];
	        this.minimize_to_tray = source["minimize_to_tray"];
	        this.show_desktop_toolbar = source["show_desktop_toolbar"];
	        this.silent_start = source["silent_start"];
	        this.mitm_only = source["mitm_only"];
	        this.mitm_tun_mode = source["mitm_tun_mode"];
	        this.mitm_proxy_enabled = source["mitm_proxy_enabled"];
	        this.mitm_debug_dump = source["mitm_debug_dump"];
	        this.openai_relay_enabled = source["openai_relay_enabled"];
	        this.openai_relay_port = source["openai_relay_port"];
	        this.openai_relay_secret = source["openai_relay_secret"];
	    }
	}

}

export namespace services {
	
	export class MitmProxyEvent {
	    at: string;
	    message: string;
	    tone: string;
	
	    static createFrom(source: any = {}) {
	        return new MitmProxyEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	        this.message = source["message"];
	        this.tone = source["tone"];
	    }
	}
	export class PoolKeyInfo {
	    key_short: string;
	    healthy: boolean;
	    runtime_exhausted: boolean;
	    cooldown_until: string;
	    has_jwt: boolean;
	    request_count: number;
	    success_count: number;
	    total_exhausted: number;
	    is_current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PoolKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key_short = source["key_short"];
	        this.healthy = source["healthy"];
	        this.runtime_exhausted = source["runtime_exhausted"];
	        this.cooldown_until = source["cooldown_until"];
	        this.has_jwt = source["has_jwt"];
	        this.request_count = source["request_count"];
	        this.success_count = source["success_count"];
	        this.total_exhausted = source["total_exhausted"];
	        this.is_current = source["is_current"];
	    }
	}
	export class MitmProxyStatus {
	    running: boolean;
	    port: number;
	    hosts_mapped: boolean;
	    ca_installed: boolean;
	    current_key: string;
	    pool_status: PoolKeyInfo[];
	    total_requests: number;
	    last_error_kind: string;
	    last_error_summary: string;
	    last_error_at: string;
	    last_error_key: string;
	    recent_events: MitmProxyEvent[];
	
	    static createFrom(source: any = {}) {
	        return new MitmProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.hosts_mapped = source["hosts_mapped"];
	        this.ca_installed = source["ca_installed"];
	        this.current_key = source["current_key"];
	        this.pool_status = this.convertValues(source["pool_status"], PoolKeyInfo);
	        this.total_requests = source["total_requests"];
	        this.last_error_kind = source["last_error_kind"];
	        this.last_error_summary = source["last_error_summary"];
	        this.last_error_at = source["last_error_at"];
	        this.last_error_key = source["last_error_key"];
	        this.recent_events = this.convertValues(source["recent_events"], MitmProxyEvent);
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
	export class OpenAIRelayStatus {
	    running: boolean;
	    port: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenAIRelayStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.url = source["url"];
	    }
	}
	export class PatchResult {
	    success: boolean;
	    already_patched: boolean;
	    modifications: string[];
	    backup_file: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.already_patched = source["already_patched"];
	        this.modifications = source["modifications"];
	        this.backup_file = source["backup_file"];
	        this.message = source["message"];
	    }
	}
	
	export class WindsurfAuthJSON {
	    token: string;
	    email?: string;
	    timestamp?: number;
	
	    static createFrom(source: any = {}) {
	        return new WindsurfAuthJSON(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.email = source["email"];
	        this.timestamp = source["timestamp"];
	    }
	}

}

