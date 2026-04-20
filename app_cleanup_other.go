//go:build !windows

package main

// getWindsurfProcesses is a no-op on non-Windows platforms.
func getWindsurfProcesses() []map[string]interface{} { return nil }
