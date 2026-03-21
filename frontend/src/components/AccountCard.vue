<script setup lang="ts">
import { computed } from 'vue'
import {
  Calendar,
  Clock,
  Delete,
  Key,
  Message,
  RefreshRight,
  SwitchButton,
  Warning,
} from '@element-plus/icons-vue'
import type { Account } from '../types/windsurf'
import { formatResetCountdownZH } from '../utils/datetimeAsia'
import {
  formatCompactDate,
  formatDateTime,
  formatSubscriptionExpiryLine,
  formatMonthlyUsage,
  formatQuota,
  getAccountHealth,
  getHealthLabel,
  getLowestRemaining,
  getPlanLabel,
  getPlanTone,
  getTokenPreview,
  getTokenSource,
  parsePercent,
} from '../utils/account'

const props = defineProps<{
  account: Account
}>()

const emit = defineEmits<{
  switch: [id: string, email: string]
  autoSwitch: [id: string]
  delete: [id: string, email: string]
}>()

const health = computed(() => getAccountHealth(props.account))
const planTone = computed(() => getPlanTone(props.account.plan_name))
const lowestRemaining = computed(() => getLowestRemaining(props.account))
const dailyPercent = computed(() => Math.max(0, Math.min(100, parsePercent(props.account.daily_remaining) ?? 0)))
const weeklyPercent = computed(() => Math.max(0, Math.min(100, parsePercent(props.account.weekly_remaining) ?? 0)))
const tokenPreview = computed(() => getTokenPreview(props.account))
const tokenSource = computed(() => getTokenSource(props.account))
const healthLabel = computed(() => getHealthLabel(health.value))
const identity = computed(() => props.account.nickname || props.account.email)
const subLabel = computed(() => (props.account.email && props.account.nickname ? props.account.email : props.account.remark || ''))
const monthlyUsage = computed(() => formatMonthlyUsage(props.account))
</script>

<template>
  <article class="account-card" :class="[`account-card--${planTone}`, `account-card--${health}`]">
    <div class="account-card__accent" />

    <header class="account-card__header">
      <div>
        <p class="account-card__name">{{ identity }}</p>
        <p v-if="subLabel" class="account-card__email">{{ subLabel }}</p>
      </div>

      <div class="account-card__badges">
        <span class="badge badge--plan" :class="`badge--${planTone}`">{{ getPlanLabel(props.account.plan_name) }}</span>
        <span class="badge badge--health" :class="`badge--${health}`">{{ healthLabel }}</span>
      </div>
    </header>

    <section class="account-card__quota">
      <div class="quota-tile">
        <span class="quota-tile__label">日额度</span>
        <strong class="quota-tile__value">{{ formatQuota(props.account.daily_remaining) }}</strong>
        <small class="quota-tile__meta">日额度下次刷新（每日 东八区 16:00）{{ formatDateTime(props.account.daily_reset_at) }}</small>
        <small v-if="props.account.daily_reset_at" class="quota-tile__meta quota-tile__meta--hint">{{
          formatResetCountdownZH(props.account.daily_reset_at)
        }}</small>
        <div class="quota-tile__meter">
          <span class="quota-tile__fill" :style="{ width: `${dailyPercent}%` }" />
        </div>
      </div>

      <div class="quota-tile">
        <span class="quota-tile__label">周额度</span>
        <strong class="quota-tile__value">{{ formatQuota(props.account.weekly_remaining) }}</strong>
        <small class="quota-tile__meta">周期额度下次刷新（周末 东八区 16:00）{{ formatDateTime(props.account.weekly_reset_at) }}</small>
        <small v-if="props.account.weekly_reset_at" class="quota-tile__meta quota-tile__meta--hint">{{
          formatResetCountdownZH(props.account.weekly_reset_at)
        }}</small>
        <div class="quota-tile__meter">
          <span class="quota-tile__fill quota-tile__fill--cyan" :style="{ width: `${weeklyPercent}%` }" />
        </div>
      </div>

      <div class="quota-tile quota-tile--compact">
        <span class="quota-tile__label">月配额</span>
        <strong class="quota-tile__value">{{ monthlyUsage }}</strong>
        <small class="quota-tile__meta">
          {{ lowestRemaining === null ? '待同步' : `最低余量 ${lowestRemaining.toFixed(1)}%` }}
        </small>
      </div>
    </section>

    <section class="account-card__meta">
      <div class="meta-row">
        <Message class="meta-row__icon" />
        <span>{{ props.account.email || '未记录邮箱' }}</span>
      </div>

      <div class="meta-row">
        <Key class="meta-row__icon" />
        <span>{{ tokenSource }}</span>
        <code>{{ tokenPreview }}</code>
      </div>

      <div class="meta-row">
        <Calendar class="meta-row__icon" />
        <span>{{ formatSubscriptionExpiryLine(props.account.subscription_expires_at) }}</span>
        <span class="meta-row__muted">创建 {{ formatCompactDate(props.account.created_at) }}</span>
      </div>

      <div class="meta-row">
        <Clock class="meta-row__icon" />
        <span>最近同步 {{ formatDateTime(props.account.last_quota_update) }}</span>
      </div>

      <div v-if="props.account.remark" class="meta-row meta-row--remark">
        <Warning class="meta-row__icon" />
        <span>{{ props.account.remark }}</span>
      </div>
    </section>

    <footer class="account-card__footer">
      <button class="control-button control-button--primary" @click="emit('switch', props.account.id, props.account.email)">
        <SwitchButton class="control-button__icon" />
        立即切换
      </button>

      <button class="control-button control-button--ghost" title="切到下一个可用账号" @click="emit('autoSwitch', props.account.id)">
        <RefreshRight class="control-button__icon" />
        下一席位
      </button>

      <button class="control-button control-button--danger" title="删除当前账号" @click="emit('delete', props.account.id, props.account.email)">
        <Delete class="control-button__icon" />
      </button>
    </footer>
  </article>
