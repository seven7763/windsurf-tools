import { main } from '../../wailsjs/go/models'
import { parsePasswordLine, mergePasswordContinuationLines } from './importParse'

/**
 * 凭证类型自动检测
 * - sk-ws-*          → api_key
 * - eyJ* (base64)    → jwt
 * - 含 @ + 有密码    → password
 * - 其他长字符串     → refresh_token
 */
export type DetectedType = 'api_key' | 'jwt' | 'password' | 'refresh_token'

export interface DetectedLine {
  type: DetectedType
  raw: string
}

const API_KEY_RE = /^sk-ws-/i
const JWT_RE = /^eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+/
const EMAIL_RE = /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/i

export function detectLineType(line: string): DetectedType {
  const trimmed = line.trim()
  const first = trimmed.split(/\s+/)[0] || ''

  if (API_KEY_RE.test(first)) return 'api_key'
  if (JWT_RE.test(first)) return 'jwt'
  if (EMAIL_RE.test(trimmed)) return 'password'
  // 长字符串 → refresh token
  if (first.length > 40) return 'refresh_token'
  // 短字符串也当 refresh token 兜底
  return 'refresh_token'
}

export interface GroupedImportItems {
  apiKeys: main.APIKeyItem[]
  jwts: main.JWTItem[]
  tokens: main.TokenItem[]
  passwords: main.EmailPasswordItem[]
}

/**
 * 将混合输入按凭证类型自动分组
 */
export function groupImportLines(rawLines: string[]): GroupedImportItems {
  const lines = rawLines.map(l => l.trim()).filter(Boolean)
  const result: GroupedImportItems = {
    apiKeys: [],
    jwts: [],
    tokens: [],
    passwords: [],
  }

  // 先把可能是邮箱+密码续行的合并
  const merged = mergePasswordContinuationLines(lines)

  // 去重 map（邮箱密码按 email 去重）
  const emailSeen = new Map<string, main.EmailPasswordItem>()

  for (const line of merged) {
    const type = detectLineType(line)
    const parts = line.trim().split(/\s+/)
    const first = parts[0] || ''
    const remark = parts.slice(1).join(' ').trim()

    switch (type) {
      case 'api_key':
        result.apiKeys.push(new main.APIKeyItem({ api_key: first, remark }))
        break
      case 'jwt':
        result.jwts.push(new main.JWTItem({ jwt: first, remark }))
        break
      case 'refresh_token':
        result.tokens.push(new main.TokenItem({ token: first, remark }))
        break
      case 'password': {
        const parsed = parsePasswordLine(line)
        if (parsed) {
          emailSeen.set(parsed.email.toLowerCase(), parsed)
        }
        break
      }
    }
  }

  result.passwords = Array.from(emailSeen.values())
  return result
}

export interface DetectionSummary {
  api_key: number
  jwt: number
  refresh_token: number
  password: number
  total: number
}

export function summarizeGrouped(g: GroupedImportItems): DetectionSummary {
  return {
    api_key: g.apiKeys.length,
    jwt: g.jwts.length,
    refresh_token: g.tokens.length,
    password: g.passwords.length,
    total: g.apiKeys.length + g.jwts.length + g.tokens.length + g.passwords.length,
  }
}
