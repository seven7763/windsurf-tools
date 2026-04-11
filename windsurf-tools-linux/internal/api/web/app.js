const state = {
  accounts: [],
  filtered: [],
  currentId: '',
}

const els = {
  dataDir: document.getElementById('dataDir'),
  generatedAt: document.getElementById('generatedAt'),
  summaryGrid: document.getElementById('summaryGrid'),
  planCounts: document.getElementById('planCounts'),
  compatibilityList: document.getElementById('compatibilityList'),
  accountRows: document.getElementById('accountRows'),
  searchInput: document.getElementById('searchInput'),
  refreshButton: document.getElementById('refreshButton'),
  newAccountButton: document.getElementById('newAccountButton'),
  accountForm: document.getElementById('accountForm'),
  deleteButton: document.getElementById('deleteButton'),
  resetButton: document.getElementById('resetButton'),
  editorTitle: document.getElementById('editorTitle'),
  settingsPreview: document.getElementById('settingsPreview'),
}

const fieldIds = [
  'accountId',
  'email',
  'nickname',
  'planName',
  'status',
  'usedQuota',
  'totalQuota',
  'dailyRemaining',
  'weeklyRemaining',
  'dailyResetAt',
  'weeklyResetAt',
  'subscriptionExpiresAt',
  'token',
  'refreshToken',
  'apiKey',
  'tags',
  'remark',
]

async function fetchState() {
  const response = await fetch('/api/state')
  if (!response.ok) {
    throw new Error(`加载失败: ${response.status}`)
  }
  return response.json()
}

function renderSummary(summary) {
  const cards = [
    ['总账号数', summary.total_accounts],
    ['活跃账号', summary.active_accounts],
    ['低额度', summary.low_quota_accounts],
    ['即将到期', summary.expiring_soon],
    ['缺少凭证', summary.missing_credentials],
  ]
  els.summaryGrid.innerHTML = cards
    .map(([label, value]) => `<article class="summary-card"><span>${label}</span><strong>${value}</strong></article>`)
    .join('')

  els.planCounts.innerHTML = summary.plan_counts
    .map((item) => `<span class="chip">${item.plan} · ${item.count}</span>`)
    .join('')
}

function renderCompatibility(items) {
  els.compatibilityList.innerHTML = items.map((item) => `<li>${item}</li>`).join('')
}

function renderSettings(settings) {
  const rows = [
    ['并发限制', settings.concurrent_limit],
    ['自动刷新 Token', settings.auto_refresh_tokens ? '开' : '关'],
    ['自动刷新额度', settings.auto_refresh_quotas ? '开' : '关'],
    ['额度策略', settings.quota_refresh_policy],
    ['自定义额度周期', `${settings.quota_custom_interval_minutes} 分钟`],
    ['导入并发', settings.import_concurrency],
    ['调试日志', settings.debug_log ? '开' : '关'],
    ['Relay', settings.openai_relay_enabled ? `开 · ${settings.openai_relay_port}` : '关'],
  ]
  els.settingsPreview.innerHTML = rows
    .map(([label, value]) => `<div class="settings-item"><span>${label}</span><strong>${value}</strong></div>`)
    .join('')
}

function applyFilter() {
  const query = els.searchInput.value.trim().toLowerCase()
  if (!query) {
    state.filtered = [...state.accounts]
  } else {
    state.filtered = state.accounts.filter((account) =>
      [account.email, account.nickname, account.tags, account.remark, account.plan_name]
        .join(' ')
        .toLowerCase()
        .includes(query),
    )
  }
  renderAccounts()
}

function quotaTone(account) {
  if (account.low_quota) return 'quota-pill quota-pill--danger'
  if (account.quota_percent_numeric >= 0 && account.quota_percent_numeric <= 35) return 'quota-pill quota-pill--warn'
  return 'quota-pill'
}

function credentialLabel(account) {
  const items = []
  if (account.has_token) items.push('JWT')
  if (account.has_refresh_token) items.push('Refresh')
  if (account.has_windsurf_api_key) items.push('API')
  if (account.has_password) items.push('Password')
  return items.length ? items.join(' · ') : '无'
}

function renderAccounts() {
  if (!state.filtered.length) {
    els.accountRows.innerHTML = '<tr><td colspan="6" class="table-empty">没有匹配的账号记录</td></tr>'
    return
  }

  els.accountRows.innerHTML = state.filtered
    .map(
      (account) => `
        <tr class="account-row ${account.id === state.currentId ? 'is-selected' : ''}" data-id="${account.id}">
          <td><div class="account-cell"><strong>${escapeHtml(account.email || '未命名账号')}</strong><span>${escapeHtml(account.nickname || '无昵称')}</span></div></td>
          <td>${escapeHtml(account.plan_name || 'unknown')}</td>
          <td><span class="${quotaTone(account)}">${escapeHtml(account.daily_remaining || account.weekly_remaining || quotaFallback(account))}</span></td>
          <td>${escapeHtml(credentialLabel(account))}</td>
          <td>${renderStatus(account)}</td>
          <td>${escapeHtml(account.subscription_expires_at || '未记录')}</td>
        </tr>
      `,
    )
    .join('')

  document.querySelectorAll('.account-row').forEach((row) => {
    row.addEventListener('click', () => {
      const account = state.accounts.find((item) => item.id === row.dataset.id)
      if (account) fillForm(account)
    })
  })
}

