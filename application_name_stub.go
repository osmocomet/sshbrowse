//go:build !linux || !cgo || android || server

package main

func setLinuxApplicationName(string) {}
