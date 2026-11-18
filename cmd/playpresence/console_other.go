//go:build !windows

package main

// launchedByDoubleClick is only detectable on Windows.
func launchedByDoubleClick() bool { return false }