</template>

<style scoped>
.account-card {
  position: relative;
  overflow: hidden;
  border-radius: 28px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background:
    linear-gradient(160deg, rgba(255, 255, 255, 0.05), rgba(9, 14, 22, 0.95) 72%),
    radial-gradient(circle at top right, rgba(255, 255, 255, 0.08), transparent 30%);
  padding: 1.2rem;
  display: grid;
  gap: 1rem;
  min-height: 330px;
  box-shadow:
    0 20px 40px rgba(3, 8, 18, 0.24),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
  transition:
    transform 180ms ease,
    border-color 180ms ease,
    box-shadow 180ms ease;
}

.account-card:hover {
  transform: translateY(-4px);
  border-color: rgba(255, 255, 255, 0.16);
  box-shadow:
    0 24px 52px rgba(3, 8, 18, 0.34),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

.account-card__accent {
  position: absolute;
  inset: 0 auto auto 0;
  width: 100%;
  height: 4px;
  opacity: 0.95;
}

.account-card--pro .account-card__accent {
  background: linear-gradient(90deg, rgba(72, 182, 255, 0.95), rgba(85, 255, 221, 0.9));
}

.account-card--max .account-card__accent {
  background: linear-gradient(90deg, rgba(255, 126, 95, 0.96), rgba(254, 180, 123, 0.92));
}

.account-card--team .account-card__accent {
  background: linear-gradient(90deg, rgba(72, 230, 165, 0.95), rgba(140, 255, 109, 0.9));
}

.account-card--enterprise .account-card__accent {
  background: linear-gradient(90deg, rgba(178, 141, 255, 0.95), rgba(99, 102, 241, 0.9));
}

.account-card--trial .account-card__accent {
  background: linear-gradient(90deg, rgba(255, 196, 92, 0.95), rgba(255, 119, 74, 0.9));
}

.account-card--free .account-card__accent,
.account-card--unknown .account-card__accent {
  background: linear-gradient(90deg, rgba(144, 164, 198, 0.9), rgba(92, 114, 148, 0.85));
}

.account-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.8rem;
}

.account-card__name {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
  line-height: 1.05;
  color: var(--wt-text-strong);
}

