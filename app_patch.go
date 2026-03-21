package main

import "windsurf-tools-wails/backend/services"

// ═══════════════════════════════════════
// Patch
// ═══════════════════════════════════════

func (a *App) FindWindsurfPath() (string, error) { return a.patchSvc.FindWindsurfPath() }
func (a *App) ApplySeamlessPatch(p string) (*services.PatchResult, error) {
	return a.patchSvc.ApplyPatch(p)
}
func (a *App) RestoreSeamlessPatch(p string) error     { return a.patchSvc.RestorePatch(p) }
func (a *App) CheckPatchStatus(p string) (bool, error) { return a.patchSvc.CheckPatchStatus(p) }
