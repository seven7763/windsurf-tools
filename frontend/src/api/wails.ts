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
  /** 号池与 settings 所在目录（跨平台 WindsurfTools） */
  getAppStoragePath: AppHooks.GetAppStoragePath,
  deleteAccount: AppHooks.DeleteAccount,
  deleteExpiredAccounts: AppHooks.DeleteExpiredAccounts,
  deleteFreePlanAccounts: AppHooks.DeleteFreePlanAccounts,

  importByEmailPassword: AppHooks.ImportByEmailPassword,
  importByJWT: AppHooks.ImportByJWT,
  importByAPIKey: AppHooks.ImportByAPIKey,
  importByRefreshToken: AppHooks.ImportByRefreshToken,
  addSingleAccount: AppHooks.AddSingleAccount,

  switchAccount: AppHooks.SwitchAccount,
  openAccountInIsolatedWindow: AppHooks.OpenAccountInIsolatedWindow,
  autoSwitchToNext: AppHooks.AutoSwitchToNext,
  getCurrentWindsurfAuth: AppHooks.GetCurrentWindsurfAuth,
  getWindsurfAuthPath: AppHooks.GetWindsurfAuthPath,

  refreshAllTokens: AppHooks.RefreshAllTokens,
  refreshAllQuotas: AppHooks.RefreshAllQuotas,
  refreshAccountQuota: AppHooks.RefreshAccountQuota,
  getBackgroundServiceStatus: AppHooks.GetBackgroundServiceStatus,
  getDesktopRuntimeStatus: AppHooks.GetDesktopRuntimeStatus,
  controlBackgroundService: AppHooks.ControlBackgroundService,

  getSettings: AppHooks.GetSettings,
  updateSettings: AppHooks.UpdateSettings,

  findWindsurfPath: AppHooks.FindWindsurfPath,
  applySeamlessPatch: AppHooks.ApplySeamlessPatch,
  applyToolbarLayout: AppHooks.ApplyToolbarLayout,
  restoreMainWindowLayout: AppHooks.RestoreMainWindowLayout,
  restoreSeamlessPatch: AppHooks.RestoreSeamlessPatch,
  checkPatchStatus: AppHooks.CheckPatchStatus,

  // MITM（与 AppHooks.* 一一对应，便于统一从 APIInfo 调用）
  startMitmProxy: AppHooks.StartMitmProxy,
  stopMitmProxy: AppHooks.StopMitmProxy,
  getMitmProxyStatus: AppHooks.GetMitmProxyStatus,
  setupMitmCA: AppHooks.SetupMitmCA,
  setupMitmHosts: AppHooks.SetupMitmHosts,
  teardownMitm: AppHooks.TeardownMitm,
  getMitmCAPath: AppHooks.GetMitmCAPath,

  // OpenAI 中转
  startOpenAIRelay: AppHooks.StartOpenAIRelay,
  stopOpenAIRelay: AppHooks.StopOpenAIRelay,
  getOpenAIRelayStatus: AppHooks.GetOpenAIRelayStatus,

  // MITM debug dump
  toggleMitmDebugDump: AppHooks.ToggleMitmDebugDump,
};