function renderStatus(account) {
  if (account.expired) return '<span class="status status--danger">已到期</span>'
  if (account.expiring_soon) return '<span class="status status--warn">即将到期</span>'
  return `<span class="status">${escapeHtml(account.status || 'active')}</span>`
}

function quotaFallback(account) {
  if (!account.total_quota) return '未同步'
  return `${Math.max(account.total_quota - account.used_quota, 0)}/${account.total_quota}`
}

function fillForm(account) {
  state.currentId = account.id
  els.editorTitle.textContent = `编辑账号 · ${account.email || account.id}`
  els.deleteButton.disabled = false
  setFieldValue('accountId', account.id)
  setFieldValue('email', account.email)
  setFieldValue('nickname', account.nickname)
  setFieldValue('planName', account.plan_name)
  setFieldValue('status', account.status)
  setFieldValue('usedQuota', account.used_quota)
  setFieldValue('totalQuota', account.total_quota)
  setFieldValue('dailyRemaining', account.daily_remaining)
  setFieldValue('weeklyRemaining', account.weekly_remaining)
  setFieldValue('dailyResetAt', account.daily_reset_at)
  setFieldValue('weeklyResetAt', account.weekly_reset_at)
  setFieldValue('subscriptionExpiresAt', account.subscription_expires_at)
  setFieldValue('token', account.token)
  setFieldValue('refreshToken', account.refresh_token)
  setFieldValue('apiKey', account.windsurf_api_key)
  setFieldValue('tags', account.tags)
  setFieldValue('remark', account.remark)
  renderAccounts()
}

function resetForm() {
  state.currentId = ''
  els.editorTitle.textContent = '新建账号'
  els.deleteButton.disabled = true
  fieldIds.forEach((id) => setFieldValue(id, ''))
  setFieldValue('usedQuota', 0)
  setFieldValue('totalQuota', 0)
  renderAccounts()
}

function setFieldValue(id, value) {
  document.getElementById(id).value = value ?? ''
}

function collectForm() {
  return {
    id: document.getElementById('accountId').value.trim(),
    email: document.getElementById('email').value.trim(),
    nickname: document.getElementById('nickname').value.trim(),
    plan_name: document.getElementById('planName').value.trim(),
    status: document.getElementById('status').value.trim(),
    used_quota: Number(document.getElementById('usedQuota').value || 0),
    total_quota: Number(document.getElementById('totalQuota').value || 0),
    daily_remaining: document.getElementById('dailyRemaining').value.trim(),
    weekly_remaining: document.getElementById('weeklyRemaining').value.trim(),
    daily_reset_at: document.getElementById('dailyResetAt').value.trim(),
    weekly_reset_at: document.getElementById('weeklyResetAt').value.trim(),
    subscription_expires_at: document.getElementById('subscriptionExpiresAt').value.trim(),
    token: document.getElementById('token').value.trim(),
    refresh_token: document.getElementById('refreshToken').value.trim(),
    windsurf_api_key: document.getElementById('apiKey').value.trim(),
    tags: document.getElementById('tags').value.trim(),
    remark: document.getElementById('remark').value.trim(),
  }
}

async function saveAccount(event) {
  event.preventDefault()
  const response = await fetch('/api/accounts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(collectForm()),
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '保存失败' }))
    alert(error.error || '保存失败')
    return
  }
  await load()
}

async function deleteAccount() {
  if (!state.currentId) return
  if (!confirm('确认删除这个账号记录吗？')) return

  const response = await fetch(`/api/accounts/${state.currentId}`, { method: 'DELETE' })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '删除失败' }))
    alert(error.error || '删除失败')
    return
  }
  await load()
  resetForm()
}

async function load() {
  const snapshot = await fetchState()
  state.accounts = snapshot.accounts
  state.currentId = state.accounts.some((item) => item.id === state.currentId) ? state.currentId : ''

  els.dataDir.textContent = snapshot.data_dir
  els.generatedAt.textContent = new Date(snapshot.generated_at).toLocaleString()
  renderSummary(snapshot.summary)
  renderCompatibility(snapshot.compatibility)
  renderSettings(snapshot.settings)
  applyFilter()

  if (state.currentId) {
    const current = state.accounts.find((item) => item.id === state.currentId)
    if (current) fillForm(current)
  }
}

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function handleLoadError(error) {
  console.error(error)
  alert(error.message || '操作失败')
}

els.searchInput.addEventListener('input', applyFilter)
els.refreshButton.addEventListener('click', () => load().catch(handleLoadError))
els.newAccountButton.addEventListener('click', resetForm)
els.accountForm.addEventListener('submit', (event) => saveAccount(event).catch(handleLoadError))
els.deleteButton.addEventListener('click', () => deleteAccount().catch(handleLoadError))
els.resetButton.addEventListener('click', resetForm)

load().catch(handleLoadError)
