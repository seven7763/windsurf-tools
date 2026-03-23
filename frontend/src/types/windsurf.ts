export type ImportMode = 'password' | 'refresh_token' | 'jwt' | 'api_key'

export type AddMode = 'api_key' | 'jwt' | 'refresh_token' | 'password'

export type PlanFilter = 'all' | 'pro' | 'max' | 'team' | 'enterprise' | 'trial' | 'free' | 'unknown'

export type HealthFilter = 'all' | 'healthy' | 'critical' | 'expired' | 'unknown'

export type SortMode = 'quotaAsc' | 'updatedDesc' | 'nameAsc'

export interface Account {
  id: string
  email: string
  password?: string
  nickname: string
  token?: string
  refresh_token?: string
  windsurf_api_key?: string
  plan_name?: string
  used_quota?: number
  total_quota?: number
  daily_remaining?: string
  weekly_remaining?: string
  daily_reset_at?: string
  weekly_reset_at?: string
  subscription_expires_at?: string
  token_expires_at?: string
  status: string
  tags?: string
  remark?: string
  last_login_at?: string
  last_quota_update?: string
  created_at?: string
}

export interface ImportResult {
  email: string
  success: boolean
  error?: string
}

export interface PatchResult {
  success: boolean
  already_patched: boolean
  modifications: string[]
  backup_file: string
  message: string
}
