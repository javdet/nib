//go:build !linux

package main

// disableDumpable is a no-op off Linux: there is no /proc/<pid>/environ to protect, and the
// backend only ships in Linux images.
func disableDumpable() error { return nil }
