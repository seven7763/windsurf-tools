/**
 * 对 `wailsjs/go/main/App` 的单一入口封装（业务页请优先用 `APIInfo`）。
 * MITM 相关方法已一并挂到 `APIInfo`，与直连 `App` 等价，见 `README.md`。
 */
import * as AppHooks from '../../wailsjs/go/main/App';
import * as Models from '../../wailsjs/go/models';

export { AppHooks, Models };

// Specific typed helper types matching the Go struct
export interface ImportResult {
  email: string;
  success: boolean;
  error?: string;
}

export const APIInfo = {
  getAllAccounts: AppHooks.GetAllAccounts,
  getAccount: AppHooks.GetAccount,
  deleteAccount: AppHooks.DeleteAccount,
  deleteExpiredAccounts: AppHooks.DeleteExpiredAccounts,
  deleteFreePlanAccounts: AppHooks.DeleteFreePlanAccounts,
  deleteAccountsByGroup: AppHooks.DeleteAccountsByGroup,
  exportAccountsByGroup: AppHooks.ExportAccountsByGroup,

  importByEmailPassword: AppHooks.ImportByEmailPassword,
  importByJWT: AppHooks.ImportByJWT,
  importByAPIKey: AppHooks.ImportByAPIKey,
  importByRefreshToken: AppHooks.ImportByRefreshToken,
  addSingleAccount: AppHooks.AddSingleAccount,

  refreshAllTokens: AppHooks.RefreshAllTokens,
  refreshAllQuotas: AppHooks.RefreshAllQuotas,
  refreshAccountQuota: AppHooks.RefreshAccountQuota,
  getBackgroundServiceStatus: AppHooks.GetBackgroundServiceStatus,
  getDesktopRuntimeStatus: AppHooks.GetDesktopRuntimeStatus,
  controlBackgroundService: AppHooks.ControlBackgroundService,

  getSettings: AppHooks.GetSettings,
  updateSettings: AppHooks.UpdateSettings,

  applyToolbarLayout: AppHooks.ApplyToolbarLayout,
  restoreMainWindowLayout: AppHooks.RestoreMainWindowLayout,

  // MITM（与 AppHooks.* 一一对应，便于统一从 APIInfo 调用）
  startMitmProxy: AppHooks.StartMitmProxy,
  stopMitmProxy: AppHooks.StopMitmProxy,
  getMitmProxyStatus: AppHooks.GetMitmProxyStatus,
  setupMitmCA: AppHooks.SetupMitmCA,
  setupMitmHosts: AppHooks.SetupMitmHosts,
  teardownMitm: AppHooks.TeardownMitm,
  getMitmCAPath: AppHooks.GetMitmCAPath,
  switchMitmToNext: AppHooks.SwitchMitmToNext,
  switchMitmToAccount: AppHooks.SwitchMitmToAccount,

  // OpenAI 中转
  startOpenAIRelay: AppHooks.StartOpenAIRelay,
  stopOpenAIRelay: AppHooks.StopOpenAIRelay,
  getOpenAIRelayStatus: AppHooks.GetOpenAIRelayStatus,

  // MITM debug dump
  toggleMitmDebugDump: AppHooks.ToggleMitmDebugDump,
};
