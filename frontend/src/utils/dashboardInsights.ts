import type { models, services } from '../../wailsjs/go/models'

export type InsightTone = 'info' | 'warning' | 'tip' | 'success'

export type DashboardInsight = {
  id: string
  tone: InsightTone
  title: string
  body?: string
}

function countApiKeyWithoutToken(accounts: models.Account[]): number {
  return accounts.filter((a) => {
    const key = (a.windsurf_api_key || '').trim()
    const tok = (a.token || '').trim()
    return key.length > 0 && tok.length === 0
  }).length
}

/**
 * 根据设置、号池与 MITM 状态生成可操作建议（纯函数，便于单测与扩展）。
 */
export function computeDashboardInsights(input: {
  settings: models.Settings | null
  accounts: models.Account[]
  mitmStatus: services.MitmProxyStatus | null
  mitmOnly: boolean
  patchApplied: boolean
  windsurfPath: string
}): DashboardInsight[] {
  const out: DashboardInsight[] = []
  const s = input.settings
  const acc = input.accounts
  const ms = input.mitmStatus

  if (input.mitmOnly) {
    out.push({
      id: 'mode_mitm_only',
      tone: 'tip',
      title: '当前为「仅 MITM」',
      body: '多号由代理与号池轮换，不写 windsurf_auth；额度用尽不会触发文件切下一席。',
    })
  }

  const nApiNoJwt = countApiKeyWithoutToken(acc)
  if (nApiNoJwt > 0) {
    out.push({
      id: 'api_key_no_jwt',
      tone: 'warning',
      title: `${nApiNoJwt} 个账号仅有 API Key、尚未同步 JWT`,
      body: '建议在控制台点「刷新全部凭证」，便于 MITM 与额度接口稳定工作。',
    })
  }

  if (!input.mitmOnly && input.windsurfPath.trim() && !input.patchApplied) {
    out.push({
      id: 'patch_missing',
      tone: 'warning',
      title: '未检测到无感补丁',
      body: '若使用「下一席」写 windsurf_auth，请在设置中应用无感补丁，否则切号可能无法在 IDE 侧生效。',
    })
  }

  if (s && !input.mitmOnly) {
    const wantSwitch = s.auto_switch_on_quota_exhausted !== false
    const syncQ = s.auto_refresh_quotas === true
    if (wantSwitch && !syncQ) {
      out.push({
        id: 'switch_without_quota_sync',
        tone: 'warning',
        title: '已开启用尽切号，但未开「定期同步额度」',
        body: '无法及时获知当前号是否用尽，建议同时开启定期同步额度，或改为依赖 MITM 轮换。',
      })
    }
  }

  if (ms?.running) {
    if (!ms.ca_installed) {
      out.push({
        id: 'mitm_no_ca',
        tone: 'warning',
        title: 'MITM 已运行但未安装 CA',
        body: 'HTTPS 解密可能失败，请在下方 MITM 面板点击安装 CA 证书。',
      })
    }
    if (!ms.hosts_mapped) {
      out.push({
        id: 'mitm_no_hosts',
        tone: 'warning',
        title: 'MITM 已运行但未配置 Hosts',
        body: '域名可能未指向本机代理，请点击「Hosts 劫持」完成配置（或自行保证解析一致）。',
      })
    }
    const pool = ms.pool_status || []
    const needJwt = pool.filter((k) => k.healthy && !k.has_jwt).length
    if (needJwt > 0) {
      out.push({
        id: 'pool_key_no_jwt',
        tone: 'warning',
        title: `号池中有 ${needJwt} 个 Key 尚未就绪 JWT`,
        body: '请点击「刷新全部凭证」或确认对应账号在号池内可用。',
      })
    }
    if (pool.length > 0 && pool.every((k) => !k.healthy)) {
      out.push({
        id: 'pool_all_unhealthy',
        tone: 'warning',
        title: '号池内 Key 当前均不健康',
        body: '请检查 API Key 是否有效、网络与代理设置，或从账号池移除失效条目。',
      })
    }
  } else if (ms && acc.length > 0 && acc.some((a) => (a.windsurf_api_key || '').trim())) {
    out.push({
      id: 'mitm_stopped_with_pool',
      tone: 'info',
      title: 'MITM 代理未运行',
      body: '号池已有 API Key 时，在下方打开代理即可无感换号；无需为切号重启 IDE。',
    })
  }

  if (acc.length >= 2 && s?.auto_refresh_tokens !== true) {
    out.push({
      id: 'multi_account_tokens',
      tone: 'tip',
      title: '多账号时可开启「自动刷新 Token」',
      body: '减少凭证过期导致的同步失败（设置 → 凭证与额度）。',
    })
  }

  const seen = new Set<string>()
  const deduped = out.filter((x) => {
    if (seen.has(x.id)) return false
    seen.add(x.id)
    return true
  })
  const order: Record<InsightTone, number> = { warning: 0, info: 1, tip: 2, success: 3 }
  return deduped.sort((a, b) => order[a.tone] - order[b.tone]).slice(0, 6)
}