.account-card__email {
  margin: 0.45rem 0 0;
  color: var(--wt-text-muted);
  font-size: 0.9rem;
}

.account-card__badges {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.45rem;
}

.badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 28px;
  padding: 0.28rem 0.72rem;
  border-radius: 999px;
  border: 1px solid transparent;
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.badge--plan.badge--pro {
  background: rgba(72, 182, 255, 0.14);
  color: #8ad7ff;
  border-color: rgba(72, 182, 255, 0.28);
}

.badge--plan.badge--max {
  background: rgba(255, 126, 95, 0.15);
  color: #ffb08f;
  border-color: rgba(255, 126, 95, 0.28);
}

.badge--plan.badge--team {
  background: rgba(72, 230, 165, 0.14);
  color: #75f2c6;
  border-color: rgba(72, 230, 165, 0.28);
}

.badge--plan.badge--enterprise {
  background: rgba(134, 124, 255, 0.15);
  color: #b9b3ff;
  border-color: rgba(134, 124, 255, 0.28);
}

.badge--plan.badge--trial {
  background: rgba(255, 182, 88, 0.14);
  color: #ffc978;
  border-color: rgba(255, 182, 88, 0.28);
}

.badge--plan.badge--free,
.badge--plan.badge--unknown {
  background: rgba(151, 168, 194, 0.12);
  color: #b8c6d9;
  border-color: rgba(151, 168, 194, 0.22);
}

.badge--health.badge--healthy {
  background: rgba(80, 224, 177, 0.14);
  color: #89f8cb;
  border-color: rgba(80, 224, 177, 0.28);
}

.badge--health.badge--critical {
  background: rgba(255, 174, 77, 0.16);
  color: #ffc580;
  border-color: rgba(255, 174, 77, 0.32);
}

.badge--health.badge--expired {
  background: rgba(255, 98, 127, 0.16);
  color: #ff9ab0;
  border-color: rgba(255, 98, 127, 0.3);
}

.badge--health.badge--unknown {
  background: rgba(151, 168, 194, 0.12);
  color: #b8c6d9;
  border-color: rgba(151, 168, 194, 0.22);
}

.account-card__quota {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.7rem;
}

.quota-tile {
  padding: 0.85rem;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.06);
  display: grid;
  gap: 0.4rem;
}

.quota-tile--compact {
  align-content: center;
}

.quota-tile__label {
  font-size: 0.74rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--wt-text-soft);
}

.quota-tile__value {
  font-size: 1.1rem;
  color: var(--wt-text-strong);
  font-family: var(--wt-font-mono);
}

.quota-tile__meta {
  color: var(--wt-text-muted);
  font-size: 0.82rem;
}

.quota-tile__meta--hint {
  font-size: 0.76rem;
  opacity: 0.92;
  margin-top: -0.1rem;
}

.quota-tile__meter {
  width: 100%;
  height: 7px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

.quota-tile__fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #ffbf6f, #4edbff);
}

.quota-tile__fill--cyan {
  background: linear-gradient(90deg, #64f5dd, #4edbff);
}

.account-card__meta {
  display: grid;
  gap: 0.65rem;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  font-size: 0.92rem;
  color: var(--wt-text-body);
}

.meta-row__icon {
  flex: 0 0 auto;
  width: 1rem;
  color: var(--wt-text-soft);
}

.meta-row code {
  margin-left: auto;
  padding: 0.2rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  color: var(--wt-text-soft);
  font-family: var(--wt-font-mono);
  font-size: 0.78rem;
}

.meta-row__muted {
  margin-left: auto;
  color: var(--wt-text-muted);
  font-size: 0.84rem;
}

.meta-row--remark {
  align-items: flex-start;
  padding-top: 0.15rem;
}

.account-card__footer {
  display: grid;
  grid-template-columns: 1.5fr 1fr auto;
  gap: 0.6rem;
}

@media (max-width: 840px) {
  .account-card__quota {
    grid-template-columns: 1fr;
  }

  .account-card__footer {
    grid-template-columns: 1fr;
  }
}
</style>
